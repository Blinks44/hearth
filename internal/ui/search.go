package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// searchResult keeps data and a deferred action, never a build's elements.
type searchResult struct {
	id, group, title, detail, icon, words string
	open                                  func()
}

type searchCacheKey struct {
	snap       *inventory.Snapshot
	projectsN  int
	project0   *projects.Project
	hiddenN    int
	pkgUpdN    int
	home       string
	langMarker string
	hasChoose  bool
	day        int
	nodeV      *versions.Node
	goV        *versions.Go
	pyV        *versions.Cycles
	phpV       *versions.Cycles
	rustV      *versions.Cycles
	rubyV      *versions.Cycles
	javaV      *versions.Cycles
	dotnetV    *versions.Dotnet
	bunV       *versions.Latest
}

type searchState struct {
	open      bool
	available bool // the lists of versions to install are searched
	query     string
	selected  int
	scroll    bool // the keyboard moved the selection: bring it into view
	recent    []string

	indexKey    searchCacheKey
	indexCached []searchResult
	indexValid  bool
	availKey    searchCacheKey
	availCached []searchResult
	availValid  bool
	indexBuilds int
}

func (a *App) toggleSearch() {
	a.search.open = !a.search.open
	a.search.available = false
	a.search.query = ""
	a.search.selected = 0
	a.search.scroll = false
}

// searchAvailable switches between the local results and the versions to
// install, from the first result.
func (a *App) searchAvailable(on bool) {
	a.search.available, a.search.selected = on, 0
}

func (a *App) searchKey(now time.Time) searchCacheKey {
	var p0 *projects.Project
	if len(a.Projects) > 0 {
		p0 = &a.Projects[0]
	}
	return searchCacheKey{
		snap:       a.Snapshot,
		projectsN:  len(a.Projects),
		project0:   p0,
		hiddenN:    len(a.Hidden),
		pkgUpdN:    len(a.PackageUpdates),
		home:       a.Home,
		langMarker: locales.SearchTools(),
		hasChoose:  a.ChooseFolder != nil,
		day:        now.Year()*1000 + now.YearDay(),
		nodeV:      a.NodeVersions,
		goV:        a.GoVersions,
		pyV:        a.PythonVersions,
		phpV:       a.PHPVersions,
		rustV:      a.RustVersions,
		rubyV:      a.RubyVersions,
		javaV:      a.JavaVersions,
		dotnetV:    a.DotnetVersions,
		bunV:       a.BunLatest,
	}
}

func (a *App) resetPageTab(path string) {
	switch path {
	case "/tools/node":
		a.nodeTab = 0
	case "/tools/go":
		a.goTab = 0
	case "/tools/python":
		a.pythonTab = 0
	case "/tools/php":
		a.phpTab = 0
	case "/tools/rust":
		a.rustTab = 0
	case "/tools/ruby":
		a.rubyTab = 0
	case "/tools/java":
		a.javaTab = 0
	case "/tools/dotnet":
		a.dotnetTab = 0
	case "/tools/bun":
		a.bunTab = 0
	case "/tools/jspm":
		a.jspmTab = 0
	case "/system/packages":
		a.packageTab = 0
	case pathPage:
		a.pathTab = 0
	case problemsPage:
		a.problemsTab = 0
	case cliPage:
		a.cliTab = 0
	case kubePage:
		a.kubeTab = 0
	case mobilePage:
		a.mobileTab = 0
	case editorsPage:
		a.editorsTab = 0
	}
}

func (a *App) searchIndex() []searchResult {
	return a.searchIndexAt(time.Now())
}

