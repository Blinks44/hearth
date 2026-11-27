package ui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

const (
	defaultVisibleScripts   = 8
	overviewQuickRunScripts = 4
)

type scriptRun struct {
	key            string
	dir            string
	script         projects.Script
	pid            int
	running        bool
	stopping       bool
	restartPending *projects.Script
	err            error
	startedAt      time.Time
	stop           func()
	logs           logView
}

func scriptRunKey(dir, source, name string) string {
	return dir + "\x00" + source + "\x00" + name
}

func (a *App) scriptRunFor(dir string, sc projects.Script) *scriptRun {
	return a.scriptRunByKey(scriptRunKey(dir, sc.Source, sc.Name))
}

func (a *App) scriptRunByKey(key string) *scriptRun {
	for _, r := range a.scriptRuns {
		if r.key == key {
			return r
		}
	}
	return nil
}

// listenerBelongsToPID reports whether a listening process matches a managed
// script process directly, as a direct child, via Unix process group ID
// (e.g. pnpm -> turbo -> rspress/vite), or via Windows ancestor PID chain.
func listenerBelongsToPID(l inventory.DatabaseListener, pid string) bool {
	if pid == "" {
		return false
	}
	return l.PID == pid ||
		(l.PPID != "" && l.PPID == pid) ||
		(l.PGID != "" && l.PGID == pid) ||
		slices.Contains(l.Ancestors, pid)
}

// scriptRunForListener matches a listening process to a Hearth-managed project
// script by PID, parent PID, process group ID, or ancestor chain, with a safe
// project-directory fallback for task runners (like Turborepo) that spawn dev
// servers from a detached daemon outside the script's process tree.
func (a *App) scriptRunForListener(l inventory.DatabaseListener) *scriptRun {
	if l.PID != "" || l.PPID != "" || l.PGID != "" || len(l.Ancestors) > 0 {
		for _, r := range a.scriptRuns {
			if !r.running || r.pid <= 0 {
				continue
			}
			if listenerBelongsToPID(l, strconv.Itoa(r.pid)) {
				return r
			}
		}
	}
	if !l.Own || len(l.Dirs) == 0 || isDockerProcess(l.Process) {
		return nil
	}
	if inventory.ClassifyDevServer(l).Role != inventory.RoleDevServer {
		return nil
	}
	var candidate *scriptRun
	for _, r := range a.scriptRuns {
		if !r.running || r.dir == "" {
			continue
		}
		if !listenerWithinDir(l.Dirs, r.dir) {
			continue
		}
		if !l.StartedAt.IsZero() && !r.startedAt.IsZero() && l.StartedAt.Before(r.startedAt.Add(-5*time.Second)) {
			continue
		}
		if candidate != nil {
			// Ambiguous: multiple running scripts cover this project directory.
			return nil
		}
		candidate = r
	}
	return candidate
}

func listenerWithinDir(dirs []string, root string) bool {
	rootSlash := filepath.ToSlash(filepath.Clean(root))
	for _, d := range dirs {
		if d != "" && within(filepath.ToSlash(filepath.Clean(d)), rootSlash) {
			return true
		}
	}
	return false
}

// scriptListeners returns all active port rows belonging to a running managed script.
func (a *App) scriptListeners(r *scriptRun, rows []portRow) []portRow {
	if r == nil || !r.running {
		return nil
	}
	var out []portRow
	for _, row := range rows {
		if a.scriptRunForListener(row.DatabaseListener) == r {
			out = append(out, row)
		}
	}
	return out
}

type scriptListenerSummary struct {
	firstPort     string
	firstHTTPPort string
	kind          string
	cpu           float64
	memBytes      int64
	ports         []string
}

func summarizeScriptListeners(matched []portRow) scriptListenerSummary {
	var s scriptListenerSummary
	for _, m := range matched {
		s.ports = append(s.ports, ":"+m.Port)
		info := inventory.ClassifyDevServer(m.DatabaseListener)
		if s.firstPort == "" {
			s.firstPort = m.Port
		}
		if s.firstHTTPPort == "" && (info.HTTPOpenable || !nonHTTPPorts[m.Port]) {
			s.firstHTTPPort = m.Port
		}
		if s.kind == "" && info.Kind != "" {
			s.kind = info.Kind
		}
		if m.CPUPercent > s.cpu {
			s.cpu = m.CPUPercent
		}
		if m.MemoryBytes > s.memBytes {
			s.memBytes = m.MemoryBytes
		}
	}
	return s
}

