package ui

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type dockerContainersState struct {
	target        docker.Target
	watching      string
	revision      int
	stop          func()
	items         []docker.Container
	err, statsErr error
	loaded        bool
	filter        int
	query         string
	open          map[string]bool
	bulk          resourceSelection
	planning      bool
	detail        dockerDetailState
	openID        string
	logs          *dockerLogState
}

func (s *dockerContainersState) stopWatch() {
	if s.stop != nil {
		s.stop()
	}
	s.stop, s.watching = nil, ""
	s.revision++
	s.planning = false
}

func (a *App) syncDockerContainers(t docker.Target, key string) {
	s := &a.engine.containers
	if a.engine.view != dockerContainers || a.WatchDockerContainers == nil {
		key = ""
	}
	if key == s.watching {
		return
	}
	s.stopWatch()
	if key == "" || t != s.target {
		s.closeLogs()
		s.closeDetail()
		s.items, s.err, s.statsErr, s.loaded = nil, nil, nil, false
		s.bulk.clear()
	}
	if key == "" {
		return
	}
	s.target, s.watching = t, key
	revision := s.revision
	s.stop = a.WatchDockerContainers(t, func(items []docker.Container, err, statsErr error) {
		if s.revision != revision {
			return
		}
		s.err, s.statsErr = err, statsErr
		if err == nil {
			s.items, s.loaded = items, true
			if id := s.openID; id != "" {
				s.openID = ""
				if slices.ContainsFunc(items, func(c docker.Container) bool { return c.ID == id }) {
					s.selectContainer(id)
				}
			}
			for id := range s.bulk.selected {
				if !slices.ContainsFunc(items, func(c docker.Container) bool { return c.ID == id }) {
					delete(s.bulk.selected, id)
				}
			}
		}
	})
}

const (
	dockerAll = iota
	dockerProblems
	dockerRunning
	dockerStopped
)

func showDockerContainer(item docker.Container, filter int, query string) bool {
	switch {
	case filter == dockerProblems && !item.Problem():
		return false
	case filter == dockerRunning && !item.Active():
		return false
	case filter == dockerStopped && item.Active():
		return false
	}
	return query == "" || strings.Contains(strings.ToLower(item.Name+" "+item.Image+" "+item.Ports), query) ||
		item.ConflictPort() > 0 && strings.Contains(strconv.Itoa(item.ConflictPort()), query)
}

type dockerGroup struct {
	key, name, kind, dir string
	compose              docker.ComposeProject
	items                []docker.Container
	running, problems    int
}

func (a *App) dockerGroups(items []docker.Container, filter int, query string) []dockerGroup {
	groups := make(map[string]*dockerGroup)
	for _, item := range items {
		g := dockerGroup{key: "standalone", name: locales.DockerNoProject()}
		switch {
		case item.Kind != "":
			g = dockerGroup{key: "kind:" + item.Kind, name: item.Kind, kind: "kind"}
		case item.Compose.Name != "":
			g = dockerGroup{key: "compose:" + item.Compose.Name + "\n" + item.Compose.Dir,
				name: item.Compose.Name, kind: "Compose", compose: item.Compose}
			for _, p := range a.Projects {
				if item.Compose.Dir != "" && filepath.Clean(item.Compose.Dir) == filepath.Clean(p.Dir) {
					g.dir = p.Dir
					break
				}
			}
		}
		if groups[g.key] == nil {
			groups[g.key] = &g
		}
		group := groups[g.key]
		if item.Problem() {
			group.problems++
		}
		if item.Active() {
			group.running++
		}
		if showDockerContainer(item, filter, query) {
			group.items = append(group.items, item)
		}
	}
	var result []dockerGroup
	for _, g := range groups {
		if len(g.items) > 0 {
			result = append(result, *g)
		}
	}
	if filter == dockerAll || filter == dockerStopped {
		for _, p := range a.Projects {
			project, ok := composeProject(p)
			if !ok || query != "" && !strings.Contains(strings.ToLower(p.Dir), query) {
				continue
			}
			if !slices.ContainsFunc(items, func(item docker.Container) bool {
				return item.Compose.Dir != "" && filepath.Clean(item.Compose.Dir) == filepath.Clean(p.Dir)
			}) {
				result = append(result, dockerGroup{key: "project:" + p.Dir, name: filepath.Base(p.Dir),
					kind: "Compose", dir: p.Dir, compose: project})
			}
		}
	}
	slices.SortFunc(result, func(a, b dockerGroup) int {
		order := func(g dockerGroup) int {
			switch {
			case g.problems > 0:
				return 0
			case g.kind == "Compose":
				return 1
			case g.kind == "kind":
				return 3
			default:
				return 2
			}
		}
		if n := cmp.Compare(order(a), order(b)); n != 0 {
			return n
		}
		return cmp.Compare(a.key, b.key)
	})
	return result
}

