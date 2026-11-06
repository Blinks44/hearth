package ui

import (
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// agentInfo is how an agent shows: its name and its logo's slug, "" for
// one without a logo.
type agentInfo struct{ id, name, brand string }

// agentInfos are the agents Hearth knows, in its order.
var agentInfos = []agentInfo{
	{"claude", "Claude Code", "claude"},
	{"codex", "Codex", "openai"},
	{"copilot", "Copilot CLI", "githubcopilot"},
	{"gemini", "Gemini CLI", "googlegemini"},
	{"cursor", "Cursor", "cursor"},
	{"opencode", "OpenCode", ""},
	{"devin", "Devin", ""},
}

// brandColors tint the logos of the agents whose brand has a color.
var brandColors = map[string]ui.Color{"claude": ui.Hex("#D97757"), "gemini": ui.Hex("#4285F4")}

func agentTitle(id string) string {
	for _, ag := range agentInfos {
		if ag.id == id {
			return ag.name
		}
	}
	return id
}

// agentPage is the path of an agent's page, under the agents.
const agentPage = "/ai/agents/agent"

func agentPath(id string) string { return agentPage + "?id=" + url.QueryEscape(id) }

// agentLogo is an agent's logo, size wide, in its brand's color.
func agentLogo(c *ui.Context, id string, size float32) {
	icon := Icon("bot")
	for _, ag := range agentInfos {
		if ag.id == id && ag.brand != "" {
			icon = Brand(ag.brand)
		}
	}
	color, ok := brandColors[id]
	if !ok {
		color = c.Theme().Text
	}
	ui.Icon(c, icon).Size(size, size).TextColor(color)
}

// agentTile is an agent's logo on a tile size wide.
func agentTile(c *ui.Context, id string, size float32) {
	pal := Colors(c.Theme())
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() { agentLogo(c, id, size*0.55) })
}

// agentAvatars are the logos of agents in small circles, as a list row
// ends with.
func agentAvatars(c *ui.Context, ids []string) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(3).Shrink(0).Children(func() {
		for _, id := range ids {
			ui.Box(c).Size(20, 20).Radius(10).Center().BorderWidth(1).BorderColor(pal.Line).
				Background(c.Theme().Background).Tooltip(agentTitle(id)).Children(func() { agentLogo(c, id, 11) })
		}
	})
}

// agents are the agents the last scan looked for, nil before the first.
func (a *App) agents() []inventory.Agent {
	if a.Snapshot == nil {
		return nil
	}
	return a.Snapshot.Agents
}

// foundAgents are the agents on the computer, in Hearth's order.
func (a *App) foundAgents() []inventory.Agent {
	var out []inventory.Agent
	for _, ag := range a.agents() {
		if ag.Found() {
			out = append(out, ag)
		}
	}
	return out
}

// foundIDs are the IDs of the agents on the computer.
func (a *App) foundIDs() []string {
	var out []string
	for _, ag := range a.foundAgents() {
		out = append(out, ag.ID)
	}
	return out
}

// allSkills are the user's skills, then the projects', in their order.
func (a *App) allSkills() []inventory.Skill {
	var out []inventory.Skill
	if a.Snapshot != nil {
		out = append(out, a.Snapshot.Skills...)
	}
	for _, p := range a.Projects {
		out = append(out, p.Skills...)
	}
	return out
}

// allServers are the MCP servers of the user's files, then the projects'.
func (a *App) allServers() []inventory.MCPServer {
	var out []inventory.MCPServer
	if a.Snapshot != nil {
		out = append(out, a.Snapshot.MCP...)
	}
	for _, p := range a.Projects {
		out = append(out, p.MCP...)
	}
	return out
}