func (a *App) startScript(dir string, sc projects.Script) {
	if a.RunScript == nil || len(sc.Argv) == 0 {
		return
	}
	key := scriptRunKey(dir, sc.Source, sc.Name)
	if existing := a.scriptRunByKey(key); existing != nil && existing.running {
		return
	}
	run := &scriptRun{
		key:       key,
		dir:       dir,
		script:    sc,
		running:   true,
		startedAt: time.Now(),
	}
	run.logs.times = true
	a.scriptRuns = slices.DeleteFunc(a.scriptRuns, func(other *scriptRun) bool { return other.key == key })
	a.scriptRuns = append(a.scriptRuns, run)

	spec := ops.ScriptSpec{
		Dir:    dir,
		Source: sc.Source,
		Name:   sc.Name,
		Argv:   slices.Clone(sc.Argv),
	}
	run.stop = a.RunScript(spec, func(pid int) {
		run.pid = pid
	}, func(lines []containerlog.LogLine) {
		run.logs.addLines(lines)
	}, func(err error) {
		run.running = false
		run.stop = nil
		run.logs.ended = true
		if !run.stopping && err != nil {
			run.err = err
			run.logs.err = err
		}
		run.stopping = false
		next := run.restartPending
		run.restartPending = nil
		if next != nil {
			wasViewingLogs := a.scriptLogsKey == run.key
			a.startScript(run.dir, *next)
			if wasViewingLogs {
				a.scriptLogsKey = scriptRunKey(run.dir, next.Source, next.Name)
			}
			return
		}
		a.refreshPortsNow()
	})
}

func (a *App) stopScript(r *scriptRun) {
	if r == nil {
		return
	}
	r.restartPending = nil
	if !r.running || r.stop == nil {
		return
	}
	r.stopping = true
	r.stop()
}

func (a *App) restartScript(r *scriptRun) {
	if r == nil {
		return
	}
	sc := r.script
	if r.running && r.stop != nil {
		a.stopScript(r)
		r.restartPending = &sc
		return
	}
	wasViewingLogs := a.scriptLogsKey == r.key
	dir := r.dir
	a.startScript(dir, sc)
	if wasViewingLogs {
		a.scriptLogsKey = scriptRunKey(dir, sc.Source, sc.Name)
	}
}

// StopAllScripts terminates all running Hearth-managed project scripts.
func (a *App) StopAllScripts() {
	for _, r := range a.scriptRuns {
		r.restartPending = nil
		if r.running && r.stop != nil {
			r.stopping = true
			r.stop()
		}
	}
}

func formatCPUAndMem(cpu float64, memBytes int64) string {
	var parts []string
	if cpu >= 0.05 || (memBytes > 0 && cpu >= 0) {
		parts = append(parts, fmt.Sprintf("%.1f%% CPU", cpu))
	}
	if memBytes > 0 {
		parts = append(parts, formatBytes(memBytes))
	}
	return strings.Join(parts, " · ")
}

func listenerURL(port string) string {
	scheme := "http://"
	if port == "443" || port == "8443" {
		scheme = "https://"
	}
	return scheme + "localhost:" + port
}

func (a *App) projectRunningCounts(p projects.Project) int {
	n := 0
	for _, r := range a.scriptRuns {
		if r.dir == p.Dir && r.running {
			n++
		}
	}
	for _, row := range a.portRows() {
		if row.Project == p.Dir && a.scriptRunForListener(row.DatabaseListener) == nil && !a.isManagedForwardListener(row.DatabaseListener) {
			n++
		}
	}
	if link, ok := a.KubeLinks[p.Dir]; ok {
		for _, f := range a.forwards {
			if !f.ended && f.target.Context == link.Context {
				n++
			}
		}
	}
	return n
}

// projectPrimaryRunning returns the primary running service's port and caption
// for the Project Hero status card (Sketch frame 05).
func (a *App) projectPrimaryRunning(p projects.Project) (port, caption string, ok bool) {
	allPorts := a.portRows()
	for _, r := range a.scriptRuns {
		if r.dir != p.Dir || !r.running {
			continue
		}
		sum := summarizeScriptListeners(a.scriptListeners(r, allPorts))
		var parts []string
		if sum.kind != "" {
			parts = append(parts, sum.kind)
		} else {
			parts = append(parts, r.script.Name)
		}
		if sum.memBytes > 0 {
			parts = append(parts, formatBytes(sum.memBytes))
		}
		return sum.firstPort, strings.Join(parts, " · "), true
	}
	for _, row := range allPorts {
		if row.Project != p.Dir || a.scriptRunForListener(row.DatabaseListener) != nil || a.isManagedForwardListener(row.DatabaseListener) {
			continue
		}
		info := inventory.ClassifyDevServer(row.DatabaseListener)
		var parts []string
		parts = append(parts, cmp.Or(info.Kind, row.Process))
		if row.MemoryBytes > 0 {
			parts = append(parts, formatBytes(row.MemoryBytes))
		}
		return row.Port, strings.Join(parts, " · "), true
	}
	return "", "", false
}

