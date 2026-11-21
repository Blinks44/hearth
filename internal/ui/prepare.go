package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// preparing belongs to one request. Workers publish only through BuildPlan's
// UI callback; replacing the request cancels it and rejects its stale result.
type preparing struct {
	dir    string
	open   bool
	cancel context.CancelFunc
}

// CancelPreparation ends a pending read when the window closes.
func (a *App) CancelPreparation() {
	if a.preparing != nil {
		a.preparing.cancel()
		a.preparing = nil
	}
}

func (a *App) prepareProject(dir string) {
	if a.op.running() {
		a.op.open = true
		return
	}
	if a.BuildPlan == nil || a.Snapshot == nil {
		return
	}
	a.CancelPreparation()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	request := &preparing{dir: dir, open: true, cancel: cancel}
	a.preparing = request
	// Capture immutable data now. No view state is read from the worker.
	snapshot := *a.Snapshot
	input := ops.ProjectInput{Snapshot: &snapshot, Releases: a.projectReleases(), Lines: a.nodeLines(), Setup: a.setupHere(), Title: toolTitle, Detect: a.DetectTool}
	var requestResult ops.ProjectPreparation
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		defer cancel()
		input.Project = projects.Read(dir)
		if len(input.Project.Errs) > 0 {
			return ops.Plan{}, input.Project.Errs[0]
		}
		if input.Detect != nil {
			var tools []string
			for _, req := range input.Project.Requirements {
				if !slices.Contains(tools, req.Tool) {
					tools = append(tools, req.Tool)
				}
			}
			// Never mutate the snapshot owned by the UI.
			snapshot.Tools = slices.Clone(snapshot.Tools)
			for _, tool := range tools {
				if ctx.Err() != nil {
					return ops.Plan{}, ctx.Err()
				}
				st, err := input.Detect(ctx, tool)
				st.Tool = tool
				if err != nil {
					st.Err = err.Error()
				}
				i := slices.IndexFunc(snapshot.Tools, func(s inventory.ToolState) bool { return s.Tool == tool })
				if i < 0 {
					snapshot.Tools = append(snapshot.Tools, st)
				} else {
					snapshot.Tools[i] = st
				}
				input.Setup.Have[tool] = len(st.Installations) > 0 && st.Err == ""
			}
		}
		result, err := ops.PrepareProject(input, env)
		if ctx.Err() != nil {
			return ops.Plan{}, ctx.Err()
		}
		// The result is delivered by the completion callback, before the dialog
		// becomes visible; no application state crosses the worker boundary.
		requestResult = result
		return result.Plan, err
	}, func(p ops.Plan, err error) {
		if a.preparing != request {
			return
		}
		a.preparing = nil
		if !request.open {
			return
		}
		if slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir }) < 0 {
			a.planErr = locales.PrepareMissingProject()
			return
		}
		if err != nil {
			// Keep the folder and retry actions available even when parsing fails.
			p = ops.Plan{Title: locales.PrepareTitle(filepath.Base(dir)), Why: locales.PrepareReadErrors()}
			a.Propose(p)
			a.op.project = &ops.ProjectPreparation{Project: projects.Project{Dir: dir}, Items: []ops.ProjectItem{{Title: locales.PrepareReadErrors(), Text: err.Error()}}}
			return
		}
		a.Propose(p)
		a.op.project = &requestResult
	})
}

func (a *App) projectReleases() map[string][]string {
	out := map[string][]string{}
	if a.NodeVersions != nil {
		for _, r := range a.NodeVersions.Releases {
			out["node"] = append(out["node"], r.Version)
		}
	}
	if a.GoVersions != nil {
		out["go"] = slices.Clone(a.GoVersions.Releases)
	}
	for _, c := range []struct {
		tool   string
		cycles *versions.Cycles
	}{{"python", a.PythonVersions}, {"php", a.PHPVersions}, {"ruby", a.RubyVersions}, {"java", a.JavaVersions}, {"rust", a.RustVersions}} {
		if c.cycles != nil {
			for _, l := range c.cycles.Lines {
				out[c.tool] = append(out[c.tool], l.Latest)
			}
		}
	}
	if a.BunLatest != nil {
		out["bun"] = []string{a.BunLatest.Version}
	}
	if a.JSVersions != nil {
		out["npm"] = []string{a.JSVersions.Npm.Version}
		out["pnpm"] = []string{a.JSVersions.Pnpm.Version}
		out["yarn"] = []string{a.JSVersions.Yarn.Version}
	}
	return out
}