func composeProject(p projects.Project) (docker.ComposeProject, bool) {
	for _, f := range p.Files {
		switch f {
		case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
			return docker.ComposeProject{Dir: p.Dir, Files: filepath.Join(p.Dir, f)}, true
		}
	}
	return docker.ComposeProject{}, false
}

func (s *dockerContainersState) groupOpen(g dockerGroup) bool {
	if open, ok := s.open[g.key]; ok {
		return open
	}
	return g.problems > 0 || g.kind == "" || s.query != "" || s.filter != dockerAll
}

func (s *dockerContainersState) setGroup(key string, open bool) {
	if s.open == nil {
		s.open = make(map[string]bool)
	}
	s.open[key] = open
}

func (a *App) dockerContainersView(c *ui.Context) {
	s := &a.engine.containers
	if !s.loaded {
		if s.err != nil {
			toolNote(c, s.err.Error())
		} else if a.WatchDockerContainers != nil {
			tableSkeleton(c, locales.DockerContainersReading())
		}
		return
	}
	if s.err != nil {
		toolNote(c, locales.KubeClusterStale(s.err.Error()))
	}
	if s.statsErr != nil {
		ui.Text(c, locales.DockerStatsUnavailable()).FontSize(11.5).TextColor(c.Theme().TextMuted).Tooltip(s.statsErr.Error())
	}
	var counts [4]int
	for _, item := range s.items {
		for tab := range counts {
			if showDockerContainer(item, tab, "") {
				counts[tab]++
			}
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
		segments(c, "docker-filter", &s.filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[dockerAll])},
			{locales.KubePodsTrouble(), strconv.Itoa(counts[dockerProblems])},
			{locales.DockerContainersRunning(), strconv.Itoa(counts[dockerRunning])},
			{locales.DockerContainersStopped(), strconv.Itoa(counts[dockerStopped])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("docker-search"), &s.query).Label(locales.DockerContainerSearch()).Placeholder(locales.DockerContainerSearch()).Width(270).MinWidth(110).Shrink(1)
	})
	groups := a.dockerGroups(s.items, s.filter, strings.ToLower(strings.TrimSpace(s.query)))
	var ids []string
	running := 0
	for _, g := range groups {
		ids = append(ids, dockerContainerIDs(g.items)...)
		for _, item := range g.items {
			if item.Active() {
				running++
			}
		}
	}
	item, compact := s.selectedContainer()
	var detail func()
	tracks := dockerContainerTracks(c)
	headers := []string{locales.KubeClusterName(), locales.KubeClusterStatus(), locales.PortsTitle(), locales.DockerUptime(), "CPU", locales.KubeMetricsMemory(), ""}
	if compact {
		tracks = []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(99), ui.Fixed(24)}
		headers = []string{locales.KubeClusterName(), locales.KubeClusterStatus(), ""}
		detail = func() { a.dockerDetails(c, item) }
	}
	pal := Colors(c.Theme())
	workloadPanes(c.Key("docker-container-panes"), func() {
		workloadHead(c, tracks, func() {
			s.bulk.checkbox(c, "docker-select-all", locales.KubeBulkSelectAll(), ids)
		}, headers...)
	}, func() {
		for _, g := range groups {
			a.dockerGroupRows(c.Key(g.key), g, tracks, compact)
		}
		if len(groups) == 0 {
			ui.Text(c, locales.DockerContainersEmpty()).FontSize(13).TextColor(c.Theme().TextMuted).Padding(12, 16)
		}
		ui.Row(c).Gap(8).Padding(8, 14).AlignItems(ui.Center).Background(pal.Subtle).Shrink(0).Children(func() {
			ui.Text(c, locales.DockerContainersCount(len(ids), running)).FontSize(11.5).TextColor(c.Theme().TextMuted).Wrap().Grow(1).MinWidth(0)
			expand := slices.ContainsFunc(groups, func(g dockerGroup) bool { return len(g.items) > 0 && !s.groupOpen(g) })
			label := locales.DockerCollapseAll()
			if expand {
				label = locales.DockerExpandAll()
			}
			if len(ids) > 0 {
				ui.Button(c.Key("expand-all"), label).Height(24).Shrink(0).OnClick(func() {
					for _, g := range groups {
						s.setGroup(g.key, expand)
					}
				})
			}
		})
	}, item.ID, detail)
	a.dockerBulkBar(c, s)
}