func (a *App) projectComposeContainers(p projects.Project) []inventory.DockerPortContainer {
	if a.Snapshot == nil || p.Dir == "" {
		return nil
	}
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil {
		return nil
	}
	wantDir := filepath.ToSlash(filepath.Clean(p.Dir))
	var out []inventory.DockerPortContainer
	for _, ct := range st.Docker.PortContainers {
		if ct.ComposeDir == "" {
			continue
		}
		if ct.Status != "" && ct.Status != "running" {
			continue
		}
		if within(filepath.ToSlash(filepath.Clean(ct.ComposeDir)), wantDir) {
			out = append(out, ct)
		}
	}
	return out
}

func dangerIconButton(c *ui.Context, key, icon, label string, size float32, onClick func()) {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ButtonBase(c.Key(key)).Size(size, size).Radius(6).Center().Shrink(0).
		BorderWidth(1).BorderColor(t.Danger.Alpha(0.35)).Background(t.Background).
		Label(label).Tooltip(label).OnClick(onClick)
	if b.Hovered() {
		b.Background(pal.DangerSoft)
	}
	b.Children(func() {
		ui.Icon(c, Icon(icon)).Size(12, 12).TextColor(t.Danger)
	})
}

func subtleSquareIconButton(c *ui.Context, key, icon, label string, size float32, onClick func()) {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ButtonBase(c.Key(key)).Size(size, size).Radius(6).Center().Shrink(0).
		BorderWidth(1).BorderColor(pal.Line).Background(pal.Subtle).
		Label(label).Tooltip(label).OnClick(onClick)
	if b.Hovered() {
		b.Background(pal.Muted)
	}
	b.Children(func() {
		ui.Icon(c, Icon(icon)).Size(12.5, 12.5).TextColor(t.Text)
	})
}

func (a *App) projectServiceSourcesLabel(p projects.Project) string {
	var sources []string
	add := func(s string) {
		if s != "" && !slices.Contains(sources, s) {
			sources = append(sources, s)
		}
	}
	for _, sc := range p.Scripts {
		add(sc.Source)
		if len(sources) >= 2 {
			break
		}
	}
	for _, stack := range composeProjects(p) {
		add(filepath.Base(stack.Files))
	}
	return strings.Join(sources, " · ")
}

func scriptShortSource(source string) string {
	switch source {
	case "package.json":
		return "pkg"
	case "Makefile", "GNUmakefile":
		return "make"
	case "justfile", "Justfile", ".justfile":
		return "just"
	case "Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml":
		return "task"
	case "mise.toml", ".mise.toml":
		return "mise"
	case "pyproject.toml":
		return "py"
	case "Cargo.toml":
		return "cargo"
	case "go.mod":
		return "go"
	default:
		return source
	}
}

