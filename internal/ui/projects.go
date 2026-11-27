package ui

import (
	"cmp"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
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
// folders; the ones changed lately are chosen. Projects already added are
// skipped, and an empty result shows a toast explaining why.
func (a *App) OfferFound(folders []string, found []projects.Suggestion, now time.Time) {
	if len(folders) == 0 {
		a.notice = locales.FoundNoFolders()
		return
	}
	dirs := make([]string, len(folders))
	for i, d := range folders {
		dirs[i] = tilde(d, a.Home)
	}
	where := strings.Join(dirs, ", ")
	if len(found) == 0 {
		a.notice = locales.FoundNothingNew(where)
		return
	}
	add := &adding{folders: folders}
	for _, s := range found {
		if slices.ContainsFunc(a.Projects, func(p projects.Project) bool { return p.Dir == s.Project.Dir }) {
			continue
		}
		add.nested = append(add.nested, s.Project)
		add.changed = append(add.changed, s.Changed)
		add.on = append(add.on, now.Sub(s.Changed) < projects.StaleAfter)
	}
	if len(add.nested) == 0 {
		a.notice = locales.FoundAllAdded(where)
		return
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
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				a.cloneProjectButton(c)
				a.findProjectsButton(c)
				a.addButton(c)
			})
		}})
		if len(a.Projects) == 0 {
			a.projectsEmpty(c)
		} else {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(14).Padding(22, 28, 24, 28).Children(func() {
					a.projectRows(c)
				})
			})
		}
	})
}

// addButton asks for a folder to add, when the app can.
func (a *App) addButton(c *ui.Context) {
	if a.ChooseFolder == nil {
		return
	}
	ui.PrimaryButton(c, "").Label(locales.AddFolder()).Height(controlHeight).OnClick(a.ChooseFolder).Children(func() {
		ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(ui.Hex("#FFFFFF"))
		ui.Text(c, locales.AddFolder()).TextColor(ui.Hex("#FFFFFF")).SingleLine()
	})
}

// findProjectsButton scans the usual code folders for projects, when the app can.
func (a *App) findProjectsButton(c *ui.Context) {
	if a.FindProjects == nil {
		return
	}
	ui.Button(c.Key("find-projects"), "").Label(locales.FindProjects()).Height(controlHeight).OnClick(a.FindProjects).Children(func() {
		ui.Icon(c, Icon("search")).Size(14, 14).TextColor(c.Theme().TextMuted)
		ui.Text(c, locales.FindProjects()).SingleLine()
	})
}