func dockerContainerTracks(c *ui.Context) []ui.Track {
	if narrow(c) {
		return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(105), ui.Fixed(80), ui.Fixed(80), ui.Fixed(45), ui.Fixed(64), ui.Fixed(24)}
	}
	return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(152), ui.Fixed(122), ui.Fixed(98), ui.Fixed(62), ui.Fixed(72), ui.Fixed(24)}
}

func dockerContainerIDs(items []docker.Container) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (a *App) dockerGroupRows(c *ui.Context, g dockerGroup, tracks []ui.Track, compact bool) {
	t := c.Theme()
	pal := Colors(t)
	s := &a.engine.containers
	open := s.groupOpen(g)
	menu := func(m *ui.Menu) { a.dockerGroupMenu(m, g) }
	workloadSelectionRow(c, pal.Line).Background(pal.Subtle).Children(func() {
		ui.Column(c).Width(18).Shrink(0).Justify(ui.Center).Children(func() {
			s.bulk.checkbox(c, "docker-select-group", locales.KubeBulkSelect(g.name), dockerContainerIDs(g.items))
		})
		row := ui.ButtonBase(c.Key("toggle")).Grow(1).MinWidth(0).MinHeight(40).Gap(8).AlignItems(ui.Center).Justify(ui.Start).
			Label(g.name).Disabled(len(g.items) == 0).OnClick(func() { s.setGroup(g.key, !open) })
		row.ContextMenu(menu)
		row.Children(func() {
			icon := "chevron-right"
			if open && len(g.items) > 0 {
				icon = "chevron-down"
			}
			ui.Icon(c, Icon(icon)).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
			if g.kind != "" {
				aiTag(c, g.kind, pal.Muted, t.TextMuted).Shrink(0)
			}
			clipped(c, g.name).FontSize(13).FontWeight(600).MinWidth(0)
			switch {
			case g.problems > 0:
				aiTag(c, locales.DockerGroupProblems(g.problems), pal.DangerSoft, pal.DangerText).Shrink(0)
			case g.running > 0:
				aiTag(c, locales.DockerGroupRunning(g.running), pal.SuccessSoft, pal.SuccessText).Shrink(0)
			default:
				aiTag(c, locales.DockerContainersStopped(), pal.Muted, t.TextMuted).Shrink(0)
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if g.dir != "" {
				if !narrow(c) && !compact {
					link := ui.ButtonBase(c.Key("project-link")).Height(24).MaxWidth(205).MinWidth(0).Padding(0, 6).
						AlignItems(ui.Center).Radius(5).Label(locales.OpenProject()).Tooltip(g.dir).
						OnClick(func() { a.router.Push(projectPath(g.dir)) })
					if link.Hovered() {
						link.Background(pal.Muted)
					}
					link.Children(func() {
						clipped(c, tilde(g.dir, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).MinWidth(0)
					})
				}
				if g.compose.Name == "" && a.canPlanDocker() {
					a.dockerComposeButton(c.Key("compose-up"), g.compose, ops.DockerUp)
				}
			}
			if g.compose.Name != "" || g.dir != "" {
				iconButton(c.Key("group-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
			} else {
				ui.Box(c).Width(24)
			}
		})
	})
	if open {
		for _, item := range g.items {
			a.dockerContainerRow(c.Key(item.ID), item, tracks, compact)
		}
	}
}

func (a *App) dockerContainerRow(c *ui.Context, item docker.Container, tracks []ui.Track, compact bool) {
	t := c.Theme()
	pal := Colors(t)
	s := &a.engine.containers
	status, bg, fg := dockerContainerStatus(c, item)
	menu := func(m *ui.Menu) {
		if m.Item(locales.Details()).Chosen() {
			s.selectContainer(item.ID)
		}
		if a.StreamDockerLogs != nil && m.Item(locales.KubeLogsOpen()).Chosen() {
			s.selectContainer(item.ID)
			a.openDockerLogs(item.Name, []docker.LogTarget{{ID: item.ID}}, false)
		}
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, item.Name)
		}
		if a.canActOnDockerContainers() {
			m.Separator()
			for _, action := range dockerContainerActions {
				if action.Applies(item) && m.Item(action.Label()+"…").Chosen() {
					a.planDockerContainers([]docker.Container{item}, action)
				}
			}
		}
	}
	row := ui.Column(c).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu)
	if item.ID == s.detail.id {
		row.Background(pal.AccentSoft)
	} else if item.Problem() {
		row.Background(bg.Alpha(0.24))
	}
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(39).Children(func() {
			s.bulk.checkbox(c, "docker-select-container", locales.KubeBulkSelect(item.Name), []string{item.ID})
			ui.ButtonBase(c.Key("docker-open-container")).Padding(0, 0, 0, 23).MinWidth(0).MinHeight(24).
				AlignItems(ui.Center).Justify(ui.Start).Label(item.Name).Tooltip(item.Name).
				OnClick(func() { s.selectContainer(item.ID) }).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					clipped(c, item.Name).FontSize(12.5).Font(FontMono)
					if !compact && item.Image != "" {
						clipped(c, item.Image).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
			})
			workloadStatus(c, status, bg, fg)
			if compact {
				iconButton(c.Key("container-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
				return
			}
			port := dockerPortLabel(item.Ports)
			if item.ConflictPort() > 0 {
				port = strconv.Itoa(item.ConflictPort())
			}
			uptime := ""
			if item.Active() && !item.State.StartedAt.IsZero() {
				uptime = age(c, item.State.StartedAt)
			}
			clipped(c, cmp.Or(port, "—")).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(cmp.Or(item.Ports, port))
			for _, value := range []string{uptime, item.CPU, item.Memory} {
				clipped(c, cmp.Or(value, "—")).Font(FontMono).FontSize(11.5).MinWidth(0)
			}
			iconButton(c.Key("container-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
		})
		if why := dockerContainerProblem(item); !compact && why != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(7).Padding(0, 16, 8, 67).Children(func() {
				ui.Icon(c, Icon("triangle-alert")).Size(13, 13).Shrink(0).TextColor(fg)
				ui.Text(c, why).FontSize(11.5).TextColor(fg).Wrap().Grow(1).MinWidth(0)
				if port := item.ConflictPort(); port > 0 {
					ui.Button(c.Key("docker-open-port"), locales.DockerOpenPorts()).Height(24).Shrink(0).OnClick(func() {
						a.portsFilter = strconv.Itoa(port)
						a.router.Push(portsPage)
					})
				}
			})
		}
	})
}

// dockerPortLabel fits published mappings in the table. Docker may list
// the same mapping once for IPv4 and once for IPv6.
func dockerPortLabel(raw string) string {
	var mappings []string
	for item := range strings.SplitSeq(raw, ",") {
		host, container, ok := strings.Cut(strings.TrimSpace(item), "->")
		if !ok {
			continue
		}
		if i := strings.LastIndexByte(host, ':'); i >= 0 {
			host = host[i+1:]
		}
		mapping := host + " → " + strings.TrimSuffix(container, "/tcp")
		if !slices.Contains(mappings, mapping) {
			mappings = append(mappings, mapping)
		}
	}
	if len(mappings) == 0 {
		return raw
	}
	return strings.Join(mappings, ", ")
}

func dockerContainerStatus(c *ui.Context, item docker.Container) (string, ui.Color, ui.Color) {
	t := c.Theme()
	p := Colors(t)
	switch item.State.Status {
	case "restarting":
		return locales.DockerContainerRestarting(item.RestartCount), p.WarningSoft, p.WarningText
	case "running":
		if !item.Problem() {
			return locales.DockerContainerRunning(), p.SuccessSoft, p.SuccessText
		}
	case "paused":
		return locales.DockerContainerPaused(), p.WarningSoft, p.WarningText
	case "exited":
		if item.State.ExitCode != 0 {
			return locales.DockerContainerExit(item.State.ExitCode), p.DangerSoft, p.DangerText
		}
	case "created":
		if !item.Problem() {
			return locales.DockerContainerCreated(), p.Muted, t.TextMuted
		}
	}
	if item.Problem() {
		return locales.DockerContainerIssue(), p.DangerSoft, p.DangerText
	}
	return locales.DockerContainerStopped(), p.Muted, t.TextMuted
}

func dockerContainerProblem(item docker.Container) string {
	var reasons []string
	switch {
	case item.ConflictPort() > 0:
		reasons = append(reasons, locales.DockerPortConflict(strconv.Itoa(item.ConflictPort())))
	case item.State.Error != "":
		reasons = append(reasons, item.State.Error)
	}
	switch {
	case item.State.OOMKilled:
		reasons = append(reasons, locales.DockerContainerOOM())
	case (item.State.Status == "exited" || item.State.Status == "restarting") && item.State.ExitCode != 0:
		switch item.State.ExitCode {
		case 137:
			reasons = append(reasons, locales.DockerContainerExit137())
		case 127:
			reasons = append(reasons, locales.DockerContainerExit127())
		case 126:
			reasons = append(reasons, locales.DockerContainerExit126())
		case 1:
			reasons = append(reasons, locales.DockerContainerExit1())
		default:
			reasons = append(reasons, locales.DockerContainerExit(item.State.ExitCode))
		}
	}
	if item.State.Status == "restarting" {
		reasons = append(reasons, locales.DockerRestartLoop(item.RestartCount))
	}
	if item.State.Health != nil && item.State.Health.Status == "unhealthy" {
		reasons = append(reasons, locales.DockerContainerUnhealthy())
	}
	return strings.Join(reasons, " · ")
}

var dockerContainerActions = []ops.DockerAction{ops.DockerStart, ops.DockerStop, ops.DockerRestart, ops.DockerRemove}

func (a *App) canPlanDocker() bool { return a.BuildPlan != nil && !a.op.running() }

func (a *App) canActOnDockerContainers() bool {
	s := &a.engine.containers
	return a.canPlanDocker() && !s.planning && s.err == nil && a.engine.err == nil
}

func (a *App) dockerBulkBar(c *ui.Context, s *dockerContainersState) {
	var selected []docker.Container
	for _, item := range s.items {
		if s.bulk.selected[item.ID] {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return
	}
	fg, bg := ui.RGB(250, 249, 247), ui.RGB(31, 29, 26)
	ui.Row(c.Key("docker-bulk-bar")).Gap(5).Padding(6, 10).Radius(10).AlignItems(ui.Center).Wrap().Background(bg).Shrink(0).Children(func() {
		ui.Text(c, locales.DockerSelected(len(selected))).FontSize(12).TextColor(fg)
		for i, action := range dockerContainerActions {
			icon := []string{"play", "circle-x", "refresh-cw", "trash-2"}[i]
			disabled := !a.canActOnDockerContainers() || slices.ContainsFunc(selected, func(item docker.Container) bool { return !action.Applies(item) })
			bulkAction(c.Key(string(action)), icon, action.Label()+"…", fg, ui.Transparent, disabled, func() {
				a.planDockerContainers(selected, action)
			})
		}
		bulkAction(c.Key("clear"), "", locales.KubeBulkClear(), fg, ui.Transparent, false, s.bulk.clear)
	})
	c.OnShortcut(0, ui.KeyEscape, s.bulk.clear)
}

func (a *App) planDockerContainers(items []docker.Container, action ops.DockerAction) {
	if !a.canActOnDockerContainers() {
		return
	}
	target, issue := a.engineTarget(a.router.Query("context"))
	if issue != "" {
		return
	}
	a.buildDockerPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerContainers(target, env, items, action)
	})
}