// projectServicesSummaryCard renders the compact "Запуск и сервисы" card on
// the Project Overview tab (Sketch frame 05): active running rows with 1-click
// Open/Logs/Restart/Stop plus a quick-run scripts & DB summary strip.
func (a *App) projectServicesSummaryCard(c *ui.Context, p projects.Project) {
	allPorts := a.portRows()
	var managed []*scriptRun
	for _, r := range a.scriptRuns {
		if r.dir == p.Dir && r.running {
			managed = append(managed, r)
		}
	}
	var external []portRow
	for _, row := range allPorts {
		if row.Project != p.Dir || a.scriptRunForListener(row.DatabaseListener) != nil || a.isManagedForwardListener(row.DatabaseListener) {
			continue
		}
		external = append(external, row)
	}
	hasRunning := len(managed) > 0 || len(external) > 0
	if !hasRunning && len(p.Scripts) == 0 && len(p.Databases) == 0 {
		return
	}

	t := c.Theme()
	pal := Colors(t)
	compact := narrow(c)

	ui.Column(c.Key("project-overview-services")).Radius(12).Clip().Background(t.Background).
		BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectTabRunServices()).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if src := a.projectServiceSourcesLabel(p); src != "" {
				clipped(c, src).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			}
		})

		for _, run := range managed {
			r := run
			sum := summarizeScriptListeners(a.scriptListeners(r, allPorts))
			var subParts []string
			if sum.firstPort != "" {
				subParts = append(subParts, "localhost:"+sum.firstPort)
			}
			if !r.startedAt.IsZero() {
				subParts = append(subParts, locales.ProjectStartedAgo(ago(c, r.startedAt)))
			}
			if m := formatCPUAndMem(sum.cpu, sum.memBytes); m != "" {
				subParts = append(subParts, m)
			}
			ui.Row(c.Key("ov-run:"+r.key)).Gap(10).Padding(10, 16).MinHeight(52).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						clipped(c, r.script.Name).MinWidth(0).Font(FontMono).FontSize(13).FontWeight(600)
						if sum.kind != "" {
							ui.Text(c, "· "+sum.kind).FontSize(12.5).TextColor(t.TextMuted).SingleLine().Shrink(0)
						}
					})
					if len(subParts) > 0 {
						clipped(c, strings.Join(subParts, " · ")).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ProjectRunningBadge())
					if sum.firstHTTPPort != "" {
						u := listenerURL(sum.firstHTTPPort)
						subtleSquareIconButton(c, "ov-open:"+r.key, "external-link", locales.PortsOpenBrowser(), 26, func() { c.OpenURL(u) })
					}
					subtleSquareIconButton(c, "ov-logs:"+r.key, "scroll-text", locales.ProjectScriptsLogs(), 26, func() {
						a.scriptLogsKey = r.key
					})
					subtleSquareIconButton(c, "ov-restart:"+r.key, "refresh-cw", locales.ProjectScriptsRestart(), 26, func() {
						a.restartScript(r)
					})
					dangerIconButton(c, "ov-stop:"+r.key, "square", locales.ProjectScriptsStop(), 26, func() {
						a.stopScript(r)
					})
				})
			})
		}

		for _, row := range external {
			r := row
			info := inventory.ClassifyDevServer(r.DatabaseListener)
			_, plan, stoppable := a.portAbout(r)
			title := cmp.Or(info.Kind, r.Process, ":"+r.Port)
			var subParts []string
			subParts = append(subParts, "localhost:"+r.Port)
			if r.PID != "" {
				subParts = append(subParts, locales.PortsPID(r.PID))
			}
			if m := formatCPUAndMem(r.CPUPercent, r.MemoryBytes); m != "" {
				subParts = append(subParts, m)
			}
			ui.Row(c.Key("ov-ext:"+r.Port+":"+r.PID)).Gap(10).Padding(10, 16).MinHeight(52).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					clipped(c, title).FontSize(13).FontWeight(500)
					clipped(c, strings.Join(subParts, " · ")).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ProjectRunningBadge())
					if info.HTTPOpenable && !nonHTTPPorts[r.Port] {
						u := listenerURL(r.Port)
						subtleSquareIconButton(c, "ov-ext-open:"+r.Port+":"+r.PID, "external-link", locales.PortsOpenBrowser(), 26, func() { c.OpenURL(u) })
					}
					if stoppable {
						dangerIconButton(c, "ov-ext-stop:"+r.Port+":"+r.PID, "square", locales.PortsStop(), 26, func() { a.Propose(plan) })
					}
				})
			})
		}

		// Quick-run scripts & databases strip.
		if len(p.Scripts) > 0 || len(p.Databases) > 0 {
			var quick []projects.Script
			for _, sc := range p.Scripts {
				if run := a.scriptRunFor(p.Dir, sc); run != nil && run.running {
					continue
				}
				quick = append(quick, sc)
				if len(quick) >= overviewQuickRunScripts {
					break
				}
			}
			var subParts []string
			if len(p.Databases) > 0 {
				var kinds []string
				for _, db := range p.Databases {
					if !slices.Contains(kinds, db.Kind) {
						kinds = append(kinds, db.Kind)
					}
				}
				subParts = append(subParts, strings.Join(kinds, " · "))
			}
			if len(p.Scripts) > 0 {
				subParts = append(subParts, locales.ProjectScriptsCountShort(len(p.Scripts)))
			}
			row := ui.Row(c.Key("ov-quick-strip")).Gap(10).Padding(10, 16).MinHeight(50).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if compact {
				row.Wrap()
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(140).Gap(2).Children(func() {
					ui.Text(c, locales.ProjectScriptsAndDbs()).FontSize(13).FontWeight(500).SingleLine()
					if len(subParts) > 0 {
						clipped(c, strings.Join(subParts, " · ")).FontSize(11).TextColor(pal.TextFaint)
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
					if a.RunScript != nil {
						nameCount := map[string]int{}
						for _, sc := range p.Scripts {
							nameCount[sc.Name]++
						}
						for _, sc := range quick {
							script := sc
							key := scriptRunKey(p.Dir, script.Source, script.Name)
							tip := script.Source
							if script.Command != "" {
								tip += ": " + script.Command
							}
							btnLabel := locales.ProjectScriptsRun() + " " + script.Name
							if nameCount[script.Name] > 1 {
								btnLabel += " (" + script.Source + ")"
							}
							ui.Button(c.Key("ov-quick:"+key), "").Height(28).Label(btnLabel).Tooltip(tip).
								OnClick(func() { a.startScript(p.Dir, script) }).Children(func() {
								ui.Icon(c, Icon("play")).Size(11, 11).TextColor(t.TextMuted)
								ui.Text(c, script.Name).Font(FontMono).FontSize(12).SingleLine()
								if nameCount[script.Name] > 1 {
									ui.Text(c, scriptShortSource(script.Source)).Font(FontMono).FontSize(10.5).
										TextColor(pal.TextFaint).SingleLine()
								}
							})
						}
					}
					if len(p.Scripts) > len(quick) || len(p.Databases) > 0 {
						link := ui.ButtonBase(c.Key("ov-all-scripts")).Padding(4, 6).Radius(6).Label(locales.ProjectScriptsAll()).
							OnClick(func() { a.projectTab = 1 })
						if link.Hovered() {
							link.Background(pal.Subtle)
						}
						link.Children(func() {
							ui.Text(c, locales.ProjectScriptsAll()).FontSize(12).FontWeight(500).TextColor(pal.AccentInk).SingleLine()
						})
					}
				})
			})
		}
	})
}

