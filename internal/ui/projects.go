package ui

import (
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// adding is a folder chosen to add, as projects.Find read it, and which of
// the projects nested in it are to be added too.
type adding struct {
	root   projects.Project
	nested []projects.Project
	on     []bool
	// folders are the usual code folders the projects were found in, when
	// Hearth offers them by itself; nil for a folder the user chose.
	folders []string
	changed []time.Time // when each nested project last changed, with folders
}

// container reports a folder that only holds projects, as ~/code: no
// project files of its own, so it is not added, only what it holds.
func (add *adding) container() bool {
	return len(add.root.Files) == 0 && len(add.root.AI) == 0 && len(add.nested) > 0
}

// Offer shows the dialog adding the folder root.Dir, which projects.Find
// read, with the projects nested in it; the ones with versions of their
// own are chosen at first, all of them in a container.
func (a *App) Offer(root projects.Project, nested []projects.Project) {
	add := &adding{root: root, nested: nested, on: make([]bool, len(nested))}
	for i, p := range nested {
		add.on[i] = add.container() || len(p.Versions()) > 0
	}
	a.adding, a.addOpen = add, true
}

// OfferFound shows the dialog adding the projects found in the usual code
// folders on the first start; the ones changed lately are chosen.
func (a *App) OfferFound(folders []string, found []projects.Suggestion, now time.Time) {
	add := &adding{folders: folders}
	for _, s := range found {
		add.nested = append(add.nested, s.Project)
		add.changed = append(add.changed, s.Changed)
		add.on = append(add.on, now.Sub(s.Changed) < projects.StaleAfter)
	}
	a.adding, a.addOpen = add, true
}

// projectPath is the router's location of the project in dir: the
// folder goes in the query, as a path has slashes of its own.
func projectPath(dir string) string { return projectPage + "?dir=" + url.QueryEscape(dir) }

// projectPage is the path of a project's page, under the projects.
const projectPage = "/projects/project"

// projectList is the projects page: the projects added, by state, with a
// filter.
func (a *App) projectList(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.addButton(c) }})
		if len(a.Projects) == 0 {
			a.projectsEmpty(c)
		} else {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(14).Padding(22, 28, 24, 28).Children(func() {
					a.projectRows(c)
				})
			})
		}
		a.addDialog(c)
	})
}

// addButton asks for a folder to add, when the app can.
func (a *App) addButton(c *ui.Context) {
	if a.ChooseFolder == nil {
		return
	}
	ui.PrimaryButton(c, "").OnClick(a.ChooseFolder).Children(func() {
		ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(ui.Hex("#FFFFFF"))
		ui.Text(c, locales.AddFolder()).TextColor(ui.Hex("#FFFFFF")).SingleLine()
	})
}

// projectsEmpty is the page before any project is added.
func (a *App) projectsEmpty(c *ui.Context) {
	pal := Colors(c.Theme())
	ui.Column(c).Grow(1).Center().Gap(10).Padding(0, 28).Children(func() {
		ui.Icon(c, Icon("folder-git-2")).Size(28, 28).TextColor(pal.TextFaint)
		ui.Text(c, locales.NoProjectsYet()).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
		a.addButton(c)
	})
}