// projectsEmpty is the page before any project is added.
func (a *App) projectsEmpty(c *ui.Context) {
	pal := Colors(c.Theme())
	ui.Column(c).Grow(1).Center().Gap(10).Padding(0, 28).Children(func() {
		ui.Icon(c, Icon("folder-git-2")).Size(28, 28).TextColor(pal.TextFaint)
		ui.Text(c, locales.NoProjectsYet()).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			a.addButton(c)
			a.cloneProjectButton(c)
			a.findProjectsButton(c)
		})
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
		needsAttention := r.problem != "" || len(p.Errs) > 0
		if needsAttention {
			attention++
		}
		if a.projectsTab == 1 && !needsAttention {
			continue
		}
		if q := strings.ToLower(strings.TrimSpace(a.projectFilter)); q != "" && !projectMatches(p, st, q) {
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
		ui.SearchField(c.Key("project-filter"), &a.projectFilter).Bind(&a.filterField).Placeholder(locales.FilterProjects()).Height(controlHeight).Width(200)
	})
	projW, aiW, stateW, actionsW := float32(260), float32(90), float32(160), float32(98)
	if narrow(c) {
		projW, aiW, stateW = 170, 68, 128
	}
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			head := func(title string) ui.Element {
				return ui.Text(c, title).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
			head(locales.ColumnProject()).Width(projW).Shrink(0)
			head(locales.ColumnProjectTools()).Grow(1).MinWidth(0)
			head("AI").Width(aiW).Shrink(0)
			head(locales.ColumnState()).Width(stateW).Shrink(0)
			ui.Box(c).Width(actionsW).Shrink(0) // over the quick actions and chevron
		})
		if len(rows) == 0 {
			cardNote(c, locales.NoProjectsMatch())
		}
		for _, r := range rows {
			dir := r.p.Dir
			menu := func(m *ui.Menu) { a.projectItems(m, dir, true) }
			e := ui.ButtonBase(c.Key("project:"+dir)).Gap(12).Padding(10, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(filepath.Base(dir)).
				ContextMenu(menu).
				Transition(colorFade)
			if e.Hovered() {
				e.Background(pal.Subtle)
			}
			e.Children(func() {
				ui.Row(c).Width(projW).Shrink(0).Gap(10).AlignItems(ui.Center).Children(func() {
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
				ui.Row(c).Width(aiW).Shrink(0).Gap(4).Wrap().Children(func() {
					for _, id := range projectAgents(r.p) {
						ui.Box(c).Size(20, 20).Radius(10).Center().BorderWidth(1).BorderColor(pal.Line).
							Tooltip(agentTitle(id)).Children(func() {
							agentLogo(c, id, 12)
						})
					}
				})
				ui.Row(c).Width(stateW).Shrink(0).Children(func() { projectChip(c, r.p, r.statuses, r.problem) })
				ui.Row(c).Width(actionsW).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
						if a.Shell != nil {
							iconButton(c.Key("folder:"+dir), "folder-open", showInFolder()).
								OnClick(func() { a.Shell.ShowItemInFolder(dir) })
						}
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
						iconButton(c.Key("menu:"+dir), "ellipsis", locales.MoreActions()).Menu(menu)
					})
					ui.Icon(c, Icon("chevron-right")).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
				})
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
// is installed, its Git status, services, AI settings, disk, and files.
func (a *App) project(c *ui.Context, dir string) {
	if a.projectTabDir != dir {
		a.projectTab = 0
		a.projectTabDir = dir
		a.scriptLogsKey = ""
	}
	i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir })
	var statuses []projects.Status
	needsPrepare := false
	if i >= 0 {
		if run := a.activeProjectScriptLogs(dir); run != nil {
			a.projectScriptLogsView(c, a.Projects[i], run)
			return
		}
		statuses = projects.Resolve(a.Projects[i], a.Snapshot, a.nodeLines())
		needsPrepare = projectNeedsPrepare(a.Projects[i], statuses)
	}
	name := filepath.Base(dir)
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, name, projectsPage.title()+" · "+tilde(dir, a.Home), headerExtras{
			back: func() { a.router.Push(projectsPage.path) },
			after: func() {
				if i >= 0 && needsPrepare {
					ui.PrimaryButton(c.Key("prepare-project"), locales.PrepareButton()).Height(controlHeight).Disabled(a.Snapshot == nil || a.BuildPlan == nil || a.preparing != nil).OnClick(func() { a.prepareProject(dir) })
				}
			},
			before: func() {
				if i >= 0 {
					a.projectHeaderActions(c, a.Projects[i])
					a.projectMenu(c, dir, !needsPrepare)
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
		a.kubeLinkView(c)
		a.kubeApplyView(c)
		a.kindDialog(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.projectHero(c, p, statuses)
				a.projectTabs(c, p)
				switch a.projectTab {
				case 1:
					a.projectServicesTab(c, p)
				case 2:
					a.projectAITab(c, p)
				case 3:
					a.projectFilesTab(c, p)
				default:
					if banner := problemBanner(statuses); banner.Title != "" {
						warningBanner(c, banner.Title, banner.Meta, func() {
							unmetCount := 0
							var firstUnmet projects.Status
							var firstCheck projects.Check
							for _, st := range statuses {
								if ch, ok := st.Unmet(); ok {
									if unmetCount == 0 {
										firstUnmet, firstCheck = st, ch
									}
									unmetCount++
								} else if st.EngineDown {
									unmetCount++
								}
							}
							if unmetCount == 1 && firstUnmet.Tool != "" {
								if _, run, ok := a.projectInstallAction(firstUnmet, a.proposeOrSay); ok {
									// Dark, as in the design: the page's one accent
									// stays for the header's action.
									ui.Button(c.Key("banner-install"), locales.InstallEllipsis(toolTitle(firstUnmet.Tool)+withSpace(firstCheck.Want))).
										Background(c.Theme().Text).TextColor(c.Theme().Background).BorderColor(c.Theme().Text).
										OnClick(run)
								}
							}
						})
					}
					if len(statuses) > 0 {
						a.requirementTable(c, statuses)
					}
					a.projectServicesSummaryCard(c, p)
					a.projectOverviewCards(c, p)
				}
			})
		})
	})
}