// projectRunningCard renders a compact live view of everything currently
// running in this project on the Services tab.
func (a *App) projectRunningCard(c *ui.Context, p projects.Project) {
	t := c.Theme()
	pal := Colors(t)
	allPorts := a.portRows()

	var managed []*scriptRun
	for _, r := range a.scriptRuns {
		if r.dir == p.Dir && r.running {
			managed = append(managed, r)
		}
	}

	var external []portRow
	for _, row := range allPorts {
		if row.Project != p.Dir {
			continue
		}
		if a.scriptRunForListener(row.DatabaseListener) != nil || a.isManagedForwardListener(row.DatabaseListener) {
			continue
		}
		external = append(external, row)
	}

	composeCts := a.projectComposeContainers(p)

	var linkedForwards []*portForward
	if link, ok := a.KubeLinks[p.Dir]; ok {
		for _, f := range a.forwards {
			if !f.ended && f.target.Context == link.Context {
				linkedForwards = append(linkedForwards, f)
			}
		}
	}

	total := len(managed) + len(external) + len(composeCts) + len(linkedForwards)
	if total == 0 {
		return
	}

	ui.Column(c.Key("project-running-card")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectRunningTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, strconv.Itoa(total)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			a.portsRefreshButton(c)
		})

		for _, run := range managed {
			r := run
			sum := summarizeScriptListeners(a.scriptListeners(r, allPorts))
			var subParts []string
			subParts = append(subParts, r.script.Source)
			if len(sum.ports) > 0 {
				subParts = append(subParts, strings.Join(sum.ports, ", "))
			}
			if r.pid > 0 {
				subParts = append(subParts, locales.PortsPID(strconv.Itoa(r.pid)))
			}
			if m := formatCPUAndMem(sum.cpu, sum.memBytes); m != "" {
				subParts = append(subParts, m)
			}
			ui.Row(c.Key("run-row:"+r.key)).Gap(10).Padding(10, 14).MinHeight(50).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, r.script.Name).Font(FontMono).FontSize(13).FontWeight(600).SingleLine()
						if sum.kind != "" {
							aiTag(c, sum.kind, pal.AccentSoft, pal.AccentInk)
						}
					})
					clipped(c, strings.Join(subParts, " · ")).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ProjectScriptsRunning())
					if sum.firstHTTPPort != "" {
						u := listenerURL(sum.firstHTTPPort)
						subtleSquareIconButton(c, "run-open:"+r.key, "external-link", locales.PortsOpenBrowser(), 26, func() {
							c.OpenURL(u)
						})
					}
					subtleSquareIconButton(c, "run-logs:"+r.key, "scroll-text", locales.ProjectScriptsLogs(), 26, func() {
						a.scriptLogsKey = r.key
					})
					subtleSquareIconButton(c, "run-restart:"+r.key, "refresh-cw", locales.ProjectScriptsRestart(), 26, func() {
						a.restartScript(r)
					})
					dangerIconButton(c, "run-stop:"+r.key, "square", locales.ProjectScriptsStop(), 26, func() {
						a.stopScript(r)
					})
				})
			})
		}

		for _, row := range external {
			r := row
			info := inventory.ClassifyDevServer(r.DatabaseListener)
			_, plan, stoppable := a.portAbout(r)
			title := cmp.Or(r.Process, ":"+r.Port)
			var subParts []string
			subParts = append(subParts, ":"+r.Port)
			if r.PID != "" {
				subParts = append(subParts, locales.PortsPID(r.PID))
			}
			if m := formatCPUAndMem(r.CPUPercent, r.MemoryBytes); m != "" {
				subParts = append(subParts, m)
			}
			listRow(c, title, false, strings.Join(subParts, " · "), func() {
				if info.Kind != "" {
					aiTag(c, info.Kind, pal.AccentSoft, pal.AccentInk)
				}
				if info.HTTPOpenable && !nonHTTPPorts[r.Port] {
					u := listenerURL(r.Port)
					subtleSquareIconButton(c, "proj-ext-open:"+r.Port+":"+r.PID, "external-link", locales.PortsOpenBrowser(), 26, func() {
						c.OpenURL(u)
					})
				}
				if stoppable {
					dangerIconButton(c, "proj-ext-stop:"+r.Port+":"+r.PID, "square", locales.PortsStop(), 26, func() {
						a.Propose(plan)
					})
				}
			})
		}

		for _, ct := range composeCts {
			container := ct
			var portLabels []string
			var firstHTTP string
			for _, port := range container.Ports {
				portLabels = append(portLabels, ":"+port)
				if firstHTTP == "" && !nonHTTPPorts[port] {
					firstHTTP = port
				}
			}
			sub := container.Image
			if len(portLabels) > 0 {
				if sub != "" {
					sub += " · "
				}
				sub += strings.Join(portLabels, ", ")
			}
			listRow(c, container.Name, false, sub, func() {
				aiTag(c, "Docker", pal.InfoSoft, pal.InfoText)
				if firstHTTP != "" {
					u := listenerURL(firstHTTP)
					subtleSquareIconButton(c, "proj-ct-open:"+container.ID, "external-link", locales.PortsOpenBrowser(), 26, func() {
						c.OpenURL(u)
					})
				}
				if a.canOpenDockerContainer() {
					subtleSquareIconButton(c, "proj-ct-docker:"+container.ID, "container", locales.OpenInDocker(), 26, func() {
						a.openDockerContainer(container.ID, container.Name)
					})
				}
			})
		}

		for _, fwd := range linkedForwards {
			f := fwd
			portStr := strconv.Itoa(f.target.Local)
			sub := f.target.Name + ":" + strconv.Itoa(f.target.Remote) + " · " + f.target.Context
			listRow(c, "localhost:"+portStr, true, sub, func() {
				aiTag(c, "k8s", pal.WarningSoft, pal.WarningText)
				if !nonHTTPPorts[portStr] {
					u := listenerURL(portStr)
					subtleSquareIconButton(c, "proj-fwd-open:"+portStr, "external-link", locales.PortsOpenBrowser(), 26, func() {
						c.OpenURL(u)
					})
				}
				if f.stop != nil {
					dangerIconButton(c, "proj-fwd-stop:"+portStr, "square", locales.PortsStop(), 26, func() {
						f.stopping = true
						f.stop()
					})
				}
			})
		}
	})
}