// projectRows are the tabs, the filter and the table of projects.
func (a *App) projectRows(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	lines := a.nodeLines()
	type row struct {
		p        projects.Project
		statuses []projects.Status
		problem  string
	}
	var rows []row
	attention := 0
	for _, p := range a.Projects {
		st := projects.Resolve(p, a.Snapshot, lines)
		r := row{p, st, projectProblem(st)}
		if r.problem != "" {
			attention++
		}
		if a.projectsTab == 1 && r.problem == "" {
			continue
		}
		if q := strings.ToLower(strings.TrimSpace(a.projectFilter)); q != "" &&
			!strings.Contains(strings.ToLower(p.Dir), q) {
			continue
		}
		rows = append(rows, r)
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		segments(c, "project-tabs", &a.projectsTab, []segment{
			{locales.TabAll(), strconv.Itoa(len(a.Projects))},
			{locales.TabAttention(), strconv.Itoa(attention)},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("project-filter"), &a.projectFilter).Bind(&a.filterField).Placeholder(locales.FilterProjects()).Width(200)
	})
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			head := func(title string) ui.Element {
				return ui.Text(c, title).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
			head(locales.ColumnProject()).Width(260).Shrink(0)
			head(locales.ColumnProjectTools()).Grow(1).MinWidth(0)
			head("AI").Width(90).Shrink(0)
			head(locales.ColumnState()).Width(170).Shrink(0)
			if a.RemoveProject != nil {
				ui.Box(c).Width(36).Shrink(0) // over the remove buttons
			}
		})
		if len(rows) == 0 {
			cardNote(c, locales.NoProjectsMatch())
		}
		for _, r := range rows {
			dir := r.p.Dir
			e := ui.ButtonBase(c.Key("project:"+dir)).Gap(12).Padding(10, 12, 10, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(filepath.Base(dir)).
				ContextMenu(func(m *ui.Menu) { a.projectItems(m, dir, true) })
			if e.Hovered() {
				e.Background(pal.Subtle)
			}
			e.Children(func() {
				ui.Row(c).Width(260).Shrink(0).Gap(10).AlignItems(ui.Center).Children(func() {
					folderTile(c, 28)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, filepath.Base(r.p.Dir)).FontSize(13.5).SingleLine()
						clipped(c, tilde(r.p.Dir, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
					})
				})
				ui.Row(c).Grow(1).MinWidth(0).Gap(6).Wrap().Children(func() {
					for _, st := range r.statuses {
						toolTag(c, st)
					}
				})
				ui.Row(c).Width(90).Shrink(0).Gap(4).Children(func() {
					for _, ag := range projectAgents(r.p) {
						ui.Box(c).Size(20, 20).Radius(10).Center().BorderWidth(1).BorderColor(pal.Line).
							Tooltip(ag.name).Children(func() {
							ui.Icon(c, Brand(ag.brand)).Size(12, 12).TextColor(t.TextMuted)
						})
					}
				})
				ui.Row(c).Width(146).Shrink(0).Children(func() { projectChip(c, r.p, r.statuses, r.problem) })
				if a.RemoveProject != nil {
					rm := ui.ButtonBase(c.Key("remove:"+dir)).Size(24, 24).Shrink(0).Radius(6).Center().
						Label(locales.RemoveFromList()).Tooltip(locales.RemoveFromList()).
						OnClick(func() { a.RemoveProject(dir) })
					color := pal.TextFaint
					if rm.Hovered() {
						rm.Background(pal.Muted)
						color = t.Danger
					}
					rm.Children(func() {
						ui.Icon(c, Icon("trash-2")).Size(14, 14).TextColor(color)
					})
				}
				ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(pal.TextFaint)
			})
			if e.Clicked() {
				a.router.Push(projectPath(r.p.Dir))
			}
		}
	})
	ui.Row(c).Gap(8).Padding(0, 4).Children(func() {
		ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
		ui.Text(c, locales.ProjectsNote()).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
	})
}

// toolTag is a tool a project needs, as "node 22", in a mono badge.
func toolTag(c *ui.Context, st projects.Status) {
	pal := Colors(c.Theme())
	text := st.Tool
	if w := statusWant(st); w != "" {
		text += " " + w
	}
	ui.Text(c, text).Font(FontMono).FontSize(11).TextColor(pal.MonoText).SingleLine().
		Padding(2, 6).Radius(5).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line)
}

// projectChip is a project's state: its first problem, files it could
// not read, no versions when its files name none, not checked when the
// scan checks none of its tools, or else ready.
func projectChip(c *ui.Context, p projects.Project, statuses []projects.Status, problem string) {
	t := c.Theme()
	pal := Colors(t)
	switch {
	case problem != "":
		chip(c, t.Warning, pal.WarningSoft, pal.WarningText, problem)
	case len(p.Errs) > 0:
		chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipReadFailed())
	case len(statuses) == 0:
		chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ChipNoVersions())
	case !anyChecked(statuses):
		chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ChipNotChecked())
	default:
		chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipReady())
	}
}