func (a *App) searchIndexAt(now time.Time) []searchResult {
	key := a.searchKey(now)
	if a.search.indexValid && a.search.indexKey == key {
		return a.search.indexCached
	}
	var out []searchResult
	add := func(r searchResult) { out = append(out, r) }
	all := pages()
	for _, p := range all {
		path := p.path
		add(searchResult{id: path, group: locales.SearchTools(), title: p.title(), detail: p.subtitle(), icon: p.icon, open: func() {
			a.resetPageTab(path)
			a.router.Push(path)
		}})
	}
	if a.Snapshot != nil {
		for _, tool := range a.Snapshot.Tools {
			target := "/tools/" + tool.Tool
			switch tool.Tool {
			case "npm", "pnpm", "yarn", "corepack":
				target = "/tools/jspm"
			case "packages":
				target = "/system/packages"
			}
			if !slices.ContainsFunc(all, func(p page) bool { return p.path == target }) {
				continue
			}
			for _, in := range searchCopies(tool) {
				name := tool.Tool
				if in.Tool != "" {
					name = in.Tool
				}
				detail := strings.Trim(in.Source.Manager+" · "+tilde(in.Path, a.Home), " ·")
				to := target
				if tool.Cli != nil && slices.Contains(kubeCommands, in.Tool) {
					to = kubePage
				}
				add(searchResult{id: "version:" + tool.Tool + ":" + in.Path, group: locales.SearchVersions(), title: strings.TrimSpace(name + " " + in.Version), detail: detail, icon: "lucide/box", open: func() {
					a.resetPageTab(to)
					a.router.Push(to)
				}})
			}
		}
		for _, k := range a.cliState().KubeContexts {
			if !k.Shadowed {
				add(searchResult{id: "kube:" + k.Name, group: locales.KubeContextsCard(), title: k.Name, detail: strings.Trim(k.Cluster+" · "+tilde(k.File, a.Home), " ·"), icon: "brand/kubernetes", open: func() {
					a.kubeTab, a.kubeFilter = 0, k.Name
					a.router.Push(kubePage)
				}})
			}
		}
	}
	for _, p := range a.Projects {
		var reqs []string
		for _, r := range p.Requirements {
			reqs = append(reqs, r.Tool+" "+r.Want+" "+r.File)
		}
		add(searchResult{id: projectPath(p.Dir), group: locales.ProjectsTitle(), title: filepath.Base(p.Dir), detail: strings.Join(reqs, " · "), words: p.Dir, icon: "lucide/folder-git-2", open: func() { a.router.Push(projectPath(p.Dir)) }})
	}
	for _, ag := range a.agents() {
		id := ag.ID
		detail := locales.AgentNotFound()
		words := id
		if ag.Found() {
			path := ag.App
			if active := inventory.TerminalCopy(ag.Installations); active >= 0 {
				path = ag.Installations[active].Path
				words += " " + ag.Installations[active].Source.Manager
			} else if len(ag.Installations) > 0 {
				path = ag.Installations[0].Path
				words += " " + ag.Installations[0].Source.Manager
			}
			detail = strings.Trim(agentVersion(ag)+" · "+tilde(path, a.Home), " ·")
		}
		add(searchResult{id: "agent:" + id, group: locales.AiAgentsTitle(), title: agentTitle(id), detail: detail, words: words, icon: agentIcon(id), open: func() {
			a.router.Push(agentPath(id))
		}})
	}
	for _, app := range a.modelApps() {
		for _, m := range app.app.Models {
			detail := strings.Trim(app.name+" · "+formatBytes(m.Size)+" · "+m.Format, " ·")
			add(searchResult{id: "model:" + app.name + ":" + m.Name, group: locales.ModelsTitle(), title: m.Name, detail: detail, words: app.name + " " + m.Format + " " + m.Quant, icon: "lucide/cpu", open: func() {
				a.router.Push(modelsPage)
			}})
		}
	}
	for _, sk := range a.allSkills() {
		if len(sk.Paths) == 0 {
			continue
		}
		add(searchResult{id: "skill:" + skillKey(sk), group: locales.SkillsTitle(), title: sk.Name, detail: sk.Description, words: sk.Project, icon: "lucide/puzzle", open: func() {
			a.skillAgent, a.skillQuery, a.skillScope, a.skillProject = "", "", 0, ""
			a.skillChosen = skillKey(sk)
			a.loadSkill(sk)
			a.router.Push(skillsPage)
		}})
	}
	for _, m := range a.mcpItems() {
		add(searchResult{id: "mcp:" + m.key(), group: "MCP", title: m.name, detail: strings.Join(m.agents(), " · "), words: m.project, icon: "lucide/plug", open: func() {
			a.mcpAgent, a.mcpQuery, a.mcpScope = "", "", 0
			a.mcpChosen = m.key()
			a.router.Push(mcpPage)
		}})
	}
	if a.Snapshot != nil {
		found := issues.FindWithLists(a.Snapshot, a.Projects, a.issueLists(), now)
		visible, _ := a.visibleIssues(found)
		for _, it := range visible {
			icon := "lucide/triangle-alert"
			if it.severity == issues.Info {
				icon = "lucide/info"
			}
			title := it.title
			add(searchResult{id: "issue:" + it.key, group: locales.ProblemsTitle(), title: it.title, detail: it.explain, icon: icon, open: func() {
				a.problemsTab = 0
				a.problemsFilter = title
				a.router.Push(problemsPage)
			}})
		}
		seenPath := map[string]bool{}
		for _, entry := range a.Snapshot.TerminalPath {
			if entry.Dir == "" || seenPath[entry.Dir] {
				continue
			}
			seenPath[entry.Dir] = true
			dir := entry.Dir
			detail := "PATH"
			if len(entry.Sources) > 0 {
				detail = "PATH · " + tilde(entry.Sources[0].File, a.Home)
			}
			add(searchResult{id: "path:" + dir, group: locales.PathTitle(), title: tilde(dir, a.Home), detail: detail, words: dir + " path", icon: "lucide/terminal", open: func() {
				a.pathTab = 0
				a.pathFilter = dir
				a.router.Push(pathPage)
			}})
		}
		for _, v := range a.Snapshot.Variables {
			name := v.Name
			raw := v.Terminal
			if raw == "" {
				raw = v.GUI
			}
			val := tilde(raw, a.Home)
			if val == "" {
				val = "—"
			}
			add(searchResult{id: "var:" + name, group: locales.PathTitle(), title: name, detail: val, words: name + " " + raw + " env", icon: "lucide/terminal", open: func() {
				a.pathTab = 2
				a.pathFilter = name
				a.router.Push(pathPage)
			}})
		}
		for _, m := range a.packageManagers() {
			mgr := m
			if checked, ok := a.PackageUpdates[m.Path]; ok {
				mgr = checked
			}
			mgrPath := mgr.Path
			for _, pkg := range mgr.Packages {
				pkgName := pkg.Name
				detail := strings.Trim(mgr.Name+" · "+pkg.Version, " ·")
				if pkg.Available != "" {
					detail += " → " + pkg.Available
				}
				add(searchResult{id: "pkg:" + mgrPath + ":" + packageKey(pkg), group: locales.PackageManagersTitle(), title: pkgName, detail: detail, words: mgr.ID + " " + pkg.Kind, icon: "lucide/package", open: func() {
					a.openPackageSearch(mgrPath, pkgName)
				}})
			}
		}
	}
	for _, e := range a.setupEntries() {
		entry := e
		title := setupTitle(entry.ID)
		detail := locales.InstallTitle()
		if ms := entry.On(goos); len(ms) > 0 {
			detail = locales.InstallTitle() + " · " + managerTitle(ms[0])
		}
		add(searchResult{id: "install:" + entry.ID, group: locales.InstallTitle(), title: title, detail: detail, words: entry.ID + " install", icon: setupToolIcon(entry.ID), open: func() {
			a.setup.wizard = false
			a.setup.step = 0
			a.setup.query = ""
			a.setup.filter = 0
			a.setup.cat = catAll
			cat := setupCategory(entry)
			if cat != catAll {
				if a.setup.more == nil {
					a.setup.more = map[installCat]bool{}
				}
				a.setup.more[cat] = true
				if a.setupHave(entry.ID) {
					if a.setup.showHave == nil {
						a.setup.showHave = map[installCat]bool{}
					}
					a.setup.showHave[cat] = true
				}
			}
			a.router.Push(setupPage)
		}})
	}
	add(searchResult{id: "action:scan", group: locales.SearchActions(), title: locales.Scan(), detail: locales.OverviewSubtitle(), icon: "lucide/refresh-cw", open: a.Scan})
	if a.ChooseFolder != nil {
		add(searchResult{id: "action:project", group: locales.SearchActions(), title: locales.AddProjectTitle(), icon: "lucide/folder-open", open: a.ChooseFolder})
	}
	add(searchResult{id: "action:versions", group: locales.SearchActions(), title: locales.SearchAvailable(), detail: locales.SearchVersions(), words: "version", icon: "lucide/download", open: func() {
		a.search.open, a.search.query = true, ""
		a.searchAvailable(true)
	}})
	a.search.indexKey = key
	a.search.indexCached = out
	a.search.indexValid = true
	a.search.indexBuilds++
	return out
}