// skillAgents are the agents finding sk, in the order of its paths.
func skillAgents(sk inventory.Skill) []string {
	var out []string
	for _, p := range sk.Paths {
		for _, id := range p.Agents {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

// agentCounts are the skills agent id can use and the MCP servers
// declared for it.
func (a *App) agentCounts(id string) (skills, servers int) {
	for _, sk := range a.allSkills() {
		if !sk.Broken && slices.Contains(skillAgents(sk), id) {
			skills++
		}
	}
	for _, s := range a.allServers() {
		if s.Agent == id {
			servers++
		}
	}
	return skills, servers
}

// agentCopy is the copy of an agent's command the terminal runs, else the
// first found; nil for none.
func agentCopy(ag inventory.Agent) *inventory.Installation {
	if len(ag.Installations) == 0 {
		return nil
	}
	if i := inventory.TerminalCopy(ag.Installations); i >= 0 {
		return &ag.Installations[i]
	}
	return &ag.Installations[0]
}

// agentVersion is the version of the copy the terminal runs, else of the
// app; "" when unknown.
func agentVersion(ag inventory.Agent) string {
	if in := agentCopy(ag); in != nil {
		return in.Version
	}
	return ag.AppVersion
}

// scopeName names where a skill or server counts: the user's, or a
// project's by its folder.
func scopeName(project string) string {
	if project == "" {
		return locales.ScopeUser()
	}
	return locales.ScopeProjectOf(filepath.Base(project))
}

// aiStat is a count in an AI page's header.
type aiStat struct{ value, label string }

// aiIntro is the top of an AI page: an icon on a tile, the title and what
// the page is about, and the counts on the right.
func aiIntro(c *ui.Context, icon, title, text string, stats []aiStat) {
	t := c.Theme()
	pal := Colors(t)
	headRow(c, func() {
		ui.Box(c).Size(48, 48).Shrink(0).Radius(12).Center().Background(pal.AccentSoft).
			BorderWidth(1).BorderColor(Accent.Alpha(0.15)).Children(func() {
			ui.Icon(c, Icon(icon)).Size(26, 26).TextColor(Accent)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(22).FontWeight(600).SingleLine()
			ui.Text(c, text).FontSize(12.5).TextColor(t.TextMuted).MaxWidth(420)
		})
	}, func() {
		for _, s := range stats {
			e := ui.Column(c).Width(88).Gap(1).Padding(8, 14)
			if narrow(c) {
				e.Grow(1).Basis(0)
			}
			e.Radius(10).Background(pal.Subtle).
				BorderWidth(1).BorderColor(pal.Line).Children(func() {
				ui.Text(c, s.value).FontSize(16).SingleLine()
				ui.Text(c, s.label).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
			})
		}
	})
}

// agentFilter is a chip per agent on the computer, and one for all,
// choosing *chosen; count counts what an agent has, "" counting all.
func (a *App) agentFilter(c *ui.Context, key string, chosen *string, count func(id string) int) {
	t := c.Theme()
	pal := Colors(t)
	chipFor := func(id, label string) {
		on := *chosen == id
		b := ui.ButtonBase(c.Key(key+":"+id)).Gap(6).Padding(5, 10, 5, 9).Radius(13).AlignItems(ui.Center).
			Label(label).BorderWidth(1)
		text, faint := t.Text, pal.TextFaint
		switch {
		case on:
			b.Background(t.Text).BorderColor(t.Text)
			text, faint = t.Background, t.Background.Alpha(0.6)
		case b.Hovered():
			b.Background(pal.Subtle).BorderColor(pal.ControlBorder)
		default:
			b.Background(t.Background).BorderColor(pal.ControlBorder)
		}
		b.Children(func() {
			if id != "" {
				agentLogo(c, id, 13)
			}
			ui.Text(c, label).FontSize(12.5).TextColor(text).SingleLine()
			ui.Text(c, strconv.Itoa(count(id))).FontSize(11).TextColor(faint).SingleLine()
		})
		if b.Clicked() {
			*chosen = id
		}
	}
	ui.Row(c).Gap(6).Wrap().Grow(1).MinWidth(0).Children(func() {
		chipFor("", locales.AllAgents())
		for _, id := range a.foundIDs() {
			chipFor(id, agentTitle(id))
		}
	})
}

// scopeSegments switch *scope between all, the user's and the projects'.
func scopeSegments(c *ui.Context, key string, scope *int) {
	segments(c, key, scope, []segment{{locales.TabAll(), ""}, {locales.ScopeUser(), ""}, {locales.ScopeProject(), ""}})
}

// inScope reports whether something of project shows under scope: 0 all,
// 1 the user's, 2 the projects'.
func inScope(scope int, project string) bool {
	return scope == 0 || (scope == 1) == (project == "")
}

// aiTag is a short label on a soft background.
func aiTag(c *ui.Context, label string, bg, fg ui.Color) {
	ui.Text(c, label).FontSize(10.5).FontWeight(500).TextColor(fg).Padding(1, 6).Radius(5).Background(bg).SingleLine()
}

// detailSection is the title of a section of an AI page's details.
func detailSection(c *ui.Context, title string) {
	pal := Colors(c.Theme())
	ui.Text(c, strings.ToUpper(title)).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).
		Padding(14, 16, 6, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).SingleLine()
}

// groupTitle is a list's heading of a scope: its name and where it comes
// from.
func groupTitle(c *ui.Context, name, from string) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).Padding(8, 14, 6, 14).AlignItems(ui.Center).Background(pal.Subtle).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(name)).FontSize(11).FontWeight(500).TextColor(c.Theme().TextMuted).Shrink(0).SingleLine()
		ui.Text(c, from).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0).SingleLine()
	})
}

