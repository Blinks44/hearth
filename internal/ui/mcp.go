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

// mcpSelectedDecl returns the declaration index currently chosen for checking m.
func (a *App) mcpSelectedDecl(m mcpItem) int {
	idx := 0
	if i := slices.IndexFunc(m.decls, func(s inventory.MCPServer) bool { return !s.Disabled }); i >= 0 {
		idx = i
	}
	if a.mcpCheckDecl != nil {
		if saved, ok := a.mcpCheckDecl[m.key()]; ok && saved >= 0 && saved < len(m.decls) {
			idx = saved
		}
	}
	return idx
}

func mcpDeclKey(m mcpItem, idx int) string {
	s := m.decls[idx]
	return m.key() + "\x00" + strconv.Itoa(idx) + "\x00" + s.Agent + "\x00" + s.File
}

func (a *App) startMCPCheck(m mcpItem, idx int) {
	if a.CheckMCP == nil || idx < 0 || idx >= len(m.decls) {
		return
	}
	s := m.decls[idx]
	key := mcpDeclKey(m, idx)
	if a.mcpChecks == nil {
		a.mcpChecks = map[string]mcpCheck{}
	}
	if prev, ok := a.mcpChecks[key]; ok && prev.running {
		return
	}
	a.mcpChecks[key] = mcpCheck{key: key, running: true}
	a.CheckMCP(s, func(res ai.MCPCheck, err error) {
		if a.mcpChecks == nil {
			return
		}
		if cur, ok := a.mcpChecks[key]; ok && cur.key == key {
			a.mcpChecks[key] = mcpCheck{key: key, res: res, err: err}
		}
	})
}