// projectMatches reports whether project p matches the lowercased filter query
// by directory path, required tools, titles, versions, config files, databases,
// or AI agents.
func projectMatches(p projects.Project, statuses []projects.Status, q string) bool {
	if strings.Contains(strings.ToLower(p.Dir), q) {
		return true
	}
	for _, st := range statuses {
		if strings.Contains(strings.ToLower(st.Tool), q) ||
			strings.Contains(strings.ToLower(toolTitle(st.Tool)), q) {
			return true
		}
		for _, ch := range st.Checks {
			if strings.Contains(strings.ToLower(ch.Want), q) ||
				strings.Contains(strings.ToLower(ch.File), q) ||
				strings.Contains(strings.ToLower(st.Tool+" "+ch.Want), q) ||
				strings.Contains(strings.ToLower(toolTitle(st.Tool)+" "+ch.Want), q) {
				return true
			}
		}
	}
	for _, f := range p.Files {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	for _, db := range p.Databases {
		if strings.Contains(strings.ToLower(db.Kind), q) ||
			strings.Contains(strings.ToLower(db.Service), q) ||
			slices.ContainsFunc(db.Ports, func(port string) bool { return strings.Contains(port, q) }) {
			return true
		}
	}
	for _, id := range projectAgents(p) {
		if strings.Contains(strings.ToLower(id), q) ||
			strings.Contains(strings.ToLower(agentTitle(id)), q) {
			return true
		}
	}
	return false
}

type projectCardSlot struct {
	weight int
	render func()
}

// projectOverviewCards arranges the project's secondary cards (Docker Compose,
// databases, Kubernetes, disk, AI, files) into a height-balanced 2-column grid
// on standard windows and a single column on narrow windows.
func (a *App) projectOverviewCards(c *ui.Context, p projects.Project) {
	const cardOverhead = 64 // header (44px) + vertical gap between cards
	var cards []projectCardSlot
	if stacks := composeProjects(p); len(stacks) > 0 {
		cards = append(cards, projectCardSlot{weight: cardOverhead + len(stacks)*48, render: func() { a.projectDockerCard(c, p) }})
	}
	if len(p.Databases) > 0 {
		cards = append(cards, projectCardSlot{weight: cardOverhead + len(p.Databases)*54, render: func() { a.projectDatabasesCard(c, p) }})
	}
	if _, linked := a.KubeLinks[p.Dir]; linked || len(p.Kube) > 0 {
		rows := 1
		if len(p.Kube) > 0 {
			rows++
		}
		if linked {
			rows++
		}
		cards = append(cards, projectCardSlot{weight: cardOverhead + rows*54, render: func() { a.projectKubeCard(c, p) }})
	}
	if len(p.Installs) > 0 {
		cards = append(cards, projectCardSlot{weight: cardOverhead + len(p.Installs)*42, render: func() { a.projectDiskCard(c, p) }})
	}
	cards = append(cards, projectCardSlot{weight: cardOverhead + max(1, len(p.AI))*52, render: func() { a.aiCard(c, p) }})
	fileRows := len(p.Files) + len(p.Errs)
	filesH := cardOverhead + 44
	if fileRows > 0 {
		filesH = cardOverhead + fileRows*66
	}
	cards = append(cards, projectCardSlot{weight: filesH, render: func() { a.filesCard(c, p) }})

	if narrow(c) || len(cards) <= 1 {
		for _, card := range cards {
			card.render()
		}
		return
	}
	left, right := balanceProjectCards(cards)
	ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(18).Children(func() {
			for _, card := range left {
				card.render()
			}
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(18).Children(func() {
			for _, card := range right {
				card.render()
			}
		})
	})
}

// balanceProjectCards splits up to 6 cards into two non-empty columns while
// preserving relative order in each column and minimizing column weight skew.
func balanceProjectCards(cards []projectCardSlot) (left, right []projectCardSlot) {
	n := len(cards)
	if n <= 1 {
		return cards, nil
	}
	bestMask := 1
	bestDiff := int(^uint(0) >> 1)
	bestLeftCount := 0
	// Fix card 0 in the left column (bit 0 = 1) to avoid mirror duplicates.
	limit := 1 << n
	for mask := 1; mask < limit; mask += 2 {
		if mask == limit-1 {
			continue
		}
		lw, rw, lc := 0, 0, 0
		for i, card := range cards {
			if mask&(1<<i) != 0 {
				lw += card.weight
				lc++
			} else {
				rw += card.weight
			}
		}
		diff := lw - rw
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff || (diff == bestDiff && lc > bestLeftCount) {
			bestDiff = diff
			bestMask = mask
			bestLeftCount = lc
		}
	}
	for i, card := range cards {
		if bestMask&(1<<i) != 0 {
			left = append(left, card)
		} else {
			right = append(right, card)
		}
	}
	return left, right
}

// projectDatabasesCard shows the local databases referenced by Compose and .env
// for project p, matched against observed servers, listeners, and port conflicts.
func (a *App) projectDatabasesCard(c *ui.Context, p projects.Project) {
	if len(p.Databases) == 0 {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	servers := inventory.DatabaseServers(a.Snapshot)
	var listeners []inventory.DatabaseListener
	if st, ok := a.Snapshot.Tool("databases"); ok && st.Databases != nil {
		listeners = st.Databases.Listeners
	}
	conflictPorts := map[string]bool{}
	for _, is := range a.issues(c) {
		if is.Kind == issues.DatabasePortConflict && is.Project == p.Dir {
			for _, d := range is.Dirs {
				conflictPorts[d] = true
			}
		}
	}
	ui.Column(c.Key("project-databases-card")).Radius(12).Clip().Background(t.Background).
		BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectDatabasesTitle()).FontSize(13.5).FontWeight(600).SingleLine().Shrink(0)
			clipped(c, "compose · .env").Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(1)
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			pageLinkAction(c, "project-db-open-all", locales.OpenDatabases(), func() {
				a.router.Push("/tools/databases")
			})
		})
		for i, ref := range p.Databases {
			title := ref.Kind
			if ref.Service != "" {
				title += " · " + ref.Service
			}
			caption := ref.File
			if len(ref.Ports) > 0 {
				ports := make([]string, len(ref.Ports))
				for j, port := range ref.Ports {
					ports[j] = ":" + port
				}
				caption += " · " + strings.Join(ports, ", ")
			}
			busyPort := ""
			for _, port := range ref.Ports {
				if conflictPorts[port] {
					busyPort = port
					break
				}
			}
			running, knownStopped := projectDatabaseState(ref, servers, listeners)
			key := strconv.Itoa(i)
			listRow(c, title, false, caption, func() {
				switch {
				case busyPort != "":
					chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ProjectDatabasePortBusy(busyPort))
					port := busyPort
					ui.Button(c.Key("project-db-port:"+key), locales.ProjectDatabaseViewPort(port)).Height(rowControlHeight).OnClick(func() {
						a.router.Push(portsPathWithFilter(port))
					})
				case running:
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.DatabaseRunning())
				case knownStopped:
					chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.DatabaseStopped())
					if plan, canStart := a.projectDatabaseStartPlan(ref, servers); canStart {
						ui.Button(c.Key("project-db-start:"+key), locales.ServiceStart()).Height(rowControlHeight).OnClick(func() {
							a.Propose(plan)
						})
					}
				}
			})
		}
	})
}