// listFrame is the bordered list of an AI page.
func listFrame(c *ui.Context, rows func()) {
	t := c.Theme()
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(Colors(t).Divider).Children(rows)
}

// aiEmpty is an AI page's body before the first scan, or with nothing in
// it.
func (a *App) aiEmpty(c *ui.Context, icon, text string) {
	if a.Snapshot == nil {
		text = locales.NotChecked()
		if a.Scanning {
			text = locales.Checking()
		}
	}
	ui.Column(c).Grow(1).Center().Gap(8).Padding(0, 28).Children(func() {
		ui.Icon(c, Icon(icon)).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
		ui.Text(c, text).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
	})
}

// agentsPage lists the agents found, with what they have, and the ones not
// found.
func (a *App) agentsPage(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		found := a.foundAgents()
		if len(found) == 0 && len(a.agents()) == 0 {
			a.aiEmpty(c, "bot", locales.NoAgents())
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				servers := map[string]bool{}
				for _, s := range a.allServers() {
					servers[s.Project+"\x00"+s.Name] = true
				}
				aiIntro(c, "bot", locales.AiAgentsTitle(), locales.AiAgentsText(), []aiStat{
					{strconv.Itoa(len(found)), locales.StatFound()},
					{strconv.Itoa(len(a.allSkills())), locales.StatSkills()},
					{strconv.Itoa(len(servers)), "MCP"},
				})
				if len(found) > 0 {
					a.agentTable(c, found)
				} else {
					listFrame(c, func() { cardNote(c, locales.NoAgents()) })
				}
				a.agentsNotFound(c)
			})
		})
	})
}

