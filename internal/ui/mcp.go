package ui

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// mcpItem is an MCP server as the list shows it: one name in one scope,
// with each agent's declaration of it.
type mcpItem struct {
	name, project string
	builtin       bool
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
		if s.Agent != "" && !slices.Contains(out, s.Agent) {
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
	if a.HearthExecutable != "" && !slices.ContainsFunc(items, func(m mcpItem) bool { return m.name == "hearth" && m.project == "" }) {
		items = append(items, mcpItem{name: "hearth", builtin: true, decls: []inventory.MCPServer{{Name: "hearth", Transport: "stdio", Command: a.HearthExecutable, Args: []string{"mcp"}}}})
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
	slices.SortStableFunc(items, func(x, y mcpItem) int { return cmp.Or(rank(x)-rank(y), a.byTitle(x.name, y.name)) })
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
					{strconv.Itoa(len(all)), locales.StatServers(len(all))},
					{strconv.Itoa(inProjects), locales.StatProjects()},
					{strconv.Itoa(len(found)), locales.StatProblems(len(found))},
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
				case m.builtin:
					aiTag(c, locales.HearthMCPAvailable(), pal.Info.Alpha(0.12), pal.Info)
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
		if m.project != project || m.builtin {
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
			if m.builtin {
				text = locales.HearthMCPDescription()
			}
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
		a.mcpSecrets(c, m)
		if !m.builtin {
			a.mcpCheckView(c, m)
		}
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
		// plain reports a value of the name some declaration writes out.
		plain := func(name string, header bool) bool {
			return slices.ContainsFunc(m.decls, func(s inventory.MCPServer) bool {
				if header {
					return slices.Contains(s.PlainHeaders, name)
				}
				return slices.Contains(s.PlainEnv, name)
			})
		}
		variable := func(name, caption string, plain bool) {
			ui.Row(c).Gap(8).Padding(5, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0).SingleLine()
				color := pal.TextFaint
				if plain {
					caption, color = "  "+locales.PlainText(), pal.WarningText
				}
				ui.Text(c, "••••••"+caption).Font(FontMono).FontSize(11).TextColor(color).SingleLine()
			})
		}
		for _, n := range env {
			variable(n, "", plain(n, false))
		}
		for _, n := range headers {
			variable(n, "  "+locales.HeaderCaption(), plain(n, true))
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
				a.mcpAgentControl(c, m, id, i)
			})
		}
		if a.Shell != nil && !m.builtin {
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

// mcpSecrets warns of the secrets m's declarations write out, each with
// the button that moves them into variables where Hearth can.
func (a *App) mcpSecrets(c *ui.Context, m mcpItem) {
	t := c.Theme()
	pal := Colors(t)
	for _, s := range m.decls {
		if !s.PlainSecret() {
			continue
		}
		ui.Row(c).Gap(10).Margin(0, 16, 12, 16).Padding(10, 10, 10, 12).Radius(10).AlignItems(ui.Center).
			Background(pal.WarningSoft).Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(t.Warning)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.McpSecretTitle()+" · "+agentTitle(s.Agent)).FontSize(12.5).FontWeight(500).TextColor(pal.WarningText)
				text := a.declFile(s) + " · " + secretWhat(s)
				if !ops.SecretMovable(s) || s.PlainArgs {
					text += ". " + locales.SecretByHand()
				}
				ui.Text(c, text).FontSize(11.5).TextColor(pal.WarningText)
			})
			if a.BuildPlan != nil && ops.SecretMovable(s) {
				ui.Button(c.Key("mcp-secret:"+s.Agent), "").Height(24).Padding(0, 10).Shrink(0).Background(t.Text).BorderColor(t.Text).
					Label(locales.SecretMove()).OnClick(func() { a.moveSecrets(s) }).Children(func() {
					ui.Text(c, locales.SecretMove()).FontSize(12).TextColor(t.Background).SingleLine()
				})
			}
		})
	}
}

// mcpAgentControl is what the agent id can do with m, i its declaration
// in m.decls or -1: connect the server as another agent declares it,
// turn it on or off where the agent has a setting for it, remove it, and
// open the file that declares it. The plans are made in the background:
// they read the files.
func (a *App) mcpAgentControl(c *ui.Context, m mcpItem, id string, i int) {
	plans := a.BuildPlan != nil && id != "aider"
	propose := func(build func(env sysenv.Env) (ops.Plan, error)) {
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) { return build(env) }, a.proposeOrSay)
	}
	if i < 0 {
		if plans {
			from := m.decls[0]
			ui.Button(c.Key("mcp-connect:"+id), locales.SkillConnect()).Shrink(0).OnClick(func() {
				propose(func(env sysenv.Env) (ops.Plan, error) {
					if m.builtin {
						return ops.HearthMCPPlan(a.HearthExecutable, id, env, agentTitle)
					}
					return ops.MCPConnectPlan(from, id, env, agentTitle)
				})
			})
		}
		return
	}
	s := m.decls[i]
	if plans && ops.MCPToggles(s) {
		on := !s.Disabled
		toggle(c, "mcp-on:"+id, &on).Label(locales.McpOnFor(agentTitle(id))).OnChange(func() {
			propose(func(env sysenv.Env) (ops.Plan, error) { return ops.MCPEnablePlan(s, on, env, agentTitle) })
		})
	}
	removable := plans && ops.MCPRemovable(s)
	if a.Shell == nil && !removable {
		return
	}
	ui.Row(c.Key("mcp-menu:" + id)).Shrink(0).Children(func() {
		iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
			if a.Shell != nil {
				if menu.Item(locales.OpenFile(filepath.Base(s.File))).Chosen() {
					a.openFile(c, s.File)
				}
				if menu.Item(showInFolder()).Chosen() {
					a.Shell.ShowItemInFolder(s.File)
				}
			}
			if removable {
				if a.Shell != nil {
					menu.Separator()
				}
				if menu.Item(locales.McpRemove(agentTitle(id))).Chosen() {
					propose(func(env sysenv.Env) (ops.Plan, error) { return ops.MCPRemovePlan(s, env, agentTitle) })
				}
			}
		})
	})
}