// project is the page of the project in dir: what it needs against what
// is installed, its AI settings and its files.
func (a *App) project(c *ui.Context, dir string) {
	i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir })
	name := filepath.Base(dir)
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, name, projectsPage.title()+" · "+tilde(dir, a.Home), headerExtras{
			back: func() { a.router.Push(projectsPage.path) },
			before: func() {
				if i >= 0 {
					a.projectMenu(c, dir)
				}
			},
		})
		// The header's menu may have just taken the project off the list.
		i = slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir })
		if i < 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, locales.ProjectNotAdded()).TextColor(c.Theme().TextMuted)
			})
			return
		}
		p := a.Projects[i]
		statuses := projects.Resolve(p, a.Snapshot, a.nodeLines())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.projectHero(c, p, statuses)
				if text := problemBanner(statuses); text != "" {
					ui.Row(c).Gap(10).Padding(10, 12).Radius(10).AlignItems(ui.Center).
						Background(Colors(c.Theme()).WarningSoft).Children(func() {
						ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
						ui.Text(c, text).FontSize(12).TextColor(Colors(c.Theme()).WarningText).Grow(1).MinWidth(0)
						for _, st := range statuses {
							if ch, ok := st.Unmet(); ok {
								if plan, ok := a.projectInstall(st); ok {
									// Dark, as in the design: the page's one accent
									// stays for the header's action.
									ui.Button(c.Key("banner-install"), locales.InstallEllipsis(toolTitle(st.Tool)+withSpace(ch.Want))).
										Background(c.Theme().Text).TextColor(c.Theme().Background).BorderColor(c.Theme().Text).
										OnClick(func() { a.Propose(plan) })
								}
								break
							}
						}
					})
				}
				if len(statuses) > 0 {
					a.requirementTable(c, statuses)
				}
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { aiCard(c, p) })
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { filesCard(c, p) })
				})
			})
		})
	})
}

// projectMenu is the project's "…": show its folder, and remove it from
// the list, which leaves the folder be.
func (a *App) projectMenu(c *ui.Context, dir string) {
	ui.Button(c.Key("project-menu"), "").Size(28, 26).Padding(0).Label(locales.MoreActions()).Menu(func(m *ui.Menu) {
		a.pinItems(m, dir)
		a.projectItems(m, dir, false)
	}).Children(func() {
		ui.Icon(c, Icon("ellipsis")).Size(14, 14).TextColor(c.Theme().TextMuted)
	})
}

// projectItems are the actions on the project in dir, for its menus:
// open its page when open, show its folder, and remove it from the list,
// which leaves the folder be and goes back to the projects.
func (a *App) projectItems(m *ui.Menu, dir string, open bool) {
	if open && m.Item(locales.OpenProject()).Chosen() {
		a.router.Push(projectPath(dir))
	}
	if a.Shell != nil && m.Item(showInFolder()).Chosen() {
		a.Shell.ShowItemInFolder(dir)
	}
	if a.RemoveProject == nil {
		return
	}
	m.Separator()
	if m.Item(locales.RemoveFromList()).Chosen() {
		a.RemoveProject(dir)
		if a.router.Path() != projectsPage.path {
			a.router.Replace(projectsPage.path)
		}
	}
}

// pinItems offer to pin, in the project in dir, the versions its terminal
// runs of Node, pnpm and Yarn, where its files do not pin them already.
func (a *App) pinItems(m *ui.Menu, dir string) {
	i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir })
	if i < 0 {
		return
	}
	p := a.Projects[i]
	node, _ := a.toolState("node")
	added := false
	for _, st := range projects.Resolve(p, a.Snapshot, a.nodeLines()) {
		if st.Terminal == nil || st.Downloads || st.Terminal.Version == "" {
			continue
		}
		t := ops.PinTarget{Tool: st.Tool, Title: toolTitle(st.Tool), Version: st.Terminal.Version, Manager: a.installManager(node)}
		plan, ok := ops.PinPlan(p, t)
		if !ok {
			continue
		}
		added = true
		if m.Item(locales.PinMenu(t.Title, t.Version)).Chosen() {
			a.Propose(plan)
		}
	}
	if added {
		m.Separator()
	}
}