// agentTable is a row per agent found: where it is, its version, what
// installed it, its skills, MCP servers and instructions.
func (a *App) agentTable(c *ui.Context, found []inventory.Agent) {
	t := c.Theme()
	pal := Colors(t)
	// Instructions wrap in a narrow window, which leaves the agent's name
	// and path room.
	instructions := float32(220)
	if narrow(c) {
		instructions = 130
	}
	listFrame(c, func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			head := func(title string) ui.Element {
				return ui.Text(c, title).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
			head(locales.ColumnAgent()).Grow(1).MinWidth(0)
			head(locales.ColumnVersion()).Width(110).Shrink(0)
			head(locales.ColumnInstall()).Width(90).Shrink(0)
			head("Skills").Width(56).Shrink(0)
			head("MCP").Width(46).Shrink(0)
			head(locales.ColumnInstructions()).Width(instructions).Shrink(0)
			ui.Box(c).Width(14).Shrink(0)
		})
		for _, ag := range found {
			skills, servers := a.agentCounts(ag.ID)
			in := agentCopy(ag)
			where := ag.App
			if in != nil {
				where = in.Path
			}
			e := ui.ButtonBase(c.Key("agent:"+ag.ID)).Gap(12).Padding(10, 12, 10, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(agentTitle(ag.ID))
			if e.Hovered() {
				e.Background(pal.Subtle)
			}
			e.Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
					agentTile(c, ag.ID, 28)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, agentTitle(ag.ID)).FontSize(13.5).SingleLine()
						ui.Text(c, tilde(where, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					})
				})
				version := agentVersion(ag)
				if version == "" {
					version = "—"
				}
				ui.Text(c, version).Font(FontMono).FontSize(13).Width(110).Shrink(0).SingleLine()
				ui.Row(c).Width(90).Shrink(0).Children(func() {
					if in != nil {
						sourceBadge(c, in.Source)
					} else {
						ui.Text(c, locales.AppSource()).Font(FontMono).FontSize(11).TextColor(pal.MonoText).
							Padding(2, 7).Radius(5).BorderWidth(1).BorderColor(pal.ControlBorder).SingleLine()
					}
				})
				count := func(n int) {
					color := t.Text
					if n == 0 {
						color = pal.TextFaint
					}
					ui.Text(c, strconv.Itoa(n)).FontSize(12.5).TextColor(color).Width(56).Shrink(0).SingleLine()
				}
				count(skills)
				count(servers)
				ui.Row(c).Width(instructions).Shrink(0).Gap(6).Wrap().Children(func() {
					shown := 0
					for _, f := range ag.Files {
						if f.Kind == inventory.FileInstructions {
							ui.Text(c, filepath.Base(f.Path)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).
								Padding(2, 6).Radius(5).Background(pal.Muted).SingleLine()
							shown++
						}
					}
					if shown == 0 {
						ui.Text(c, locales.NoInstructions()).FontSize(12).TextColor(pal.TextFaint).SingleLine()
					}
				})
				ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(pal.TextFaint)
			})
			if e.Clicked() {
				a.router.Push(agentPath(ag.ID))
			}
		}
	})
}

// agentsNotFound lists the agents Hearth knows that the scan did not
// find, telling those whose settings are left.
func (a *App) agentsNotFound(c *ui.Context) {
	var missing []inventory.Agent
	for _, ag := range a.agents() {
		if !ag.Found() {
			missing = append(missing, ag)
		}
	}
	if len(missing) == 0 {
		return
	}
	listCard(c, locales.NotFoundHere(), locales.NotFoundWhere(), func() {
		for _, ag := range missing {
			caption := ""
			// Skills directories other agents share say nothing of this one.
			if slices.ContainsFunc(ag.Files, func(f inventory.AgentFile) bool {
				return strings.HasPrefix(filepath.ToSlash(f.Path), filepath.ToSlash(ag.Dir)+"/")
			}) {
				caption = locales.SettingsLeft(tilde(ag.Dir, a.Home))
			}
			listRow(c, agentTitle(ag.ID), false, caption, func() {})
		}
	})
}

// agentDetail is an agent's page: where it stands, how it is installed,
// its files, the skills it can use and its MCP servers.
func (a *App) agentDetail(c *ui.Context, id string) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, agentTitle(id), locales.SectionAI()+" · "+locales.AgentsTitle(), headerExtras{
			back: func() { a.router.Push("/ai/agents") },
		})
		i := slices.IndexFunc(a.agents(), func(ag inventory.Agent) bool { return ag.ID == id })
		if i < 0 {
			text := locales.AgentNotFound()
			if a.Snapshot == nil {
				text = ""
			}
			a.aiEmpty(c, "bot", text)
			return
		}
		ag := a.agents()[i]
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.agentHero(c, ag)
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.installCard(c, ag)
						a.agentFilesCard(c, ag)
					})
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.agentSkillsCard(c, ag.ID)
						a.agentServersCard(c, ag.ID)
					})
				})
			})
		})
	})
}