func projectDatabaseState(ref projects.DatabaseReference, servers []inventory.Database, listeners []inventory.DatabaseListener) (running, knownStopped bool) {
	for _, port := range ref.Ports {
		if slices.ContainsFunc(listeners, func(l inventory.DatabaseListener) bool { return l.Port == port }) {
			return true, false
		}
	}
	for _, db := range servers {
		if db.Kind != ref.Kind {
			continue
		}
		if db.Status == "running" {
			return true, false
		}
		if db.Status == "stopped" {
			knownStopped = true
		}
	}
	return false, knownStopped
}

// projectNeedsPrepare reports whether the project has unmet requirements,
// config read errors, or unchecked tools that warrant the primary Prepare
// button in the page header.
func projectNeedsPrepare(p projects.Project, statuses []projects.Status) bool {
	if len(p.Errs) > 0 || projectProblem(statuses) != "" {
		return true
	}
	return len(statuses) > 0 && !anyChecked(statuses)
}

// projectMenu is the project's "…": optional re-check/prepare when ready,
// pin actions, show its folder, and remove it from the list.
func (a *App) projectMenu(c *ui.Context, dir string, includePrepare bool) {
	ui.Button(c.Key("project-menu"), "").Size(controlHeight, controlHeight).Padding(0).Label(locales.MoreActions()).Menu(func(m *ui.Menu) {
		if includePrepare && a.Snapshot != nil && a.BuildPlan != nil && a.preparing == nil {
			if m.Item(locales.PrepareButton()).Chosen() {
				a.prepareProject(dir)
			}
			m.Separator()
		}
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
// runs of its tools, where its files do not pin them already.
func (a *App) pinItems(m *ui.Menu, dir string) {
	i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir })
	if i < 0 {
		return
	}
	p := a.Projects[i]
	added := false
	for _, st := range projects.Resolve(p, a.Snapshot, a.nodeLines()) {
		if st.Terminal == nil || st.Downloads || st.Terminal.Version == "" {
			continue
		}
		ts, _ := a.toolState(st.Tool)
		if ts.Tool == "" {
			ts.Tool = st.Tool
		}
		t := ops.PinTarget{Tool: st.Tool, Title: toolTitle(st.Tool), Version: st.Terminal.Version, Manager: a.installManager(ts)}
		plan, ok := ops.PinPlan(p, t)
		if !ok {
			continue
		}
		added = true
		if m.Item(locales.PinMenu(t.Title, t.Version)).Chosen() {
			if a.BuildPlan != nil && slices.ContainsFunc(plan.Steps, func(s ops.Step) bool {
				return len(s.Argv) > 0 && s.Argv[0] == "mise"
			}) {
				base := plan
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					mise, err := ops.SetUpMise(env)
					if err != nil {
						return ops.Plan{}, err
					}
					return ops.WithMise(base, mise), nil
				}, a.proposeOrSay)
			} else {
				a.Propose(plan)
			}
		}
	}
	if added {
		m.Separator()
	}
}

// projectInstall is the plan that installs the version a project needs and
// does not have: the newest matching release through the manager in use (or
// system package manager for PHP).
func (a *App) projectInstall(st projects.Status) (ops.Plan, bool) {
	ch, ok := st.Unmet()
	if !ok || ch.Installed != nil {
		return ops.Plan{}, false
	}
	version, ok := a.projectInstallVersion(st, ch)
	if !ok {
		return ops.Plan{}, false
	}
	if st.Tool == "php" {
		newest := ""
		if a.PHPVersions != nil {
			newest = a.PHPVersions.Newest(time.Now())
		}
		return a.phpInstall(version, newest)
	}
	ts, _ := a.toolState(st.Tool)
	if ts.Tool == "" {
		ts.Tool = st.Tool
	}
	manager := a.installManager(ts)
	if manager == "" {
		return ops.Plan{}, false
	}
	installVersion, label := projectInstallTarget(st.Tool, version)
	return a.versionPlan(ops.VersionInstall, st.Tool, manager, installVersion, label, "")
}

func (a *App) projectInstallVersion(st projects.Status, ch projects.Check) (string, bool) {
	releases := a.projectReleases()
	if st.Tool == "dotnet" && ch.SDK != nil {
		return projects.PickDotnetSDK(releases["dotnet"], *ch.SDK)
	}
	selection, err := projects.SelectVersion(st, releases[st.Tool], a.nodeLines())
	if err != nil || selection.Installed != nil {
		return "", false
	}
	return selection.Version, true
}