func (a *App) dockerGroupMenu(m *ui.Menu, g dockerGroup) {
	if a.StreamDockerLogs != nil && g.compose.Name != "" && len(g.items) > 0 && m.Item(locales.DockerComposeLogs()).Chosen() {
		var targets []docker.LogTarget
		for _, item := range a.engine.containers.items {
			if item.Kind == "" && item.Compose.Name == g.compose.Name && item.Compose.Dir == g.compose.Dir {
				targets = append(targets, docker.LogTarget{ID: item.ID, Name: item.Name})
			}
		}
		a.openDockerLogs(g.name, targets, true)
	}
	if g.dir != "" && m.Item(locales.OpenProject()).Chosen() {
		a.router.Push(projectPath(g.dir))
	}
	if !a.canActOnDockerContainers() || g.compose.Files == "" {
		return
	}
	actions := dockerContainerActions
	if g.dir != "" {
		actions = append(slices.Clone(actions), ops.DockerUp)
		if g.compose.Name != "" {
			actions = append(actions, ops.DockerDown)
		} else {
			actions = []ops.DockerAction{ops.DockerUp}
		}
	}
	for _, action := range actions {
		if m.Item(action.Label() + "…").Chosen() {
			a.planDockerCompose(g.compose, action)
		}
	}
}

