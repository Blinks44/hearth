package ui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// searchResult keeps data and a deferred action, never a build's elements.
type searchResult struct {
	id, group, title, detail, icon, words string
	open                                  func()
}

type searchState struct {
	open      bool
	available bool // the lists of versions to install are searched
	query     string
	selected  int
	scroll    bool // the keyboard moved the selection: bring it into view
	recent    []string
}

func (a *App) toggleSearch() {
	a.search = searchState{open: !a.search.open, recent: a.search.recent}
}

// searchAvailable switches between the local results and the versions to
// install, from the first result.
func (a *App) searchAvailable(on bool) {
	a.search.available, a.search.selected = on, 0
}

func (a *App) searchIndex() []searchResult {
	var out []searchResult
	add := func(r searchResult) { out = append(out, r) }
	all := pages()
	for _, p := range all {
		add(searchResult{id: p.path, group: locales.SearchTools(), title: p.title(), detail: p.subtitle(), icon: p.icon, open: func() { a.router.Push(p.path) }})
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
				add(searchResult{id: "version:" + tool.Tool + ":" + in.Path, group: locales.SearchVersions(), title: strings.TrimSpace(name + " " + in.Version), detail: detail, icon: "lucide/box", open: func() { a.router.Push(target) }})
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
	for _, sk := range a.allSkills() {
		if len(sk.Paths) == 0 {
			continue
		}
		add(searchResult{id: "skill:" + skillKey(sk), group: locales.SkillsTitle(), title: sk.Name, detail: sk.Description, words: sk.Project, icon: "lucide/puzzle", open: func() {
			a.skillAgent, a.skillQuery, a.skillScope = "", "", 0
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
	add(searchResult{id: "action:scan", group: locales.SearchActions(), title: locales.Scan(), detail: locales.OverviewSubtitle(), icon: "lucide/refresh-cw", open: a.Scan})
	if a.ChooseFolder != nil {
		add(searchResult{id: "action:project", group: locales.SearchActions(), title: locales.AddProjectTitle(), icon: "lucide/folder-open", open: a.ChooseFolder})
	}
	add(searchResult{id: "action:versions", group: locales.SearchActions(), title: locales.SearchAvailable(), detail: locales.SearchVersions(), words: "version", icon: "lucide/download", open: func() {
		a.search.open, a.search.query = true, ""
		a.searchAvailable(true)
	}})
	return out
}

func (a *App) searchResults() []searchResult {
	all := a.searchIndex()
	if a.search.available {
		all = a.availableSearchIndex()
	}
	q := strings.Fields(strings.ToLower(a.search.query))
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
	for _, r := range all {
		text := strings.ToLower(r.title + " " + r.detail + " " + r.words)
		if !slices.ContainsFunc(q, func(word string) bool { return !strings.Contains(text, word) }) {
			out = append(out, r)
		}
		if len(out) == 60 {
			break
		}
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
			ui.Row(c).Padding(14, 16, 12, 16).Children(func() {
				ui.SearchField(c.Key("global-query"), &a.search.query).Label(locales.SearchPlaceholder()).Placeholder(locales.SearchPlaceholder()).AutoFocus().Grow(1).Height(37).
					OnChange(func() { a.search.selected = 0 }).OnSubmit(a.submitSearch)
			})
			if a.search.available {
				ui.Button(c, locales.SearchLocal()).Margin(0, 16, 8, 16).OnClick(func() { a.searchAvailable(false) })
			}
			results := a.searchResults()
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
func (a *App) availableSearchIndex() []searchResult {
	var out []searchResult
	add := func(tool, v string, tab *int) {
		if v == "" {
			return
		}
		out = append(out, searchResult{id: "available:" + tool + ":" + v, group: locales.SearchVersions(), title: tool + " " + v, detail: locales.AvailableTitle(), icon: "lucide/download", open: func() { *tab = 1; a.router.Push("/tools/" + tool) }})
	}
	if a.NodeVersions != nil {
		for _, r := range a.NodeVersions.Releases {
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
				add(t.id, l.Latest, t.tab)
			}
		}
	}
	if a.DotnetVersions != nil {
		for _, l := range a.DotnetVersions.Lines {
			add("dotnet", l.Version, &a.dotnetTab)
		}
	}
	return out
}

func (a *App) submitSearch() {
	results := a.searchResults()
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
