package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// mcpItem is an MCP server as the list shows it: one name in one scope,
// with each agent's declaration of it.
type mcpItem struct {
	name, project string
	decls         []inventory.MCPServer
}

func (m mcpItem) key() string { return m.project + "\x00" + m.name }

// conflict reports that the agents declare it differently.
func (m mcpItem) conflict() bool {
	return slices.ContainsFunc(m.decls, func(s inventory.MCPServer) bool { return launch(s) != launch(m.decls[0]) })
}

// missing reports that a declaration in use starts a command that is not
// there; off, that every one is turned off.
func (m mcpItem) missing() bool {
	return slices.ContainsFunc(m.decls, func(s inventory.MCPServer) bool { return s.Missing && !s.Disabled })
}

func (m mcpItem) off() bool {
	return !slices.ContainsFunc(m.decls, func(s inventory.MCPServer) bool { return !s.Disabled })
}

func (m mcpItem) agents() []string {
	var out []string
	for _, s := range m.decls {
		if !slices.Contains(out, s.Agent) {
			out = append(out, s.Agent)
		}
	}
	return out
}

// launch is how a server starts: its command line, or its URL.
func launch(s inventory.MCPServer) string {
	if s.Transport != "stdio" {
		return s.URL
	}
	return strings.Join(append([]string{s.Command}, s.Args...), " ")
}

// mcpItems groups the servers by scope and name: the projects' first, in
// the order of the projects, then those Claude Code keeps of other
// folders, then the user's.
func (a *App) mcpItems() []mcpItem {
	var items []mcpItem
	for _, s := range a.allServers() {
		i := slices.IndexFunc(items, func(m mcpItem) bool { return m.name == s.Name && m.project == s.Project })
		if i < 0 {
			items = append(items, mcpItem{name: s.Name, project: s.Project})
			i = len(items) - 1
		}
		items[i].decls = append(items[i].decls, s)
	}
	rank := func(m mcpItem) int {
		if m.project == "" {
			return len(a.Projects) + 1
		}
		if i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == m.project }); i >= 0 {
			return i
		}
		return len(a.Projects)
	}
	slices.SortStableFunc(items, func(x, y mcpItem) int { return rank(x) - rank(y) })
	return items
}

// mcpPageView lists the MCP servers by scope, filtered by agent, scope and
// text, with the chosen one's details beside them.
func (a *App) mcpPageView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		all := a.mcpItems()
		if len(all) == 0 && (a.Snapshot == nil || len(a.Snapshot.MCPErrs) == 0) {
			a.aiEmpty(c, "plug", locales.NoServers())
			return
		}
		var found []issues.Issue
		for _, is := range a.issues(c) {
			if len(is.MCP) > 0 {
				found = append(found, is)
			}
		}
		inProjects := 0
		for _, m := range all {
			if m.project != "" {
				inProjects++
			}
		}
		q := strings.ToLower(strings.TrimSpace(a.mcpQuery))
		var shown []mcpItem
		for _, m := range all {
			if !inScope(a.mcpScope, m.project) ||
				(a.mcpAgent != "" && !slices.Contains(m.agents(), a.mcpAgent)) ||
				(q != "" && !strings.Contains(strings.ToLower(m.name+" "+launch(m.decls[0])), q)) {
				continue
			}
			shown = append(shown, m)
		}
		chosen := slices.IndexFunc(shown, func(m mcpItem) bool { return m.key() == a.mcpChosen })
		if chosen < 0 && len(shown) > 0 {
			chosen = 0
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				aiIntro(c, "plug", locales.McpTitle(), locales.McpText(), []aiStat{
					{strconv.Itoa(len(all)), locales.StatServers()},
					{strconv.Itoa(inProjects), locales.StatProjects()},
					{strconv.Itoa(len(found)), locales.StatProblems()},
				})
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					a.agentFilter(c, "mcp-agent", &a.mcpAgent, func(id string) int {
						n := 0
						for _, m := range all {
							if id == "" || slices.Contains(m.agents(), id) {
								n++
							}
						}
						return n
					})
					scopeSegments(c, "mcp-scope", &a.mcpScope)
				})
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.mcpList(c, shown, chosen)
						a.mcpProblems(c, found)
					})
					detailColumn(c).Children(func() {
						if chosen >= 0 {
							a.mcpDetail(c, shown[chosen])
						}
					})
				})
			})
		})
	})
}