// projectScriptsCard renders scripts parsed from package.json, Makefile,
// justfile, Taskfile.yml, mise.toml, pyproject.toml, go.mod, and Cargo.toml
// with search and compact Run / Logs controls.
func (a *App) projectScriptsCard(c *ui.Context, p projects.Project) {
	if len(p.Scripts) == 0 {
		return
	}
	if a.scriptFilterDir != p.Dir {
		a.scriptFilter = ""
		a.scriptSourceFilter = ""
		a.scriptFilterDir = p.Dir
	}
	t := c.Theme()
	pal := Colors(t)

	var sources []string
	sourceCounts := map[string]int{}
	for _, sc := range p.Scripts {
		if sourceCounts[sc.Source] == 0 {
			sources = append(sources, sc.Source)
		}
		sourceCounts[sc.Source]++
	}
	if a.scriptSourceFilter != "" && sourceCounts[a.scriptSourceFilter] == 0 {
		a.scriptSourceFilter = ""
	}

	q := strings.ToLower(strings.TrimSpace(a.scriptFilter))
	var filtered []projects.Script
	for _, sc := range p.Scripts {
		if a.scriptSourceFilter != "" && sc.Source != a.scriptSourceFilter {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(sc.Name+" "+sc.Command+" "+sc.Source), q) {
			continue
		}
		filtered = append(filtered, sc)
	}

	expanded := a.scriptsExpanded[p.Dir] || q != "" || a.scriptSourceFilter != ""
	visible := filtered
	hiddenCount := 0
	if !expanded && len(visible) > defaultVisibleScripts {
		hiddenCount = len(visible) - defaultVisibleScripts
		visible = visible[:defaultVisibleScripts]
	}

	ui.Column(c.Key("project-scripts-card")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectScriptsTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, strconv.Itoa(len(p.Scripts))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			ui.Text(c, locales.ProjectScriptsSubtitle()).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0).SingleLine()
			if len(p.Scripts) > 6 {
				ui.SearchField(c.Key("scripts-filter"), &a.scriptFilter).Placeholder(locales.ProjectScriptsSearch()).Height(28).Width(180)
			}
		})
		if len(sources) > 1 {
			ui.Row(c).Gap(6).Padding(0, 14, 10, 14).Wrap().AlignItems(ui.Center).Children(func() {
				pill := func(key, label string, count int, active bool, onClick func()) {
					b := ui.ButtonBase(c.Key(key)).Row().Gap(4).Padding(3, 8).Radius(6).AlignItems(ui.Center).
						BorderWidth(1).Label(label).OnClick(onClick)
					if active {
						b.Background(pal.AccentSoft).BorderColor(Accent.Alpha(0.35))
					} else {
						b.Background(pal.Subtle).BorderColor(pal.Line)
						if b.Hovered() {
							b.Background(pal.Muted)
						}
					}
					fg := t.TextMuted
					if active {
						fg = pal.AccentInk
					}
					b.Children(func() {
						ui.Text(c, label).Font(FontMono).FontSize(11).FontWeight(500).TextColor(fg).SingleLine()
						ui.Text(c, strconv.Itoa(count)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					})
				}
				pill("script-src:all", locales.TabAll(), len(p.Scripts), a.scriptSourceFilter == "", func() {
					a.scriptSourceFilter = ""
				})
				for _, src := range sources {
					s := src
					pill("script-src:"+s, s, sourceCounts[s], a.scriptSourceFilter == s, func() {
						a.scriptSourceFilter = s
					})
				}
			})
		}
		if len(visible) == 0 {
			cardNote(c, locales.NothingMatches())
			return
		}
		for _, sc := range visible {
			script := sc
			key := scriptRunKey(p.Dir, script.Source, script.Name)
			run := a.scriptRunFor(p.Dir, script)
			ui.Row(c.Key("script-row:"+key)).Gap(10).Padding(8, 14).MinHeight(44).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						clipped(c, script.Name).MinWidth(0).Font(FontMono).FontSize(13).FontWeight(500)
						aiTag(c, script.Source, pal.Muted, pal.MonoText).Shrink(0)
						if run != nil {
							switch {
							case run.running:
								chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ProjectScriptsRunning())
							case run.err != nil:
								chip(c, t.Danger, pal.DangerSoft, pal.DangerText, locales.ProjectScriptsFailed())
							default:
								chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ProjectScriptsExited())
							}
						}
					})
					if script.Command != "" {
						clipped(c, script.Command).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					if run != nil && run.running {
						subtleSquareIconButton(c, "sc-logs:"+key, "scroll-text", locales.ProjectScriptsLogs(), 26, func() {
							a.scriptLogsKey = key
						})
					} else {
						if run != nil && (len(run.logs.lines) > 0 || run.err != nil) {
							subtleSquareIconButton(c, "sc-logs:"+key, "scroll-text", locales.ProjectScriptsLogs(), 26, func() {
								a.scriptLogsKey = key
							})
						}
						if a.RunScript != nil {
							subtleSquareIconButton(c, "sc-run:"+key, "play", locales.ProjectScriptsRun(), 26, func() {
								a.startScript(p.Dir, script)
							})
						}
					}
				})
			})
		}
		if q == "" && a.scriptSourceFilter == "" && (hiddenCount > 0 || expanded && len(p.Scripts) > defaultVisibleScripts) {
			ui.Row(c).Padding(8, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				label := locales.ProjectScriptsShowMore(hiddenCount)
				if expanded {
					label = locales.ProjectScriptsShowLess()
				}
				ui.Button(c.Key("scripts-toggle:"+p.Dir), label).Height(28).OnClick(func() {
					if a.scriptsExpanded == nil {
						a.scriptsExpanded = map[string]bool{}
					}
					a.scriptsExpanded[p.Dir] = !expanded
				})
			})
		}
	})
}