// mcpCheck is the last check of an MCP server: the key of its item, and,
// once it is done, what the server answered or why it failed.
type mcpCheck struct {
	key      string
	running  bool
	res      ai.MCPCheck
	err      error
	allTools bool
}

const (
	// shownTools is how many tools a check lists before "Show N more".
	shownTools = 8
	// shownErrLines is how many last lines of a failure it shows.
	shownErrLines = 12
)

// mcpCheckView checks m on a button: Hearth starts, or reaches, its first
// declaration in use as the agent would, and lists its tools or says why
// it does not answer.
func (a *App) mcpCheckView(c *ui.Context, m mcpItem) {
	if a.CheckMCP == nil {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	s := m.decls[0]
	if i := slices.IndexFunc(m.decls, func(s inventory.MCPServer) bool { return !s.Disabled }); i >= 0 {
		s = m.decls[i]
	}
	ch := a.mcpCheck
	if ch.key != m.key() {
		ch = mcpCheck{}
	}
	check := func() {
		key := m.key()
		a.mcpCheck = mcpCheck{key: key, running: true}
		a.CheckMCP(s, func(res ai.MCPCheck, err error) {
			if a.mcpCheck.key == key {
				a.mcpCheck = mcpCheck{key: key, res: res, err: err}
			}
		})
	}
	detailSection(c, locales.McpCheckSection())
	// The icon stays by the first line of a long message.
	ui.Row(c).Gap(8).Padding(0, 16, 10, 16).AlignItems(ui.Start).Children(func() {
		text := func(s string, color ui.Color) {
			ui.Text(c, s).FontSize(12.5).TextColor(color).Grow(1).MinWidth(0)
		}
		switch {
		case ch.running:
			ui.Spinner(c).Label(locales.McpChecking())
			text(locales.McpChecking(), t.TextMuted)
		case ch.key == "":
			text(locales.McpCheckHint(agentTitle(s.Agent)), t.TextMuted)
		case ch.err == nil:
			ui.Icon(c, Icon("circle-check")).Size(14, 14).Shrink(0).Margin(2, 0, 0, 0).TextColor(t.Success)
			name := strings.TrimSpace(ch.res.Server + " " + ch.res.Version)
			text(locales.McpAnswered(cmp.Or(name, m.name), seconds(ch.res.Took), len(ch.res.Tools)), t.Text)
		default:
			ui.Icon(c, Icon("circle-alert")).Size(14, 14).Shrink(0).Margin(2, 0, 0, 0).TextColor(pal.WarningText)
			text(mcpFailure(ch.err), pal.WarningText)
		}
		if !ch.running {
			label := locales.McpCheck()
			if ch.key != "" {
				label = locales.McpCheckAgain()
			}
			ui.Button(c.Key("mcp-check"), label).Shrink(0).OnClick(check)
		}
	})
	switch {
	case ch.running || ch.key == "":
	case ch.err != nil:
		lines := strings.Split(ch.err.Error(), "\n")
		lines = lines[max(0, len(lines)-shownErrLines):]
		ui.Text(c, strings.Join(lines, "\n")).Font(FontMono).FontSize(11).TextColor(pal.MonoText).
			Margin(0, 16, 12, 16).Padding(8, 10).Radius(6).Background(pal.Subtle).Selectable()
	case len(ch.res.Tools) == 0:
		ui.Text(c, locales.McpNoTools()).FontSize(12).TextColor(pal.TextFaint).Padding(0, 16, 10, 16)
	default:
		tools := ch.res.Tools
		if !ch.allTools && len(tools) > shownTools {
			tools = tools[:shownTools]
		}
		for _, tool := range tools {
			ui.Row(c).Gap(10).Padding(3, 16).AlignItems(ui.Baseline).Children(func() {
				clipped(c, tool.Name).Font(FontMono).FontSize(12).Shrink(0).MaxWidth(200)
				clipped(c, tool.Description).FontSize(11.5).TextColor(pal.TextFaint).Shrink(1).MinWidth(0)
			})
		}
		if more := len(ch.res.Tools) - len(tools); more > 0 {
			ui.Row(c).Padding(4, 16, 10, 16).Children(func() {
				ui.Button(c.Key("mcp-tools-more"), locales.McpMoreTools(more)).OnClick(func() { a.mcpCheck.allTools = true })
			})
		} else {
			ui.Box(c).Height(8)
		}
	}
}

// mcpFailure says why a check failed.
func mcpFailure(err error) string {
	e, ok := errors.AsType[*ai.MCPError](err)
	if !ok {
		return locales.McpFailed()
	}
	switch e.Kind {
	case ai.MCPNotFound:
		return locales.McpFailNotFound()
	case ai.MCPExited:
		return locales.McpFailExited()
	case ai.MCPTimedOut:
		return locales.McpFailTimedOut()
	case ai.MCPUnreachable:
		return locales.McpFailUnreachable()
	case ai.MCPDenied:
		return locales.McpFailDenied()
	}
	return locales.McpFailed()
}