// projectInstall is the plan that installs the Node version a project
// needs and does not have: the newest release its file asks for, the
// recommended one when it names none, through the manager in use.
func (a *App) projectInstall(st projects.Status) (ops.Plan, bool) {
	ch, ok := st.Unmet()
	nv := a.NodeVersions
	if !ok || ch.Installed != nil || st.Tool != "node" || nv == nil {
		return ops.Plan{}, false
	}
	var version string
	if ch.Want == "" {
		r, ok := nv.Recommended()
		if !ok {
			return ops.Plan{}, false
		}
		version = r.Version
	} else {
		releases := make([]string, len(nv.Releases))
		for i, r := range nv.Releases {
			releases[i] = r.Version
		}
		if version, ok = projects.Pick("node", ch.Want, releases, nv.Lines); !ok {
			return ops.Plan{}, false
		}
	}
	node, _ := a.toolState("node")
	return a.versionPlan(ops.VersionInstall, "node", a.installManager(node), version, versionText(version), "")
}

// projectHero is the folder, its name and what it needs, and on the right
// the cards of where it stands: needed, ready, AI.
func (a *App) projectHero(c *ui.Context, p projects.Project, statuses []projects.Status) {
	t := c.Theme()
	pal := Colors(t)
	var needs, files []string
	ready := 0
	for _, st := range statuses {
		needs = append(needs, toolTitle(st.Tool)+withSpace(statusWant(st)))
		if st.Checked && statusProblem(st) == "" {
			ready++
		}
	}
	for _, f := range p.Files {
		if len(files) < 2 {
			files = append(files, f)
		}
	}
	caption := tilde(p.Dir, a.Home)
	if len(needs) > 0 {
		caption += " · " + strings.Join(needs, ", ")
	}
	headRow(c, func() {
		folderTile(c, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, filepath.Base(p.Dir)).FontSize(22).FontWeight(600).SingleLine()
			ui.Text(c, caption).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
	}, func() {
		stateCard(c, pal.TextFaint, locales.Needed(), locales.ToolsCount(len(statuses)), strings.Join(files, " · "))
		problem := projectProblem(statuses)
		dot, readyCaption := t.Success, locales.ChipReady()
		if problem != "" {
			dot, readyCaption = t.Warning, problem
		}
		stateCard(c, dot, locales.Ready(), locales.ReadyOf(ready, len(statuses)), readyCaption)
		var names []string
		for _, ag := range projectAgents(p) {
			names = append(names, ag.name)
		}
		value, aiCaption := "—", locales.NoAISettings()
		if len(names) > 0 {
			value, aiCaption = strings.Join(names, ", "), strings.Join(p.AI, " · ")
		}
		stateCard(c, Accent, "AI", value, aiCaption)
	})
}