func projectInstallTarget(tool, version string) (installVersion, label string) {
	installVersion = version
	label = versionLabel(tool, version)
	switch tool {
	case "java":
		installVersion, _, _ = strings.Cut(version, ".")
		label = "Temurin " + version
	case "dotnet":
		label = "SDK " + version
	}
	return installVersion, label
}

// projectInstallAction returns an inline install action for an unmet,
// uninstalled requirement: through the installed version manager, the tool
// catalog for unversioned tools (Composer, uv, Docker, Git…), or auto-mise
// when a versioned tool has no version manager yet.
func (a *App) projectInstallAction(st projects.Status, done func(ops.Plan, error)) (label string, run func(), ok bool) {
	ch, unmet := st.Unmet()
	if !unmet || ch.Installed != nil {
		return "", nil, false
	}
	if plan, ok := a.projectInstall(st); ok {
		return plan.Title, func() { done(plan, nil) }, true
	}
	if ch.Want == "" && statusWant(st) == "" && a.canInstallTool(st.Tool) {
		tool := st.Tool
		return locales.PkgInstallTitle(toolTitle(tool)), func() { a.installToolPlan(tool, done) }, true
	}
	if st.Tool == "php" || a.BuildPlan == nil {
		return "", nil, false
	}
	version, ok := a.projectInstallVersion(st, ch)
	if !ok {
		return "", nil, false
	}
	ts, _ := a.toolState(st.Tool)
	if ts.Tool == "" {
		ts.Tool = st.Tool
	}
	if a.installManager(ts) != "" {
		return "", nil, false
	}
	installVersion, verLabel := projectInstallTarget(st.Tool, version)
	p, ok := a.versionPlan(ops.VersionInstall, st.Tool, "mise", installVersion, verLabel, "")
	if !ok {
		return "", nil, false
	}
	return p.Title, func() {
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			s, err := ops.SetUpMise(env)
			return ops.WithMise(p, s), err
		}, done)
	}, true
}

// projectHero is the folder, its name with inline Git branch pill and what it
// needs, and on the right the cards of where it stands: needed, status, AI.
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
	headRow(c, func() {
		folderTile(c, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, filepath.Base(p.Dir)).FontSize(22).FontWeight(600).SingleLine()
				if g := p.Git; g != nil && g.Branch != "" {
					bg, border, fg := pal.Subtle, pal.Line, pal.MonoText
					statusText := locales.ProjectGitClean()
					if g.Dirty {
						bg, border, fg = pal.WarningSoft, t.Warning.Alpha(0.30), pal.WarningText
						statusText = locales.ProjectGitModified(g.Modified)
					}
					ui.Row(c.Key("project-hero-git")).Gap(5).Padding(3, 8).Radius(6).AlignItems(ui.Center).Shrink(0).
						Background(bg).BorderWidth(1).BorderColor(border).Children(func() {
						ui.Icon(c, Icon("git-branch")).Size(12, 12).TextColor(fg)
						ui.Text(c, g.Branch).Font(FontMono).FontSize(11.5).FontWeight(500).TextColor(fg).SingleLine()
						ui.Text(c, "·").FontSize(11).TextColor(fg).SingleLine()
						ui.Text(c, statusText).FontSize(11).TextColor(fg).SingleLine()
					})
				}
			})
			ui.Column(c).Gap(2).AlignItems(ui.Start).Children(func() {
				if g := p.Git; g != nil && g.Remote != "" {
					if g.RemoteURL != "" {
						remoteURL := g.RemoteURL
						btn := ui.ButtonBase(c.Key("project-git-remote")).Row().Gap(4).Padding(1, 0).Radius(5).
							AlignItems(ui.Center).Shrink(0).Label(g.Remote).Tooltip(locales.ProjectGitOpenRemote()).
							OnClick(func() { c.OpenURL(remoteURL) })
						if btn.Hovered() {
							btn.Background(pal.Subtle)
						}
						btn.Children(func() {
							ui.Text(c, g.Remote).Font(FontMono).FontSize(12).TextColor(t.TextMuted).SingleLine()
							ui.Icon(c, Icon("external-link")).Size(11, 11).TextColor(pal.TextFaint)
						})
					} else {
						ui.Text(c, g.Remote).Font(FontMono).FontSize(12).TextColor(t.TextMuted).SingleLine()
					}
				} else {
					ui.Text(c, tilde(p.Dir, a.Home)).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
				}
				if len(needs) > 0 {
					ui.Text(c, strings.Join(needs, " · ")).FontSize(12).TextColor(pal.TextFaint).Wrap()
				}
			})
		})
	}, func() {
		stateCard(c, pal.TextFaint, locales.Needed(), locales.ToolsCount(len(statuses)), strings.Join(files, " · "))
		problem := projectProblem(statuses)
		if runPort, runCaption, running := a.projectPrimaryRunning(p); running && problem == "" {
			val := locales.ProjectStatusRunningNoPort()
			if runPort != "" {
				val = locales.ProjectStatusRunning(runPort)
			}
			stateCard(c, t.Success, locales.ProjectStatus(), val, runCaption)
		} else {
			dot, readyCaption := t.Success, locales.ChipReady()
			if problem != "" {
				dot, readyCaption = t.Warning, problem
			}
			stateCard(c, dot, locales.Ready(), locales.ReadyOf(ready, len(statuses)), readyCaption)
		}
		var names []string
		for _, id := range projectAgents(p) {
			names = append(names, agentTitle(id))
		}
		value, aiCaption := "—", locales.NoAISettings()
		switch {
		case len(names) > 0:
			value = locales.ProjectAgentsCount(len(names))
			aiCaption = strings.Join(names, ", ")
		case len(p.Skills) > 0:
			value = locales.ProjectSkillsCount(len(p.Skills))
		}
		stateCard(c, Accent, "AI", value, aiCaption)
	})
}