// activeProjectScriptLogs returns the open scriptRun when it belongs to dir.
func (a *App) activeProjectScriptLogs(dir string) *scriptRun {
	if a.scriptLogsKey == "" {
		return nil
	}
	run := a.scriptRunByKey(a.scriptLogsKey)
	if run == nil || run.dir != dir {
		return nil
	}
	return run
}

// projectScriptLogsView renders a full-screen dedicated log screen matching
// Docker and Kubernetes container logs (breadcrumb header, single toolbar row,
// full-height logPanel).
func (a *App) projectScriptLogsView(c *ui.Context, p projects.Project, run *scriptRun) {
	t := c.Theme()
	pal := Colors(t)
	l := &run.logs
	l.endedNote = locales.ProjectScriptsExited()
	rows := l.matches()
	if l.match >= len(rows) {
		l.match = 0
	}
	sum := summarizeScriptListeners(a.scriptListeners(run, a.portRows()))
	name := filepath.Base(p.Dir)

	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}

	ui.Column(c).Fill().Children(func() {
		// Breadcrumb topbar matching Docker/K8s log views.
		ui.Row(c.Key("script-logs-header")).Height(56).Shrink(0).Padding(0, right, 0, 20).Gap(10).AlignItems(ui.Center).
			Background(t.Background).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).DragWindow().Children(func() {
			iconButton(c.Key("script-logs-back"), "chevron-left", locales.ProjectScriptsCloseLogs()).
				Size(controlHeight, controlHeight).OnClick(func() { a.scriptLogsKey = "" })
			ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
				backBtn := ui.ButtonBase(c.Key("script-logs-crumb-proj")).Height(28).Padding(0, 6).Radius(6).
					AlignItems(ui.Center).Label(name).OnClick(func() { a.scriptLogsKey = "" })
				if backBtn.Hovered() {
					backBtn.Background(pal.Muted)
				}
				backBtn.Children(func() {
					clipped(c, name).FontSize(13).FontWeight(500).TextColor(t.TextMuted)
				})
				pathSlash(c)
				clipped(c, run.script.Name).Font(FontMono).FontSize(13).FontWeight(600)
				aiTag(c, locales.KubeLogsTag(), pal.Muted, t.TextMuted).Shrink(0)
				if sum.firstPort != "" {
					aiTag(c, ":"+sum.firstPort, pal.AccentSoft, pal.AccentInk).Shrink(0)
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				switch {
				case run.running:
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ProjectScriptsRunning())
				case run.err != nil:
					chip(c, t.Danger, pal.DangerSoft, pal.DangerText, locales.ProjectScriptsFailed())
				default:
					chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ProjectScriptsExited())
				}
				if run.running && sum.firstHTTPPort != "" {
					u := listenerURL(sum.firstHTTPPort)
					subtleSquareIconButton(c, "logs-open-url:"+run.key, "external-link", locales.PortsOpenBrowser(), controlHeight, func() {
						c.OpenURL(u)
					})
				}
				if run.running {
					subtleSquareIconButton(c, "logs-restart:"+run.key, "refresh-cw", locales.ProjectScriptsRestart(), controlHeight, func() {
						a.restartScript(run)
					})
					dangerIconButton(c, "logs-stop:"+run.key, "square", locales.ProjectScriptsStop(), controlHeight, func() {
						a.stopScript(run)
					})
				} else if a.RunScript != nil {
					ui.Button(c.Key("logs-rerun:"+run.key), locales.ProjectScriptsRun()).Height(controlHeight).OnClick(func() {
						a.startScript(run.dir, run.script)
						a.scriptLogsKey = run.key
					})
				}
				a.logActions(c, l, run.script.Name)
			})
		})

		ui.Column(c.Key("project-script-logs:"+run.key)).Grow(1).MinHeight(0).Gap(12).Padding(18, 28, 24, 28).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				if run.script.Command != "" {
					clipped(c, run.script.Command).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint)
				}
				l.searchControls(c, rows)
			})
			a.logPanel(c, l, rows, locales.ProjectScriptsFailed(), func() {
				ui.Row(c).Gap(8).Padding(10, 14, 10, 16).AlignItems(ui.Center).Background(t.Background).
					BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Text(c, locales.ProjectScriptsLogsTitle(run.script.Name)).FontSize(12.5).FontWeight(500).SingleLine()
					aiTag(c, run.script.Source, pal.Muted, pal.MonoText)
					ui.Box(c).Grow(1).MinWidth(0).Height(1)
					if l.dropped > 0 {
						ui.Text(c, locales.KubeLogsDropped()).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
					}
					ui.Text(c, locales.KubeLogsLines(len(l.lines))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
				})
			})
		})
	})
}