// requirementTable is a row per tool: what the project needs and from
// which file, what is installed, and what the terminal runs in its folder.
func (a *App) requirementTable(c *ui.Context, statuses []projects.Status) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			for i, h := range []string{locales.ColumnTool(), locales.ColumnNeeded(), locales.ColumnInstalled(), locales.ColumnRunsHere()} {
				ui.Text(c, h).FontSize(11.5).TextColor(pal.TextFaint).Grow(columnGrow[i]).Basis(0).MinWidth(0).SingleLine()
			}
			ui.Box(c).Width(110).Shrink(0)
		})
		for _, st := range statuses {
			first := st.Checks[0]
			ui.Row(c).Gap(12).Padding(12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Row(c).Grow(columnGrow[0]).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, toolIcon(st.Tool)).Size(16, 16).TextColor(t.TextMuted)
					ui.Text(c, toolTitle(st.Tool)).FontSize(13.5).SingleLine()
				})
				want := statusWant(st)
				if want == "" {
					want = locales.AnyVersion()
				}
				cell(c, columnGrow[1], want, checkSource(first), pal.TextFaint)
				installed, caption, warn := installedCell(st)
				color := pal.TextFaint
				if warn {
					color = pal.WarningText
				}
				cell(c, columnGrow[2], installed, caption, color)
				run, runCaption := "—", locales.NotInPath()
				switch {
				case !st.Checked:
					runCaption = ""
				case st.Downloads:
					run, runCaption = copyVersion(*st.Terminal), locales.CorepackDownloads()
				case st.Terminal != nil:
					run, runCaption = copyVersion(*st.Terminal), sourceName(st.Terminal.Source)
				}
				cell(c, columnGrow[3], run, runCaption, pal.TextFaint)
				ui.Row(c).Width(110).Shrink(0).Justify(ui.End).Children(func() {
					plan, install := a.projectInstall(st)
					switch {
					case install:
						ui.Button(c.Key("install:"+st.Tool), "").Label(plan.Title).OnClick(func() { a.Propose(plan) }).Children(func() {
							ui.Icon(c, Icon("download")).Size(13, 13).TextColor(t.TextMuted)
							ui.Text(c, locales.Install()).SingleLine()
						})
					case !st.Checked:
						chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ChipNotChecked())
					case statusProblem(st) == "":
						chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipReady())
					default:
						chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipNotReady())
					}
				})
			})
		}
	})
}

// columnGrow are the shares of the requirement table's columns: the
// installed versions take the most.
var columnGrow = [4]float32{0.8, 1, 1.4, 1}

// cell is a table cell taking grow of the row: a value in mono over a
// caption in color.
func cell(c *ui.Context, grow float32, value, caption string, color ui.Color) {
	ui.Column(c).Grow(grow).Basis(0).MinWidth(0).Gap(2).Children(func() {
		ui.Text(c, value).Font(FontMono).FontSize(13).SingleLine()
		if caption != "" {
			ui.Text(c, caption).FontSize(11.5).TextColor(color).SingleLine()
		}
	})
}

// installedCell is what is installed of a tool: its versions, and under
// them whether the one needed is among them, a warning when it is not.
func installedCell(st projects.Status) (value, caption string, warn bool) {
	if !st.Checked {
		return "—", locales.NotCheckedYet(toolTitle(st.Tool)), false
	}
	vs := installedVersions(st)
	if len(vs) == 0 && st.Downloads {
		return "—", locales.NotInCorepackCache(), false
	}
	if len(vs) == 0 {
		return "—", locales.ToolNotInstalled(toolTitle(st.Tool)), true
	}
	for _, ch := range st.Checks {
		if ch.Err == "" && ch.Installed == nil {
			return strings.Join(vs, " · "), locales.NeededMissing(ch.Want), true
		}
	}
	return strings.Join(vs, " · "), "", false
}

// installedVersions are the versions of a tool the scan found, each once,
// oldest first.
func installedVersions(st projects.Status) []string {
	var vs []string
	for _, in := range st.Installations {
		if in.Version != "" && !slices.Contains(vs, in.Version) {
			vs = append(vs, in.Version)
		}
	}
	slices.SortFunc(vs, func(a, b string) int { return semver.Compare("v"+a, "v"+b) })
	return vs
}

// aiCard lists the project's AI settings: instructions, settings, skills,
// MCP servers.
func aiCard(c *ui.Context, p projects.Project) {
	listCard(c, locales.ProjectAI(), "", func() {
		if len(p.AI) == 0 {
			cardNote(c, locales.NoAISettings())
		}
		for _, f := range p.AI {
			listRow(c, f, false, aiFileCaption(f), func() {})
		}
	})
}