// requirementTable is a row per tool: what the project needs and from
// which files, what is installed, and what the terminal runs in its folder.
func (a *App) requirementTable(c *ui.Context, statuses []projects.Status) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			for i, h := range []string{locales.ColumnTool(), locales.ColumnNeeded(), locales.ColumnInstalled(), locales.ColumnRunsHere()} {
				ui.Text(c, h).FontSize(11.5).TextColor(pal.TextFaint).Grow(columnGrow[i]).Basis(0).MinWidth(0).SingleLine()
			}
			ui.Box(c).Width(124).Shrink(0)
		})
		for _, st := range statuses {
			dest := projectToolPath(st.Tool)
			row := ui.ButtonBase(c.Key("req:"+st.Tool)).Gap(12).Padding(12, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(toolTitle(st.Tool)).Transition(colorFade)
			if row.Hovered() {
				row.Background(pal.Subtle)
			}
			childClicked := false
			row.Children(func() {
				ui.Row(c).Grow(columnGrow[0]).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
					iconOrLogo(c, toolIcon(st.Tool), 16, t.TextMuted)
					ui.Text(c, toolTitle(st.Tool)).FontSize(13.5).SingleLine()
				})
				want := statusWants(st)
				if want == "" {
					want = locales.AnyVersion()
				}
				cell(c, columnGrow[1], want, checkSources(st), pal.TextFaint)
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
				ui.Row(c).Width(124).Shrink(0).Justify(ui.End).Children(func() {
					label, runInstall, install := a.projectInstallAction(st, a.proposeOrSay)
					switch {
					case install:
						ui.Button(c.Key("install:"+st.Tool), "").Label(label).OnClick(func() {
							childClicked = true
							runInstall()
						}).Children(func() {
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
			if row.Clicked() && !childClicked && dest != "" {
				a.router.Push(dest)
			}
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
		clipped(c, value).Font(FontMono).FontSize(13)
		if caption != "" {
			clipped(c, caption).FontSize(11.5).TextColor(color)
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
func (a *App) aiCard(c *ui.Context, p projects.Project) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectAI()).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			pageLinkAction(c, "project-ai-manage", locales.ProjectAiManage(), func() {
				a.projectTab = 2
			})
		})
		if len(p.AI) == 0 {
			cardNote(c, locales.NoAISettings())
		}
		for _, f := range p.AI {
			name := f
			full := filepath.Join(p.Dir, filepath.FromSlash(name))
			listRow(c, name, false, aiFileCaption(name), func() {
				if p.Dir == "" {
					return
				}
				switch name {
				case ".claude", ".agents", ".agent", "_agents", "_agent", ".cursor":
					if a.Shell != nil {
						iconButton(c.Key("ai-open:"+name), "folder-open", showInFolder()).OnClick(func() {
							if err := a.Shell.OpenPath(full); err != nil {
								c.Toast(locales.OpenFailed(tilde(full, a.Home), err.Error()))
							}
						})
					}
				default:
					if a.canOpenFile() {
						iconButton(c.Key("ai-open:"+name), "external-link", locales.OpenInEditor()).OnClick(func() {
							a.openFile(c, full)
						})
					}
				}
			})
		}
	})
}

// filesCard lists the project files the versions come from, and the ones
// that could not be read.
func (a *App) filesCard(c *ui.Context, p projects.Project) {
	pal := Colors(c.Theme())
	listCard(c, locales.ProjectFiles(), locales.WhereVersionsFrom(), func() {
		if len(p.Files) == 0 && len(p.Errs) == 0 {
			cardNote(c, locales.NoProjectFiles())
		}
		for _, f := range p.Files {
			file := f
			var open func()
			if p.Dir != "" && a.canOpenFile() {
				full := filepath.Join(p.Dir, filepath.FromSlash(file))
				open = func() { a.openFile(c, full) }
			}
			fileRow(c, p, file, open)
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
					fileRow(c, add.root, f, nil)
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
						ui.Checkbox(c.Key("nested:"+p.Dir), &add.on[i], "").Label(filepath.ToSlash(rel))
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
				ui.Button(c, cancel).Height(controlHeight).OnClick(func() { a.addOpen = false })
				ui.PrimaryButton(c, locales.AddProjects(count)).Height(controlHeight).Disabled(count == 0).OnClick(func() {
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
// a checkbox; the ones long unchanged say so.
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
				ui.Checkbox(c.Key("found:"+p.Dir), &add.on[i], "").Label(filepath.ToSlash(rel))
			})
		}
	}
}

func dialogSection(c *ui.Context, title string) {
	pal := Colors(c.Theme())
	ui.Text(c, title).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Padding(14, 20, 6, 20).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).SingleLine()
}

// fileRequirementSummary returns deduplicated keys and per-tool grouped pins
// for one project file (for example ["node >=24 · ^24.0.0", "pnpm >=12.14.0 · 12.14.0"]).
func fileRequirementSummary(p projects.Project, file string) (keys, pills []string) {
	type toolPins struct {
		tool  string
		wants []string
	}
	var grouped []toolPins
	for _, r := range p.Requirements {
		if r.File != file {
			continue
		}
		if r.Key != "" && !slices.Contains(keys, r.Key) {
			keys = append(keys, r.Key)
		}
		idx := slices.IndexFunc(grouped, func(g toolPins) bool { return g.tool == r.Tool })
		if idx < 0 {
			grouped = append(grouped, toolPins{tool: r.Tool})
			idx = len(grouped) - 1
		}
		if want := strings.TrimSpace(r.Want); want != "" && !slices.Contains(grouped[idx].wants, want) {
			grouped[idx].wants = append(grouped[idx].wants, want)
		}
	}
	pills = make([]string, 0, len(grouped))
	for _, g := range grouped {
		if len(g.wants) > 0 {
			pills = append(pills, g.tool+" "+strings.Join(g.wants, " · "))
		} else {
			pills = append(pills, g.tool)
		}
	}
	return keys, pills
}

// filePinPill renders one compact mono requirement badge.
func filePinPill(c *ui.Context, text string) {
	pal := Colors(c.Theme())
	ui.Box(c).Padding(2, 7).Radius(5).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Text(c, text).Font(FontMono).FontSize(11).TextColor(pal.MonoText).SingleLine()
	})
}