func (a *App) searchResults() []searchResult {
	return a.searchResultsAt(time.Now())
}

func (a *App) searchResultsAt(now time.Time) []searchResult {
	all := a.searchIndexAt(now)
	if a.search.available {
		all = a.availableSearchIndex(now)
	}
	query := strings.TrimSpace(strings.ToLower(a.search.query))
	q := strings.Fields(query)
	var out []searchResult
	if len(q) == 0 && a.search.available {
		return all[:min(len(all), 60)]
	}
	if len(q) == 0 {
		for _, id := range a.search.recent {
			if i := slices.IndexFunc(all, func(r searchResult) bool { return r.id == id }); i >= 0 {
				r := all[i]
				r.group = locales.SearchRecent()
				out = append(out, r)
			}
		}
		for _, r := range all {
			if r.group == locales.SearchActions() {
				out = append(out, r)
			}
		}
		return out
	}
	type scored struct {
		result searchResult
		rank   int
	}
	var matched []scored
	for _, r := range all {
		title := strings.ToLower(r.title)
		text := title + " " + strings.ToLower(r.detail+" "+r.words)
		if slices.ContainsFunc(q, func(word string) bool { return !strings.Contains(text, word) }) {
			continue
		}
		rank := 4
		switch {
		case title == query:
			rank = 0
		case strings.HasPrefix(title, query):
			rank = 1
		case strings.Contains(title, query):
			rank = 2
		case !slices.ContainsFunc(q, func(word string) bool { return !strings.Contains(title, word) }):
			rank = 3
		}
		matched = append(matched, scored{result: r, rank: rank})
	}
	slices.SortStableFunc(matched, func(a, b scored) int {
		return cmp.Compare(a.rank, b.rank)
	})
	limit := min(len(matched), 60)
	out = make([]searchResult, limit)
	for i := range limit {
		out[i] = matched[i].result
	}
	return out
}