// filesCard lists the project files the versions come from, and the ones
// that could not be read.
func filesCard(c *ui.Context, p projects.Project) {
	pal := Colors(c.Theme())
	listCard(c, locales.ProjectFiles(), locales.WhereVersionsFrom(), func() {
		if len(p.Files) == 0 && len(p.Errs) == 0 {
			cardNote(c, locales.NoProjectFiles())
		}
		for _, f := range p.Files {
			fileRow(c, p, f)
		}
		for _, err := range p.Errs {
			ui.Text(c, err.Error()).FontSize(12).TextColor(pal.WarningText).Padding(9, 14).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
		}
	})
}

// addDialog shows the folder chosen to add: what was found in it, and the
// projects nested in it, each to add on its own.
func (a *App) addDialog(c *ui.Context) {
	add := a.adding
	if add == nil {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	dialogBase(c, &a.addOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(520).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 14, 20).Children(func() {
				title, note, where := locales.AddProjectTitle(), locales.AddProjectNote(), tilde(add.root.Dir, a.Home)
				if add.folders != nil {
					title, note = locales.FoundProjectsTitle(), locales.FoundProjectsNote()
					dirs := make([]string, len(add.folders))
					for i, d := range add.folders {
						dirs[i] = tilde(d, a.Home)
					}
					where = strings.Join(dirs, " · ")
				}
				ui.Text(c, title).FontSize(18).FontWeight(600)
				ui.Text(c, note).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).Gap(8).Padding(6, 10).Radius(7).AlignItems(ui.Center).
						BorderWidth(1).BorderColor(pal.ControlBorder).Children(func() {
						ui.Icon(c, Icon("folder-open")).Size(14, 14).TextColor(t.TextMuted)
						clipped(c, where).FontSize(12.5)
					})
					if a.ChooseFolder != nil {
						ui.Button(c, locales.OtherFolder()).OnClick(a.ChooseFolder)
					}
				})
			})
			ui.Scroll(c).MaxHeight(380).Children(func() {
				if add.folders != nil {
					a.foundRows(c, add)
					return
				}
				dialogSection(c, locales.FoundInFolder())
				if len(add.root.Files) == 0 && len(add.root.AI) == 0 {
					cardNote(c, locales.NothingFound())
				}
				for _, f := range add.root.Files {
					fileRow(c, add.root, f)
				}
				if len(add.root.AI) > 0 {
					listRow(c, strings.Join(add.root.AI, " · "), false, locales.ProjectAI(), func() {})
				}
				if len(add.nested) > 0 {
					dialogSection(c, locales.NestedProjects())
				}
				for i, p := range add.nested {
					rel, err := filepath.Rel(add.root.Dir, p.Dir)
					if err != nil {
						rel = p.Dir
					}
					caption := locales.SameVersions()
					if add.container() {
						caption = strings.Join(p.Files, " · ")
					}
					if v := p.Versions(); len(v) > 0 {
						caption = locales.OwnVersion(v[0].File, toolTitle(v[0].Tool)+" "+v[0].Want)
					}
					listRow(c, filepath.ToSlash(rel), false, caption, func() {
						toggle(c, "nested:"+p.Dir, &add.on[i]).Label(filepath.ToSlash(rel))
					})
				}
			})
			count := 1
			if add.container() {
				count = 0
			}
			for _, on := range add.on {
				if on {
					count++
				}
			}
			ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				cancel := locales.Cancel()
				if add.folders != nil {
					cancel = locales.NotNow()
				}
				ui.Button(c, cancel).OnClick(func() { a.addOpen = false })
				ui.PrimaryButton(c, locales.AddProjects(count)).Disabled(count == 0).OnClick(func() {
					var dirs []string
					if !add.container() {
						dirs = append(dirs, add.root.Dir)
					}
					for i, p := range add.nested {
						if add.on[i] {
							dirs = append(dirs, p.Dir)
						}
					}
					if a.AddProjects != nil {
						a.AddProjects(dirs)
					}
					a.addOpen = false
				})
			})
		})
	})
	if !a.addOpen {
		a.adding = nil
	}
}