// agentHero is the agent's logo and name, how it was installed, and the
// cards of its version, the terminal's copy and its settings.
func (a *App) agentHero(c *ui.Context, ag inventory.Agent) {
	t := c.Theme()
	pal := Colors(t)
	in := agentCopy(ag)
	caption := locales.ToolNotInstalled(agentTitle(ag.ID))
	switch {
	case in != nil:
		caption = locales.AgentInstalledVia(sourceName(in.Source))
	case ag.App != "":
		caption = locales.AppSource() + " · " + tilde(ag.App, a.Home)
	}
	headRow(c, func() {
		agentTile(c, ag.ID, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, agentTitle(ag.ID)).FontSize(22).FontWeight(600).SingleLine()
			ui.Text(c, caption).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
	}, func() {
		version := agentVersion(ag)
		if version == "" {
			version = "—"
		}
		stateCard(c, pal.Info, locales.StateVersion(), version, locales.VersionsCount(len(ag.Installations)))
		term, termCaption, dot := "—", locales.NotInPath(), pal.TextFaint
		if i := inventory.TerminalCopy(ag.Installations); i >= 0 {
			term, termCaption, dot = filepath.Base(ag.Installations[i].Path), sourceName(ag.Installations[i].Source), t.Success
		}
		stateCard(c, dot, locales.StateInTerminal(), term, termCaption)
		stateCard(c, pal.TextFaint, locales.StateSettings(), locales.FilesCount(len(ag.Files)), tilde(ag.Dir, a.Home))
	})
}

// installCard lists the copies of the agent's command, by where PATH has
// them, and its app.
func (a *App) installCard(c *ui.Context, ag inventory.Agent) {
	listCard(c, locales.InstallationCard(), "", func() {
		if len(ag.Installations) == 0 && ag.App == "" {
			cardNote(c, locales.ToolNotInstalled(agentTitle(ag.ID)))
		}
		for _, in := range ag.Installations {
			where := locales.CopyGuiOnly()
			switch {
			case in.OnPath == 1:
				where = locales.CopyFirst()
			case in.OnPath > 1:
				where = locales.CopyOnPath(int(in.OnPath))
			}
			listRow(c, tilde(in.Path, a.Home), true, where+" · "+sourceName(in.Source), func() {
				ui.Text(c, in.Version).Font(FontMono).FontSize(11.5).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			})
		}
		if ag.App != "" {
			listRow(c, tilde(ag.App, a.Home), true, locales.AppSource(), func() {
				ui.Text(c, ag.AppVersion).Font(FontMono).FontSize(11.5).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			})
		}
	})
}

// agentFilesCard lists the agent's files in the home folder, each to open.
func (a *App) agentFilesCard(c *ui.Context, ag inventory.Agent) {
	listCard(c, locales.SettingsCard(), tilde(ag.Dir, a.Home), func() {
		if len(ag.Files) == 0 {
			cardNote(c, locales.NoAgentFiles())
		}
		for _, f := range ag.Files {
			listRow(c, tilde(f.Path, a.Home), false, fileKind(f.Kind), func() {
				if a.Shell == nil {
					return
				}
				path := f.Path
				ui.Row(c.Key("file:" + path)).Shrink(0).Children(func() {
					if f.Kind == inventory.FileSkills {
						iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(path) })
					} else {
						iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, path) })
					}
				})
			})
		}
	})
}

// openFile opens path in its app, telling in a toast when it cannot.
func (a *App) openFile(c *ui.Context, path string) {
	if err := a.Shell.OpenPath(path); err != nil {
		c.Toast(locales.OpenFailed(tilde(path, a.Home), err.Error()))
	}
}

// fileKind says what an agent's file holds.
func fileKind(k inventory.FileKind) string {
	switch k {
	case inventory.FileInstructions:
		return locales.FileInstructions()
	case inventory.FileMCP:
		return locales.FileMcp()
	case inventory.FileSkills:
		return locales.FileSkills()
	}
	return locales.FileSettings()
}

// shownSkills is how many skills an agent's page lists; the rest are on
// the skills page.
const shownSkills = 8