func (a *App) dockerComposeButton(c *ui.Context, project docker.ComposeProject, action ops.DockerAction) {
	ui.Button(c, action.Label()+"…").Height(24).OnClick(func() { a.planDockerCompose(project, action) })
}

func (a *App) planDockerCompose(project docker.ComposeProject, action ops.DockerAction) {
	if !a.canPlanDocker() || a.engine.containers.planning {
		return
	}
	name := a.router.Query("context")
	if a.router.Path() != dockerEnginePage {
		st, _ := a.toolState("docker")
		if st.Docker == nil || !a.dockerAnswers() {
			return
		}
		name = st.Docker.Selected
	}
	target, issue := a.engineTarget(name)
	if issue != "" {
		return
	}
	a.buildDockerPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerCompose(context.Background(), run.Exec{}, env, target, project, action)
	})
}

func (a *App) buildDockerPlan(build func(sysenv.Env, ops.Terminal) (ops.Plan, error)) {
	s := &a.engine.containers
	path, name, dir := a.router.Path(), a.router.Query("context"), a.router.Query("dir")
	s.planning = true
	revision := s.revision
	a.BuildPlan(build, func(p ops.Plan, err error) {
		if s.revision != revision {
			return
		}
		s.planning = false
		if a.router.Path() == path && a.router.Query("context") == name && a.router.Query("dir") == dir && a.canPlanDocker() {
			a.proposeOrSay(p, err)
		}
	})
}

func (a *App) projectDockerCard(c *ui.Context, p projects.Project) {
	project, ok := composeProject(p)
	if !ok || !a.canPlanDocker() || !a.dockerAnswers() {
		return
	}
	listCard(c, locales.DockerComposeTitle(), "", func() {
		ui.Row(c).Gap(8).Padding(10, 12).Wrap().Children(func() {
			a.dockerComposeButton(c.Key("project-compose-up"), project, ops.DockerUp)
			a.dockerComposeButton(c.Key("project-compose-down"), project, ops.DockerDown)
		})
	})
}