func (a *App) openSearchResult(r searchResult) {
	a.search.open = false
	if strings.HasPrefix(r.id, "action:") {
		r.open()
		return
	}
	a.search.recent = slices.DeleteFunc(a.search.recent, func(id string) bool { return id == r.id })
	a.search.recent = append([]string{r.id}, a.search.recent...)
	if len(a.search.recent) > 5 {
		a.search.recent = a.search.recent[:5]
	}
	r.open()
}

func (a *App) searchButton(c *ui.Context) {
	pal := Colors(c.Theme())
	ui.ButtonBase(c.Key("open-search")).Height(30).Shrink(0).Margin(0, 0, 10, 0).Padding(0, 9).Gap(7).AlignItems(ui.Center).
		Radius(7).Background(c.Theme().Background).BorderWidth(1).BorderColor(pal.ControlBorder).Label(locales.SearchPlaceholder()).OnClick(a.toggleSearch).Children(func() {
		ui.Icon(c, Icon("search")).Size(14, 14).TextColor(pal.TextFaint)
		ui.Text(c, locales.SearchPlaceholder()).FontSize(12).TextColor(pal.TextFaint).Grow(1).SingleLine()
		ui.Text(c, cmdKey("K")).FontSize(11).TextColor(pal.TextFaint)
	})
}