func (a *App) mcpPageView(c *ui.Context, p page) {
	all := a.mcpItems()
	var checkable []mcpItem
	for _, m := range all {
		if !m.builtin && !m.off() {
			checkable = append(checkable, m)
		}
	}
	anyChecking := false
	for _, m := range checkable {
		if ch, ok := a.mcpChecks[mcpDeclKey(m, a.mcpSelectedDecl(m))]; ok && ch.running {
			anyChecking = true
			break
		}
	}
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if a.CheckMCP != nil && len(checkable) > 0 {
				items := slices.Clone(checkable)
				ui.Button(c.Key("mcp-check-all"), locales.McpCheckAll()).Height(controlHeight).Disabled(anyChecking).OnClick(func() {
					for _, m := range items {
						a.startMCPCheck(m, a.mcpSelectedDecl(m))
					}
				})
			}
			if a.BuildPlan != nil {
				ui.PrimaryButton(c.Key("mcp-add"), locales.McpAddButton()).Height(controlHeight).OnClick(func() {
					a.openAddMCPForm()
				})
			}
		}})
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
				(a.mcpAgent != "" && !slices.Contains(m.agents(), a.mcpAgent)) {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(m.name), q) &&
				!slices.ContainsFunc(m.decls, func(s inventory.MCPServer) bool {
					return strings.Contains(strings.ToLower(launch(s)), q)
				}) {
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
				if !m.builtin {
					if ch, ok := a.mcpChecks[mcpDeclKey(m, a.mcpSelectedDecl(m))]; ok {
						switch {
						case ch.running:
							ui.Spinner(c).Label(locales.McpChecking())
						case ch.err == nil:
							aiTag(c, locales.McpStatusOK(len(ch.res.Tools)), pal.SuccessSoft, pal.SuccessText)
						default:
							aiTag(c, locales.McpStatusFailed(), pal.WarningSoft, pal.WarningText)
						}
					}
				}
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
				icon, copyLabel, copiedToast := "terminal", locales.McpCopyCommand(), locales.McpCommandCopied()
				if first.Transport != "stdio" {
					icon, copyLabel, copiedToast = "globe", locales.McpCopyURL(), locales.McpURLCopied()
				}
				ui.Icon(c, Icon(icon)).Size(13, 13).Shrink(0).TextColor(t.TextMuted)
				clipped(c, launch(first)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).Shrink(1).MinWidth(0)
				iconButton(c, "copy", copyLabel).OnClick(func() {
					c.WriteClipboard(launch(first))
					c.Toast(copiedToast)
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
			ui.Row(c).Gap(8).Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Button(c.Key("mcp-open"), "").Shrink(0).OnClick(func() { a.openFile(c, first.File) }).Children(func() {
					ui.Icon(c, Icon("external-link")).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, locales.OpenFile(filepath.Base(first.File))).SingleLine()
				})
				ui.Button(c.Key("mcp-reveal"), "").Shrink(0).OnClick(func() { a.Shell.ShowItemInFolder(first.File) }).Children(func() {
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
	editable := plans && ops.MCPEditable(s)
	if a.Shell == nil && !removable && !editable {
		return
	}
	ui.Row(c.Key("mcp-menu:" + id)).Shrink(0).Children(func() {
		iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
			if editable {
				if menu.Item(locales.McpEditEllipsis()).Chosen() {
					a.openEditMCPForm(s)
				}
			}
			if a.Shell != nil {
				if editable {
					menu.Separator()
				}
				if menu.Item(locales.OpenFile(filepath.Base(s.File))).Chosen() {
					a.openFile(c, s.File)
				}
				if menu.Item(showInFolder()).Chosen() {
					a.Shell.ShowItemInFolder(s.File)
				}
			}
			if removable {
				if a.Shell != nil || editable {
					menu.Separator()
				}
				if menu.Item(locales.McpRemove(agentTitle(id))).Chosen() {
					propose(func(env sysenv.Env) (ops.Plan, error) { return ops.MCPRemovePlan(s, env, agentTitle) })
				}
			}
		})
	})
}

// mcpCheck is the last check of an MCP server: the key of its item and
// declaration, and once done, what the server answered or why it failed.
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

// mcpCheckView checks m on a button: Hearth starts, or reaches, the chosen
// declaration as the agent would, and lists its tools or says why it does not
// answer. When multiple declarations exist, the user can select which one to
// test.
func (a *App) mcpCheckView(c *ui.Context, m mcpItem) {
	if a.CheckMCP == nil {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	idx := a.mcpSelectedDecl(m)
	detailSection(c, locales.McpCheckSection())
	if len(m.decls) > 1 {
		agentCounts := map[string]int{}
		for _, d := range m.decls {
			agentCounts[d.Agent]++
		}
		segs := make([]segment, len(m.decls))
		for i, d := range m.decls {
			label := agentTitle(d.Agent)
			if agentCounts[d.Agent] > 1 && d.File != "" {
				label += " · " + filepath.Base(d.File)
			}
			segs[i] = segment{label: label}
		}
		sel := idx
		ui.Row(c).Padding(0, 16, 8, 16).Wrap().Children(func() {
			segments(c, "mcp-check-decl:"+m.key(), &sel, segs).OnChange(func() {
				if a.mcpCheckDecl == nil {
					a.mcpCheckDecl = map[string]int{}
				}
				a.mcpCheckDecl[m.key()] = sel
			})
		})
	}
	s := m.decls[idx]
	declKey := mcpDeclKey(m, idx)
	ch := a.mcpChecks[declKey]
	check := func() {
		a.startMCPCheck(m, idx)
	}
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
				key := declKey
				ui.Button(c.Key("mcp-tools-more"), locales.McpMoreTools(more)).OnClick(func() {
					if cur, ok := a.mcpChecks[key]; ok {
						cur.allTools = true
						a.mcpChecks[key] = cur
					}
				})
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

var mcpCapableAgents = []string{"claude", "codex", "copilot", "gemini", "antigravity", "cursor", "opencode", "devin", "grok", "kimi"}

type mcpFormState struct {
	open       bool
	edit       bool
	loading    bool
	err        string
	orig       inventory.MCPServer
	name       string
	agent      int
	agentLabel string
	scope      int
	scopeLabel string
	transport  int // 0=stdio, 1=http, 2=sse
	command    string
	args       string
	url        string
	env        string
	headers    string
}

func (a *App) defaultMCPAgentIndex() int {
	for _, id := range a.foundIDs() {
		if idx := slices.Index(mcpCapableAgents, id); idx >= 0 {
			return idx
		}
	}
	return 0
}

func (a *App) openAddMCPForm() {
	idx := a.defaultMCPAgentIndex()
	a.mcpForm = &mcpFormState{
		open:       true,
		agent:      idx,
		agentLabel: agentTitle(mcpCapableAgents[idx]),
		scopeLabel: locales.ScopeUser(),
	}
}

func (a *App) openEditMCPForm(s inventory.MCPServer) {
	f := &mcpFormState{
		open:    true,
		edit:    true,
		loading: true,
		orig:    s,
		name:    s.Name,
		agent:   max(0, slices.Index(mcpCapableAgents, s.Agent)),
	}
	a.mcpForm = f
	if a.BuildPlan == nil {
		f.loading = false
		return
	}
	var loaded ai.MCPDef
	a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		var err error
		loaded, err = ai.ReadMCPDef(s)
		return ops.Plan{}, err
	}, func(_ ops.Plan, err error) {
		if a.mcpForm != f {
			return
		}
		f.loading = false
		if err != nil {
			f.err = err.Error()
			return
		}
		switch loaded.Transport {
		case "http":
			f.transport = 1
		case "sse":
			f.transport = 2
		default:
			f.transport = 0
		}
		f.command = loaded.Command
		f.args = joinMCPArgs(loaded.Args)
		f.url = loaded.URL
		f.env = formatKVLines(loaded.Env, "=")
		f.headers = formatKVLines(loaded.Headers, ": ")
	})
}

func (a *App) mcpFormDialog(c *ui.Context) {
	f := a.mcpForm
	if f == nil || !f.open {
		return
	}
	t, pal := c.Theme(), Colors(c.Theme())
	dialogBase(c.Key("mcp-form"), &f.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(560).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		title := locales.McpFormAddTitle()
		if f.edit {
			title = locales.McpFormEditTitle(f.orig.Name)
		}
		agentOptions := make([]string, len(mcpCapableAgents))
		for i, id := range mcpCapableAgents {
			agentOptions[i] = agentTitle(id)
		}
		if f.agent < 0 || f.agent >= len(agentOptions) {
			f.agent = 0
		}
		f.agentLabel = agentOptions[f.agent]
		scopeOptions := []string{locales.ScopeUser()}
		for _, pr := range a.Projects {
			scopeOptions = append(scopeOptions, filepath.Base(pr.Dir))
		}
		if f.scope < 0 || f.scope >= len(scopeOptions) {
			f.scope = 0
		}
		f.scopeLabel = scopeOptions[f.scope]
		transportNames := []string{"stdio", "http", "sse"}
		submit := func() {
			f.err = ""
			def := ai.MCPDef{Transport: transportNames[f.transport]}
			if def.Transport == "stdio" {
				def.Command = strings.TrimSpace(f.command)
				def.Args = splitMCPArgs(f.args)
				envMap, err := parseKVLines(f.env, false)
				if err != nil {
					f.err = err.Error()
					return
				}
				def.Env = envMap
			} else {
				def.URL = strings.TrimSpace(f.url)
				hdrMap, err := parseKVLines(f.headers, true)
				if err != nil {
					f.err = err.Error()
					return
				}
				def.Headers = hdrMap
			}
			if a.BuildPlan == nil {
				return
			}
			if f.edit {
				orig := f.orig
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.MCPEditPlan(orig, def, env, agentTitle)
				}, func(p ops.Plan, err error) {
					if err != nil {
						f.err = err.Error()
						return
					}
					f.open = false
					a.Propose(p)
				})
				return
			}
			agentID := mcpCapableAgents[f.agent]
			project := ""
			if f.scope > 0 && f.scope-1 < len(a.Projects) {
				project = a.Projects[f.scope-1].Dir
			}
			name := strings.TrimSpace(f.name)
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.MCPAddPlan(name, agentID, project, def, env, agentTitle)
			}, func(p ops.Plan, err error) {
				if err != nil {
					f.err = err.Error()
					return
				}
				f.open = false
				a.Propose(p)
			})
		}
		ui.Column(c).Children(func() {
			ui.Scroll(c).MaxHeight(560).Children(func() {
				ui.Column(c).Gap(12).Padding(20, 20, 16, 20).Children(func() {
					ui.Text(c, title).FontSize(18).FontWeight(600)
					if f.loading {
						ui.Text(c, locales.OpVerifying()).FontSize(12).TextColor(t.TextMuted)
						return
					}
					if f.edit {
						ui.Text(c, agentTitle(f.orig.Agent)+" · "+a.declFile(f.orig)).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
					} else {
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormName()).FontSize(12).TextColor(t.TextMuted)
							ui.TextInput(c.Key("mcp-form-name"), &f.name).Placeholder("github").Label(locales.McpFormName()).AutoFocus()
						})
						ui.Row(c).Gap(12).Children(func() {
							ui.Column(c).Grow(1).Basis(0).Gap(4).Children(func() {
								ui.Text(c, locales.McpFormAgent()).FontSize(12).TextColor(t.TextMuted)
								ui.Select(c.Key("mcp-form-agent"), &f.agentLabel, agentOptions).Label(locales.McpFormAgent()).FillWidth().OnChange(func() {
									if i := slices.Index(agentOptions, f.agentLabel); i >= 0 {
										f.agent = i
									}
								})
							})
							ui.Column(c).Grow(1).Basis(0).Gap(4).Children(func() {
								ui.Text(c, locales.McpFormScope()).FontSize(12).TextColor(t.TextMuted)
								ui.Select(c.Key("mcp-form-scope"), &f.scopeLabel, scopeOptions).Label(locales.McpFormScope()).FillWidth().OnChange(func() {
									if i := slices.Index(scopeOptions, f.scopeLabel); i >= 0 {
										f.scope = i
									}
								})
							})
						})
					}
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, locales.McpFormTransport()).FontSize(12).TextColor(t.TextMuted)
						segments(c, "mcp-form-transport", &f.transport, []segment{{label: "stdio"}, {label: "http"}, {label: "sse"}})
					})
					if f.transport == 0 {
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormCommand()).FontSize(12).TextColor(t.TextMuted)
							ui.TextInput(c.Key("mcp-form-command"), &f.command).Placeholder("npx").Label(locales.McpFormCommand())
						})
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormArgs()).FontSize(12).TextColor(t.TextMuted)
							ui.TextInput(c.Key("mcp-form-args"), &f.args).Placeholder("-y @modelcontextprotocol/server-github").Label(locales.McpFormArgs())
						})
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormEnv()).FontSize(12).TextColor(t.TextMuted)
							ui.TextArea(c.Key("mcp-form-env"), &f.env).Height(88).Font(FontMono).Label(locales.McpFormEnv())
							ui.Text(c, locales.McpFormEnvHint()).FontSize(11).TextColor(pal.TextFaint)
						})
					} else {
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormURL()).FontSize(12).TextColor(t.TextMuted)
							ui.TextInput(c.Key("mcp-form-url"), &f.url).Placeholder("https://mcp.example.com/mcp").Label(locales.McpFormURL())
						})
						ui.Column(c).Gap(4).Children(func() {
							ui.Text(c, locales.McpFormHeaders()).FontSize(12).TextColor(t.TextMuted)
							ui.TextArea(c.Key("mcp-form-headers"), &f.headers).Height(88).Font(FontMono).Label(locales.McpFormHeaders())
							ui.Text(c, locales.McpFormHeadersHint()).FontSize(11).TextColor(pal.TextFaint)
						})
					}
					if f.err != "" {
						ui.Text(c, f.err).FontSize(12).TextColor(pal.WarningText)
					}
				})
			})
			ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { f.open = false })
				ui.PrimaryButton(c, locales.GlobalInstructionsPlan()).Height(controlHeight).Disabled(f.loading).OnClick(submit)
			})
		})
	})
}

func splitMCPArgs(raw string) []string {
	var args []string
	var cur strings.Builder
	var quote rune
	inTok := false
	for _, r := range raw {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inTok = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inTok {
				args = append(args, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if inTok {
		args = append(args, cur.String())
	}
	return args
}

func joinMCPArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'") {
			out[i] = strconv.Quote(a)
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}

func formatKVLines(m map[string]string, sep string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + sep + m[k]
	}
	return strings.Join(lines, "\n")
}

func parseKVLines(raw string, allowColon bool) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := map[string]string{}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var k, v string
		var ok bool
		if allowColon {
			k, v, ok = strings.Cut(line, ":")
			if !ok {
				k, v, ok = strings.Cut(line, "=")
			}
		} else {
			k, v, ok = strings.Cut(line, "=")
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok || k == "" {
			return nil, errors.New(locales.McpFormBadKV(line))
		}
		out[k] = v
	}
	return out, nil
}