// mcpList is the search field and the servers by scope.
func (a *App) mcpList(c *ui.Context, shown []mcpItem, chosen int) {
	t := c.Theme()
	pal := Colors(t)
	listFrame(c, func() {
		ui.Row(c).Padding(6, 10).Children(func() {
			ui.SearchField(c.Key("mcp-search"), &a.mcpQuery).Bind(&a.filterField).Placeholder(locales.SearchServers()).Grow(1)
		})
		if len(shown) == 0 {
			cardNote(c, locales.NothingMatches())
		}
		for i, m := range shown {
			if i == 0 || shown[i-1].project != m.project {
				groupTitle(c, scopeName(m.project), a.mcpFiles(shown, m.project))
			}
			e := ui.ButtonBase(c.Key("mcp:"+m.key())).Gap(10).Padding(9, 14, 9, 12).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(m.name)
			switch {
			case i == chosen:
				e.Background(pal.ActiveRow)
			case e.Hovered():
				e.Background(pal.Subtle)
			}
			first := m.decls[0]
			e.Children(func() {
				icon := "terminal"
				if first.Transport != "stdio" {
					icon = "globe"
				}
				ui.Box(c).Size(27, 27).Shrink(0).Radius(7).Center().Background(pal.Subtle).
					BorderWidth(1).BorderColor(pal.Line).Children(func() {
					ui.Icon(c, Icon(icon)).Size(15, 15).TextColor(t.TextMuted)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, m.name).FontSize(13.5).SingleLine()
					ui.Text(c, first.Transport+" · "+tilde(launch(first), a.Home)).Font(FontMono).FontSize(11).
						TextColor(pal.TextFaint).SingleLine()
				})
				switch {
				case m.missing():
					aiTag(c, locales.TagNoCommand(), pal.WarningSoft, pal.WarningText)
				case m.conflict():
					aiTag(c, locales.TagConflict(), pal.WarningSoft, pal.WarningText)
				case m.off():
					aiTag(c, locales.TagOff(), pal.Muted, t.TextMuted)
				}
				agentAvatars(c, m.agents())
			})
			if e.Clicked() {
				a.mcpChosen = m.key()
			}
		}
	})
}

// mcpFiles are the files declaring the servers of a scope, as
// "~/.claude.json · ~/.codex/config.toml", relative to the project for a
// project's, and Claude Code's of one folder marked local.
func (a *App) mcpFiles(items []mcpItem, project string) string {
	var out []string
	for _, m := range items {
		if m.project != project {
			continue
		}
		for _, s := range m.decls {
			f := a.declFile(s)
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	return strings.Join(out, " · ")
}

// declFile is the file a server is declared in, as the list shows it.
func (a *App) declFile(s inventory.MCPServer) string {
	if s.Project != "" {
		if rel, err := filepath.Rel(s.Project, s.File); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return tilde(s.File, a.Home) + " (" + locales.ScopeLocal() + ")"
	}
	return tilde(s.File, a.Home)
}

// mcpProblems are the MCP issues, and the files that could not be read.
func (a *App) mcpProblems(c *ui.Context, found []issues.Issue) {
	t := c.Theme()
	pal := Colors(t)
	var lines []string
	for _, is := range found {
		title, _ := issueText(is, a.Home)
		lines = append(lines, title)
	}
	if a.Snapshot != nil {
		lines = append(lines, a.Snapshot.MCPErrs...)
	}
	if len(lines) == 0 {
		return
	}
	ui.Row(c).Gap(10).Padding(10, 12).Radius(12).AlignItems(ui.Start).Background(pal.WarningSoft).Children(func() {
		ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(t.Warning)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			for _, l := range lines {
				ui.Text(c, l).FontSize(12).TextColor(pal.WarningText)
			}
		})
	})
}