func (a *App) preparationView(c *ui.Context) {
	request := a.preparing
	if request == nil {
		return
	}
	dialogBase(c.Key("preparing"), &request.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Background(c.Theme().Background)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.PrepareTitle(filepath.Base(request.dir))).FontSize(18).FontWeight(600)
			ui.Text(c, locales.PrepareChecking()).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Button(c.Key("prepare-cancel"), locales.Cancel()).OnClick(func() { request.open = false; a.CancelPreparation() })
		})
	})
	if !request.open {
		a.CancelPreparation()
	}
}

func (a *App) projectPreparationItems(c *ui.Context, p *ops.ProjectPreparation, before bool) {
	dialogSection(c, locales.PrepareItems())
	if before {
		ui.Text(c, locales.PrepareBefore()).FontSize(11.5).TextColor(c.Theme().TextMuted).Padding(4, 20, 8, 20)
	}
	if len(p.Items) == 0 {
		ui.Text(c, locales.PrepareNoRequirements()).FontSize(12).Padding(10, 20)
	}
	for i, item := range p.Items {
		ui.Column(c.Key("prepare-item:"+item.Tool+":"+strconv.Itoa(i))).Gap(5).Padding(8, 20, 12, 20).Children(func() {
			ui.Text(c, item.Title+withSpace(item.Version)).FontSize(13).FontWeight(500)
			ui.Text(c, ops.ProjectSources(p.Project, item.Tool)).FontSize(11.5).TextColor(c.Theme().TextMuted)
			color := c.Theme().TextMuted
			if item.Ready && !before {
				color = c.Theme().Success
			}
			ui.Text(c, item.Text).FontSize(12).TextColor(color)
			if len(item.Command) > 0 {
				ui.Text(c, ops.Command(item.Command)).Font(FontMono).FontSize(11.5)
			}
			if item.Config != "" {
				ui.Text(c, item.Config).Font(FontMono).FontSize(11.5)
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if len(item.Command) > 0 {
					command := ops.Command(item.Command)
					ui.Button(c.Key("prepare-copy:"+item.Tool), locales.PrepareCopy()).OnClick(func() { c.WriteClipboard(command); c.Toast(locales.PrepareCopied()) })
				}
				if item.Config != "" {
					ui.Button(c.Key("prepare-setting:"+item.Tool), locales.PrepareCopySetting()).OnClick(func() { c.WriteClipboard(item.Config); c.Toast(locales.PrepareSettingCopied()) })
				}
				if item.Tool != "" {
					ui.Button(c.Key("prepare-tool:"+item.Tool), locales.PrepareOpenTool()).OnClick(func() { a.op.open = false; a.router.Push(projectToolPath(item.Tool)) })
				}
				if a.Shell != nil {
					for _, file := range item.Files {
						ui.Button(c.Key("prepare-file:"+item.Tool+":"+file), file).OnClick(func() { a.openFile(c, filepath.Join(p.Project.Dir, file)) })
					}
				}
			})
			if item.Tool == "python" {
				ui.Text(c, locales.PreparePythonNote()).FontSize(11.5).TextColor(c.Theme().TextMuted)
				ui.Button(c.Key("prepare-python-guide"), locales.PrepareGuide()).OnClick(func() { c.OpenURL("https://docs.astral.sh/uv/guides/projects/") })
			}
		})
	}
}

func projectToolPath(tool string) string {
	switch tool {
	case "pnpm", "yarn", "npm":
		return "/tools/jspm"
	case "composer":
		return "/tools/php"
	case "uv", "pipx":
		return "/tools/python"
	case "bundler":
		return "/tools/ruby"
	case "maven", "gradle":
		return "/tools/java"
	case "git":
		return gitPage
	case "cpp":
		return cppPage
	case "docker":
		return "/tools/docker"
	case "kubectl", "helm":
		return kubePage
	}
	for _, l := range languages {
		if l.id == tool {
			return "/tools/" + tool
		}
	}
	return cliPage
}

func (a *App) projectPreparationButtons(c *ui.Context, op *operation) {
	dir := op.project.Project.Dir
	ui.Button(c.Key("prepare-close"), locales.OpClose()).Height(controlHeight).OnClick(func() { op.open = false })
	if a.Shell != nil {
		ui.Button(c.Key("prepare-folder"), locales.PrepareOpenFolder()).Height(controlHeight).OnClick(func() { a.openFile(c, dir) })
	}
	label := locales.PrepareAgain()
	if op.started && (op.state.Phase == ops.Failed || op.state.Phase == ops.Stopped) {
		label = locales.PrepareContinue()
	}
	ui.PrimaryButton(c.Key("prepare-again"), label).Height(controlHeight).Disabled(a.BuildPlan == nil).OnClick(func() { op.open = false; a.prepareProject(dir) })
}