// cmdKey is the shortcut of key with the platform's modifier, as ⌘K on
// macOS and Ctrl+K elsewhere.
func cmdKey(key string) string {
	if ui.Cmd == ui.Super {
		return "⌘" + key
	}
	return "Ctrl+" + key
}

func (a *App) searchDialog(c *ui.Context) {
	if !a.search.open {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	width, height := c.Size()
	dialogBase(c.Key("global-search"), &a.search.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.18)).Justify(ui.Start).Padding(min(float32(80), height/10), 0, 0, 0)
		panel.Width(640).MaxWidth(width-32).Radius(14).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Shadow(0, 12, 32, 0, pal.Shadow)
		ui.Column(c).Children(func() {
			now := c.Now()
			ui.Row(c).Padding(14, 16, 12, 16).Children(func() {
				ui.SearchField(c.Key("global-query"), &a.search.query).Label(locales.SearchPlaceholder()).Placeholder(locales.SearchPlaceholder()).AutoFocus().Grow(1).Height(37).
					OnChange(func() { a.search.selected = 0 }).OnSubmit(func() { a.submitSearchAt(now) })
			})
			if a.search.available {
				ui.Button(c, locales.SearchLocal()).Margin(0, 16, 8, 16).OnClick(func() { a.searchAvailable(false) })
			}
			results := a.searchResultsAt(now)
			selected := min(a.search.selected, max(len(results)-1, 0))
			scroll := a.search.scroll
			a.search.scroll = false
			move := func(by int) {
				if n := len(results); n > 0 {
					a.search.selected = (selected + by + n) % n
					a.search.scroll = true
				}
			}
			c.OnShortcut(ui.Cmd, ui.KeyK, a.toggleSearch)
			c.OnShortcut(0, ui.KeyDown, func() { move(1) })
			c.OnShortcut(0, ui.KeyUp, func() { move(-1) })
			ui.Scroll(c.Key("search-results")).MaxHeight(min(float32(472), height-220)).Children(func() {
				ui.Column(c).Children(func() {
					if len(results) == 0 {
						ui.Column(c).Padding(32, 20).Gap(12).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, Icon("search")).Size(24, 24).TextColor(pal.TextFaint)
							ui.Text(c, locales.SearchNoResults(a.search.query)).FontSize(14)
							ui.Button(c, locales.SearchAvailable()).Disabled(a.search.available).OnClick(func() { a.searchAvailable(true) })
						})
					}
					group := ""
					for i, r := range results {
						if r.group != group {
							group = r.group
							ui.Text(c, strings.ToUpper(group)).FontSize(11).TextColor(pal.TextFaint).Padding(9, 20, 6, 20).BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider)
						}
						row := ui.ButtonBase(c.Key(r.id)).Height(44).Shrink(0).Padding(7, 20).Gap(12).AlignItems(ui.Center).Label(r.title).OnClick(func() { a.openSearchResult(r) })
						if i == selected {
							row.Background(pal.AccentSoft)
							if scroll {
								row.ScrollIntoView()
							}
						} else if row.Hovered() {
							row.Background(pal.Subtle)
						}
						row.Children(func() {
							iconOrLogo(c, r.icon, 16, t.TextMuted)
							ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
								ui.Text(c, r.title).FontSize(13).SingleLine()
								ui.Text(c, r.detail).FontSize(11).TextColor(pal.TextFaint).SingleLine()
							})
							if i == selected {
								ui.Text(c, locales.SearchOpen()).FontSize(11).TextColor(t.Accent)
							}
						})
					}
				})
			})
			ui.Row(c).Height(56).Shrink(0).Padding(10, 16).Gap(8).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider).Children(func() {
				ui.Text(c, locales.SearchKeys()+"   "+cmdKey("K")+" "+locales.SearchClose()).FontSize(11).TextColor(pal.TextFaint).Grow(1)
				ui.Text(c, locales.SearchCount(len(results))).FontSize(11).TextColor(pal.TextFaint)
			})
		})
	})
}