// mcpDetail is the chosen server: how it starts, its variables with their
// values hidden, which agents declare it and where, and actions.
func (a *App) mcpDetail(c *ui.Context, m mcpItem) {
	t := c.Theme()
	pal := Colors(t)
	first := m.decls[0]
	listFrame(c, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m.name).FontSize(17).FontWeight(500).Shrink(1).MinWidth(0).SingleLine()
				aiTag(c, scopeName(m.project)+" · "+first.Transport, pal.AccentSoft, pal.AccentInk)
			})
			text := locales.DeclaredBy(len(m.agents()))
			if m.conflict() {
				text += " " + locales.CommandsDiffer()
			}
			ui.Text(c, text).FontSize(12.5).TextColor(t.TextMuted)
			ui.Row(c).AlignSelf(ui.Start).Gap(6).Padding(3, 4, 3, 8).Radius(6).AlignItems(ui.Center).MaxWidth(390).
				BorderWidth(1).BorderColor(pal.ControlBorder).Children(func() {
				ui.Icon(c, Icon("terminal")).Size(13, 13).Shrink(0).TextColor(t.TextMuted)
				clipped(c, launch(first)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).Shrink(1).MinWidth(0)
				iconButton(c, "copy", locales.CopyPath()).OnClick(func() {
					c.WriteClipboard(launch(first))
					c.Toast(locales.PathCopied())
				})
			})
		})
		detailSection(c, locales.VariablesHidden())
		var env, headers []string
		for _, s := range m.decls {
			for _, n := range s.Env {
				if !slices.Contains(env, n) {
					env = append(env, n)
				}
			}
			for _, n := range s.Headers {
				if !slices.Contains(headers, n) {
					headers = append(headers, n)
				}
			}
		}
		if len(env)+len(headers) == 0 {
			ui.Text(c, locales.NoVariables()).FontSize(12).TextColor(pal.TextFaint).Padding(4, 16, 10, 16)
		}
		variable := func(name, caption string) {
			ui.Row(c).Gap(8).Padding(5, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, "••••••"+caption).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			})
		}
		for _, n := range env {
			variable(n, "")
		}
		for _, n := range headers {
			variable(n, "  "+locales.HeaderCaption())
		}
		detailSection(c, locales.WhereDeclared())
		ids := a.foundIDs()
		for _, id := range m.agents() {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			i := slices.IndexFunc(m.decls, func(s inventory.MCPServer) bool { return s.Agent == id })
			ui.Row(c).Gap(10).Padding(5, 16).AlignItems(ui.Center).Children(func() {
				agentTile(c, id, 24)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, agentTitle(id)).FontSize(13).SingleLine()
					if i < 0 {
						ui.Text(c, locales.NotDeclared()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
						return
					}
					s := m.decls[i]
					caption, color := a.declFile(s)+" · "+tilde(s.Command, a.Home), pal.TextFaint
					if s.Transport != "stdio" {
						caption = a.declFile(s)
					}
					switch {
					case s.Missing:
						caption, color = caption+" · "+locales.CommandMissing(), pal.WarningText
					case launch(s) != launch(first):
						color = pal.WarningText
					}
					ui.Text(c, caption).Font(FontMono).FontSize(11).TextColor(color).SingleLine()
				})
				if i >= 0 && m.decls[i].Disabled {
					aiTag(c, locales.TagOff(), pal.Muted, t.TextMuted)
				}
			})
		}
		if a.Shell != nil {
			ui.Row(c).Gap(8).Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c.Key("mcp-open"), "").OnClick(func() { a.openFile(c, first.File) }).Children(func() {
					ui.Icon(c, Icon("external-link")).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, locales.OpenFile(filepath.Base(first.File))).SingleLine()
				})
				ui.Button(c.Key("mcp-reveal"), "").OnClick(func() { a.Shell.ShowItemInFolder(first.File) }).Children(func() {
					ui.Icon(c, Icon("folder-open")).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, showInFolder()).SingleLine()
				})
			})
		}
	})
}