// agentSkillsCard lists the skills the agent can use, with a way to all
// of them on the skills page.
func (a *App) agentSkillsCard(c *ui.Context, id string) {
	var skills []inventory.Skill
	for _, sk := range a.allSkills() {
		if !sk.Broken && slices.Contains(skillAgents(sk), id) {
			skills = append(skills, sk)
		}
	}
	listCard(c, "Skills", strconv.Itoa(len(skills)), func() {
		if len(skills) == 0 {
			cardNote(c, locales.NoSkillsFor())
		}
		for _, sk := range skills[:min(len(skills), shownSkills)] {
			listRow(c, sk.Name, false, tilde(sk.Dir, a.Home), func() {
				ui.Text(c, strings.ToLower(scopeName(sk.Project))).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			})
		}
		if len(skills) > 0 {
			moreLink(c, "agent-skills", locales.AllInSkills(), func() {
				a.skillAgent = id
				a.router.Push(skillsPage)
			})
		}
	})
}

// agentServersCard lists the MCP servers declared for the agent.
func (a *App) agentServersCard(c *ui.Context, id string) {
	var servers []inventory.MCPServer
	for _, s := range a.allServers() {
		if s.Agent == id {
			servers = append(servers, s)
		}
	}
	listCard(c, "MCP", locales.ServersCount(len(servers)), func() {
		if len(servers) == 0 {
			cardNote(c, locales.NoServersFor())
		}
		for _, s := range servers {
			listRow(c, s.Name, false, strings.ToLower(scopeName(s.Project))+" · "+tilde(s.File, a.Home), func() {
				ui.Text(c, s.Transport).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			})
		}
	})
}

// moreLink is a list card's last row going elsewhere.
func moreLink(c *ui.Context, key, label string, onClick func()) {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ButtonBase(c.Key(key)).Gap(4).Padding(9, 14).AlignItems(ui.Center).Label(label).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).OnClick(onClick)
	color := t.TextMuted
	if b.Hovered() {
		color = t.Text
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).TextColor(color).SingleLine()
		ui.Icon(c, Icon("chevron-right")).Size(13, 13).TextColor(color)
	})
}

// overviewAgents is the overview's card of the agents found, each with
// its version and how many skills and MCP servers it has.
func (a *App) overviewAgents(c *ui.Context) {
	if a.firstScan() {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	found := a.foundAgents()
	count := ""
	if len(found) > 0 {
		count = strconv.Itoa(len(found))
	}
	card(c, locales.AiAgentsTitle(), count, func() {
		b := ui.ButtonBase(c.Key("overview-ai")).Gap(4).AlignItems(ui.Center).Label(locales.SkillsAndMcp()).
			OnClick(func() { a.router.Push(skillsPage) })
		color := t.TextMuted
		if b.Hovered() {
			color = t.Text
		}
		b.Children(func() {
			ui.Text(c, locales.SkillsAndMcp()).FontSize(12.5).TextColor(color).SingleLine()
			ui.Icon(c, Icon("chevron-right")).Size(13, 13).TextColor(color)
		})
	}, func() {
		if len(found) == 0 {
			cardNote(c, locales.NoAgents())
		}
		for _, ag := range found {
			skills, servers := a.agentCounts(ag.ID)
			e := ui.ButtonBase(c.Key("overview-agent:"+ag.ID)).Gap(10).Padding(10, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(agentTitle(ag.ID))
			if e.Hovered() {
				e.Background(pal.Subtle)
			}
			e.Children(func() {
				agentTile(c, ag.ID, 26)
				ui.Text(c, agentTitle(ag.ID)).FontSize(13).Shrink(0).SingleLine()
				ui.Text(c, agentVersion(ag)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0).SingleLine()
				counts := "—"
				if skills+servers > 0 {
					counts = locales.AgentCounts(skills, servers)
				}
				ui.Text(c, counts).FontSize(11.5).TextColor(t.TextMuted).Shrink(0).SingleLine()
			})
			if e.Clicked() {
				a.router.Push(agentPath(ag.ID))
			}
		}
	})
}