// foundRows are the projects Hearth found by itself, by folder, each with
// a switch; the ones long unchanged say so.
func (a *App) foundRows(c *ui.Context, add *adding) {
	for _, folder := range add.folders {
		var rows []int
		for i, p := range add.nested {
			if strings.HasPrefix(p.Dir, folder+string(filepath.Separator)) {
				rows = append(rows, i)
			}
		}
		if len(rows) == 0 {
			continue
		}
		dialogSection(c, locales.FoundIn(tilde(folder, a.Home), len(rows)))
		for _, i := range rows {
			p := add.nested[i]
			rel, _ := filepath.Rel(folder, p.Dir)
			caption := strings.Join(p.Files, " · ")
			if !add.on[i] && !add.changed[i].IsZero() {
				caption = locales.ProjectUnchanged()
			}
			listRow(c, filepath.ToSlash(rel), false, caption, func() {
				toggle(c, "found:"+p.Dir, &add.on[i]).Label(filepath.ToSlash(rel))
			})
		}
	}
}

func dialogSection(c *ui.Context, title string) {
	pal := Colors(c.Theme())
	ui.Text(c, title).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Padding(14, 20, 6, 20).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).SingleLine()
}

// fileRow is a project file: its name, the fields versions come from, and
// on the right what it asks, as "node 22 · pnpm 10.4.1".
func fileRow(c *ui.Context, p projects.Project, file string) {
	var keys, wants []string
	for _, r := range p.Requirements {
		if r.File != file {
			continue
		}
		if r.Key != "" {
			keys = append(keys, r.Key)
		}
		switch {
		case r.Want != "":
			wants = append(wants, r.Tool+" "+r.Want)
		case !slices.Contains(wants, r.Tool):
			wants = append(wants, r.Tool)
		}
	}
	listRow(c, file, false, strings.Join(keys, " · "), func() {
		ui.Text(c, strings.Join(wants, " · ")).Font(FontMono).FontSize(11.5).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	})
}

// nodeLines are Node's release lines, which lts/* needs; nil until the
// version list comes.
func (a *App) nodeLines() map[int]versions.Line {
	if a.NodeVersions == nil {
		return nil
	}
	return a.NodeVersions.Lines
}

// statusWant is the version a project wants of a tool, by its first file
// that names one; "" when none does.
func statusWant(st projects.Status) string {
	for _, ch := range st.Checks {
		if ch.Want != "" {
			return ch.Want
		}
	}
	return ""
}

// checkSource is where a requirement comes from, as "package.json ·
// packageManager".
func checkSource(ch projects.Check) string {
	if ch.Key == "" || ch.File == ".tool-versions" {
		return ch.File
	}
	return ch.File + " · " + ch.Key
}

// statusProblem is what keeps a checked tool from being ready in the
// project: the needed version is not installed, or the terminal runs
// another; "" when nothing does.
func statusProblem(st projects.Status) string {
	ch, ok := st.Unmet()
	switch {
	case !ok:
		return ""
	case ch.Installed == nil:
		return locales.ProblemMissing(toolTitle(st.Tool) + withSpace(ch.Want))
	}
	return locales.ProblemTerminal(toolTitle(st.Tool) + withSpace(ch.Want))
}

// projectProblem is a project's first problem, "" when it has none.
func projectProblem(statuses []projects.Status) string {
	for _, st := range statuses {
		if p := statusProblem(st); p != "" {
			return p
		}
	}
	return ""
}

// problemBanner explains the project's first problem: what it needs and
// from which file, what is installed and what the terminal runs.
func problemBanner(statuses []projects.Status) string {
	for _, st := range statuses {
		ch, ok := st.Unmet()
		if !ok {
			continue
		}
		vs := installedVersions(st)
		installed := locales.NothingInstalled()
		if len(vs) > 0 {
			installed = strings.Join(vs, ", ")
		}
		run := "—"
		if st.Terminal != nil {
			run = copyVersion(*st.Terminal)
		}
		want := ch.Want
		if want == "" {
			want = locales.AnyVersion()
		}
		return locales.ProblemBanner(toolTitle(st.Tool), want, ch.File, installed, run)
	}
	return ""
}