// availableSearchIndex uses lists already fetched or read from cache.
func (a *App) availableSearchIndex(now time.Time) []searchResult {
	key := a.searchKey(now)
	if a.search.availValid && a.search.availKey == key {
		return a.search.availCached
	}
	var out []searchResult
	add := func(tool, v string, tab *int) {
		if v == "" {
			return
		}
		out = append(out, searchResult{id: "available:" + tool + ":" + v, group: locales.SearchVersions(), title: tool + " " + v, detail: locales.AvailableTitle(), icon: "lucide/download", open: func() { *tab = 1; a.router.Push("/tools/" + tool) }})
	}
	if a.NodeVersions != nil {
		for _, r := range a.NodeVersions.Releases {
			if before, _, _ := strings.Cut(r.Version, "."); before != "" {
				if major, err := strconv.Atoi(before); err == nil {
					if line, ok := a.NodeVersions.Lines[major]; ok && line.Ended(now) {
						continue
					}
				}
			}
			add("node", r.Version, &a.nodeTab)
		}
	}
	if a.GoVersions != nil {
		for _, v := range a.GoVersions.Releases {
			add("go", v, &a.goTab)
		}
	}
	if a.BunLatest != nil {
		add("bun", a.BunLatest.Version, &a.bunTab)
	}
	for _, t := range []struct {
		id   string
		list *versions.Cycles
		tab  *int
	}{
		{"python", a.PythonVersions, &a.pythonTab}, {"php", a.PHPVersions, &a.phpTab}, {"rust", a.RustVersions, &a.rustTab}, {"ruby", a.RubyVersions, &a.rubyTab}, {"java", a.JavaVersions, &a.javaTab},
	} {
		if t.list != nil {
			for _, l := range t.list.Lines {
				if l.Ended(now) || (!l.Release.IsZero() && now.Before(l.Release)) {
					continue
				}
				add(t.id, l.Latest, t.tab)
			}
		}
	}
	if a.DotnetVersions != nil {
		for _, l := range a.DotnetVersions.Lines {
			if l.Ended(now) {
				continue
			}
			add("dotnet", l.Version, &a.dotnetTab)
		}
	}
	a.search.availKey = key
	a.search.availCached = out
	a.search.availValid = true
	return out
}

func (a *App) submitSearchAt(now time.Time) {
	results := a.searchResultsAt(now)
	if len(results) > 0 {
		a.openSearchResult(results[min(a.search.selected, len(results)-1)])
	}
}

// searchCopies are the tool's copies and, for a page of several tools,
// the tools it lists: kubectl on CLI & clouds, Xcode on Mobile
// development, the editors.
func searchCopies(tool inventory.ToolState) []inventory.Installation {
	out := slices.Clone(tool.Installations)
	if tool.Cpp != nil {
		out = append(out, tool.Cpp.Tools...)
	}
	if tool.Cli != nil {
		out = append(out, tool.Cli.Tools...)
	}
	if m := tool.Mobile; m != nil {
		for _, x := range m.Xcodes {
			out = append(out, inventory.Installation{Tool: "Xcode", Version: x.Version, Path: x.Path})
		}
		if m.CocoaPods != nil {
			out = append(out, *m.CocoaPods)
		}
		if m.Flutter != nil {
			out = append(out, m.Flutter.Installation)
		}
		if m.Android != nil && !m.Android.Missing {
			out = append(out, inventory.Installation{Tool: "Android SDK", Path: m.Android.Root})
		}
	}
	if ed := tool.Editors; ed != nil {
		for _, e := range ed.Editors {
			if e.App != "" {
				out = append(out, inventory.Installation{Tool: e.Name, Version: e.Version, Path: e.App})
			}
		}
	}
	if d := tool.Databases; d != nil {
		for _, db := range d.Servers {
			if db.Path == "" { // a service alone: its server is the row with a path
				continue
			}
			out = append(out, inventory.Installation{Tool: db.Kind, Version: db.Version, Path: db.Path, Source: inventory.Source{Manager: db.Source}})
		}
		out = append(out, d.Clients...)
	}
	return out
}