// fileRow is a project file: its name, the fields versions come from, and
// grouped requirement pills that wrap cleanly below the filename when needed
// so long requirement lists never compress the filename.
func fileRow(c *ui.Context, p projects.Project, file string, open func()) {
	pal := Colors(c.Theme())
	keys, pills := fileRequirementSummary(p, file)
	ui.Column(c).Gap(6).MinHeight(46).Padding(9, 12, 9, 14).Justify(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, file).FontSize(13).SingleLine().Tooltip(file)
				if len(keys) > 0 {
					caption := strings.Join(keys, " · ")
					ui.Text(c, caption).FontSize(11.5).TextColor(pal.TextFaint).SingleLine().Tooltip(caption)
				}
			})
			if open != nil {
				iconButton(c.Key("file-open:"+file), "external-link", locales.OpenInEditor()).OnClick(open)
			}
		})
		if len(pills) > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, pill := range pills {
					filePinPill(c, pill)
				}
			})
		}
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

// statusWants joins every distinct non-empty version requirement for a tool.
func statusWants(st projects.Status) string {
	var wants []string
	for _, ch := range st.Checks {
		if want := strings.TrimSpace(ch.Want); want != "" && !slices.Contains(wants, want) {
			wants = append(wants, want)
		}
	}
	return strings.Join(wants, " · ")
}

// checkSource is where a requirement comes from, as "package.json ·
// packageManager".
func checkSource(ch projects.Check) string {
	if ch.Key == "" || ch.File == ".tool-versions" {
		return ch.File
	}
	return ch.File + " · " + ch.Key
}

// checkSources joins every distinct requirement source for a tool.
func checkSources(st projects.Status) string {
	var sources []string
	for _, ch := range st.Checks {
		if src := checkSource(ch); src != "" && !slices.Contains(sources, src) {
			sources = append(sources, src)
		}
	}
	return strings.Join(sources, ", ")
}