// anyChecked reports that the scan checked a tool of the project: until
// then the project cannot be called ready.
func anyChecked(statuses []projects.Status) bool {
	for _, st := range statuses {
		if st.Checked {
			return true
		}
	}
	return false
}

func withSpace(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

// agent is an AI agent a project has settings for.
type agent struct{ name, brand string }

// projectAgents are the agents whose files the project has: Claude Code
// for CLAUDE.md, .claude and .mcp.json; Codex and others for AGENTS.md;
// Cursor for .cursor.
func projectAgents(p projects.Project) []agent {
	var out []agent
	add := func(a agent) {
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	for _, f := range p.AI {
		switch f {
		case "CLAUDE.md", ".claude", ".mcp.json":
			add(agent{"Claude Code", "claude"})
		case "AGENTS.md", ".agents":
			add(agent{"Codex", "openai"})
		case ".cursor":
			add(agent{"Cursor", "cursor"})
		}
	}
	return out
}

// aiFileCaption says what an AI file of a project is.
func aiFileCaption(f string) string {
	switch f {
	case "CLAUDE.md":
		return locales.AiInstructions("Claude Code")
	case ".claude":
		return locales.AiSettings("Claude Code")
	case ".mcp.json":
		return locales.AiMcp()
	case "AGENTS.md":
		return locales.AiInstructions("Codex, Copilot, Gemini")
	case ".agents":
		return locales.AiSkills()
	case ".cursor":
		return locales.AiSettings("Cursor")
	}
	return ""
}

// toolTitles are the names of tools projects need, as they are written.
var toolTitles = map[string]string{
	"node": "Node", "npm": "npm", "pnpm": "pnpm", "yarn": "Yarn", "bun": "Bun", "deno": "Deno",
	"python": "Python", "uv": "uv", "rust": "Rust", "go": "Go", "java": "Java", "ruby": "Ruby",
	"php": "PHP", "composer": "Composer", "dotnet": ".NET", "docker": "Docker", "git": "Git", "cpp": "C/C++",
}

func toolTitle(id string) string {
	if n, ok := toolTitles[id]; ok {
		return n
	}
	return id
}

// toolIcon is a tool's logo, or a package for a tool without one.
func toolIcon(id string) *ui.SVG {
	for _, l := range languages {
		if l.id == id {
			return mustIcon(l.icon)
		}
	}
	switch id {
	case "npm", "pnpm", "docker", "git":
		return Brand(id)
	}
	return Icon("package")
}

// folderTile is a project's folder icon on a tile of size.
func folderTile(c *ui.Context, size float32) {
	pal := Colors(c.Theme())
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Icon(c, Icon("folder-git-2")).Size(size/2, size/2).TextColor(c.Theme().TextMuted)
	})
}

// segment is a tab of segments: its label and a count after it.
type segment struct{ label, count string }

// segments are tabs switching *tab, as the Node.js page's.
func segments(c *ui.Context, key string, tab *int, segs []segment) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c).Children(func() {
		seg := ui.SegmentedBase(c.Key(key), tab, len(segs))
		seg.Track.Gap(2).Padding(2).Radius(8).Background(pal.Muted).Children(func() {
			for i, s := range segs {
				e := seg.Segment(i).Gap(5).Padding(5, 12).Radius(6).AlignItems(ui.Center).Label(s.label)
				color := t.TextMuted
				if i == *tab {
					e.Background(t.Background).Shadow(0, 1, 2, 0, t.Text.Alpha(0.1))
					color = t.Text
				}
				e.Children(func() {
					ui.Text(c, s.label).FontSize(12.5).TextColor(color).SingleLine()
					if s.count != "" {
						ui.Text(c, s.count).FontSize(11.5).TextColor(pal.TextFaint)
					}
				})
			}
		})
	})
}