// statusProblem is what keeps a checked tool from being ready in the
// project: the needed version is not installed, or the terminal runs
// another; "" when nothing does.
func statusProblem(st projects.Status) string {
	ch, ok := st.Unmet()
	switch {
	case !ok && st.EngineDown:
		return locales.ProblemEngineDown(toolTitle(st.Tool))
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

type projectProblemBanner struct {
	Title string
	Meta  string
}

// problemBanner summarizes a project's environment issues as a clear heading
// and concise metadata line: single issues state what is missing or mismatched
// and where it is declared; multiple issues list the affected tools directly.
func problemBanner(statuses []projects.Status) projectProblemBanner {
	type problemDetail struct {
		title string
		meta  string
		short string
	}
	var items []problemDetail
	for _, st := range statuses {
		ch, ok := st.Unmet()
		if !ok {
			if st.EngineDown {
				var file string
				if len(st.Checks) > 0 {
					file = st.Checks[0].File
				}
				meta := locales.ProblemEngineDownMeta()
				if file != "" {
					meta = locales.ProblemEngineDownForFile(file)
				}
				items = append(items, problemDetail{
					title: locales.ProblemEngineDownTitle(),
					meta:  meta,
					short: locales.ProblemItemEngineDown(),
				})
			}
			continue
		}

		tool := toolTitle(st.Tool)
		need := tool + withSpace(ch.Want)
		vs := installedVersions(st)

		var title, meta, short string
		switch {
		case len(vs) == 0:
			title = locales.ProblemNotInstalledTitle(need)
			if ch.File != "" {
				meta = locales.ProblemRequiredBy(ch.File)
			} else {
				meta = locales.ProblemNotInstalledMeta()
			}
			short = locales.ProblemItemNotInstalled(need)

		case ch.Installed != nil:
			want := cmp.Or(ch.Want, copyVersion(*ch.Installed))
			if st.Terminal != nil {
				runVer := copyVersion(*st.Terminal)
				title = locales.ProblemWrongActiveTitle(tool, runVer, want)
				short = locales.ProblemItemWrongVersion(need, runVer)
			} else {
				title = locales.ProblemNotInPathTitle(need)
				short = locales.ProblemItemNotInPath(need)
			}
			meta = locales.ProblemAlreadyInstalledMeta(want)
			if ch.File != "" {
				meta = locales.ProblemRequiredBy(ch.File) + " · " + meta
			}

		default:
			title = locales.ProblemMissingVersionTitle(need)
			var stateMeta string
			if st.Terminal != nil {
				stateMeta = locales.ProblemInstalledMeta(strings.Join(vs, ", "), copyVersion(*st.Terminal))
			} else {
				stateMeta = locales.ProblemInstalledNotInPathMeta(strings.Join(vs, ", "))
			}
			if ch.File != "" {
				meta = locales.ProblemRequiredBy(ch.File) + " · " + stateMeta
			} else {
				meta = stateMeta
			}
			short = locales.ProblemItemMissing(need)
		}

		items = append(items, problemDetail{title: title, meta: meta, short: short})
	}

	if len(items) == 0 {
		return projectProblemBanner{}
	}
	if len(items) == 1 {
		return projectProblemBanner{Title: items[0].title, Meta: items[0].meta}
	}

	const maxListed = 3
	listed := make([]string, 0, min(len(items), maxListed)+1)
	for i := range min(len(items), maxListed) {
		listed = append(listed, items[i].short)
	}
	if len(items) > maxListed {
		listed = append(listed, locales.AttentionMore(len(items)-maxListed))
	}
	return projectProblemBanner{
		Title: locales.ProblemMultiTitle(len(items)),
		Meta:  strings.Join(listed, " · "),
	}
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

// projectAgents are the agents whose files the project has: Claude Code
// for CLAUDE.md, .claude and .mcp.json; Codex and others for AGENTS.md;
// GitHub Copilot for .github/copilot-instructions.md; Cursor for .cursor.
func projectAgents(p projects.Project) []string {
	var out []string
	add := func(id string) {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, f := range p.AI {
		switch f {
		case "CLAUDE.md", ".claude", ".mcp.json":
			add("claude")
		case "AGENTS.md", ".agents":
			add("codex")
			add("antigravity")
		case "GEMINI.md":
			add("antigravity")
			add("gemini")
		case ".github/copilot-instructions.md":
			add("copilot")
		case ".agent", "_agents", "_agent":
			add("antigravity")
		case ".cursor":
			add("cursor")
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
		return locales.AiInstructions("Codex, Copilot, Gemini CLI, Antigravity CLI")
	case "GEMINI.md":
		return locales.AiInstructions("Gemini CLI, Antigravity CLI")
	case ".github/copilot-instructions.md":
		return locales.AiInstructions("GitHub Copilot")
	case ".agents":
		return locales.AiSkills()
	case ".agent", "_agents", "_agent":
		return locales.AiSettings("Antigravity CLI")
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
	"pipx": "pipx", "bundler": "Bundler", "maven": "Maven", "gradle": "Gradle", "helm": "Helm",
	"terraform": "Terraform", "tofu": "OpenTofu",
}

func toolTitle(id string) string {
	if n, ok := toolTitles[id]; ok {
		return n
	}
	return id
}

// toolIcon is the icon key of a tool's logo, or of a package for a tool
// without one.
func toolIcon(id string) string {
	for _, l := range languages {
		if l.id == id {
			return l.icon
		}
	}
	switch id {
	case "npm", "pnpm", "docker", "git", "mise":
		return "brand/" + id
	case "kubectl", "kind":
		return "brand/kubernetes"
	}
	return "lucide/package"
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
func segments(c *ui.Context, key string, tab *int, segs []segment) ui.Element {
	t := c.Theme()
	pal := Colors(t)
	var track ui.Element
	ui.Row(c).Children(func() {
		seg := ui.SegmentedBase(c.Key(key), tab, len(segs))
		track = seg.Track
		seg.Track.Height(controlHeight).Gap(2).Padding(2).Radius(8).Background(pal.Muted).Children(func() {
			for i, s := range segs {
				e := seg.Segment(i).Height(controlHeight-4).Gap(5).Padding(0, 12).Radius(6).AlignItems(ui.Center).Label(s.label)
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
	return track
}
