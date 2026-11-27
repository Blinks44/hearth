package ui

import (
	"context"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
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
	{"antigravity", "Antigravity CLI", "antigravity"},
	{"gemini", "Gemini CLI", "googlegemini"},
	{"cursor", "Cursor", "cursor"},
	{"opencode", "OpenCode", "opencode"},
	{"devin", "Devin", ""},
	{"grok", "Grok Build", "grok"},
	{"kimi", "Kimi Code", "kimi"},
	{"aider", "Aider", ""},
}

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

// agentIcon returns the logo or fallback icon key for the agent id.
func agentIcon(id string) string {
	for _, ag := range agentInfos {
		if ag.id == id && ag.brand != "" {
			return "brand/" + ag.brand
		}
	}
	return "lucide/bot"
}

// agentLogo is an agent's logo, size wide, in its colors.
func agentLogo(c *ui.Context, id string, size float32) {
	iconOrLogo(c, agentIcon(id), size, c.Theme().Text)
}

func agentLogoOn(c *ui.Context, id string, size float32, color ui.Color, darkBg bool) {
	iconOrLogoOn(c, agentIcon(id), size, color, darkBg)
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
			e := ui.Column(c).MinWidth(88).Gap(1).Padding(8, 14)
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
				agentLogoOn(c, id, 13, text, t.Dark != on)
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
func aiTag(c *ui.Context, label string, bg, fg ui.Color) ui.Element {
	return ui.Text(c, label).FontSize(10.5).FontWeight(500).TextColor(fg).Padding(1, 6).Radius(5).Background(bg).SingleLine()
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
		clipped(c, from).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
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
					{strconv.Itoa(len(a.allSkills())), locales.StatSkills(len(a.allSkills()))},
					{strconv.Itoa(len(servers)), "MCP"},
				})
				if len(found) > 0 {
					a.agentRadarCard(c)
					a.sharedInstructionsBanner(c)
					a.agentTable(c, found)
				} else {
					listFrame(c, func() {
						cardNote(c, locales.NoAgents())
						ui.Row(c).Padding(0, 14, 12, 14).Children(func() { a.installPrimary(c, "claude") })
					})
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
			head(locales.SkillsTitle()).Width(56).Shrink(0)
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
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(agentTitle(ag.ID)).Transition(colorFade)
			if e.Hovered() {
				e.Background(pal.Subtle)
			}
			e.Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
					agentTile(c, ag.ID, 28)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, agentTitle(ag.ID)).FontSize(13.5).SingleLine()
						clipped(c, tilde(where, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
					})
				})
				version := agentVersion(ag)
				if version == "" {
					version = "—"
				}
				ui.Column(c).Width(110).Shrink(0).Gap(1).Children(func() {
					ui.Text(c, version).Font(FontMono).FontSize(13).SingleLine()
					if in == nil {
						return
					}
					if _, pkg, ok := a.copyPackage(*in); ok && pkg.Available != "" {
						ui.Text(c, locales.AgentUpdateAvailable(pkg.Available)).FontSize(11).TextColor(pal.Info).SingleLine()
					}
				})
				ui.Row(c).Width(90).Shrink(0).Children(func() {
					if in != nil {
						sourceBadge(c, in.Source)
					} else {
						ui.Text(c, locales.AppSource()).Font(FontMono).FontSize(11).TextColor(pal.MonoText).
							Padding(2, 7).Radius(5).BorderWidth(1).BorderColor(pal.ControlBorder).SingleLine()
					}
				})
				count := func(n int, width float32) {
					color, text := t.Text, strconv.Itoa(n)
					if n == 0 {
						color = pal.TextFaint
					}
					if ag.ID == "aider" { // it has neither skills nor MCP
						text = "—"
					}
					ui.Text(c, text).FontSize(12.5).TextColor(color).Width(width).Shrink(0).SingleLine()
				}
				count(skills, 56)
				count(servers, 46)
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
						if ag.Instructions != nil && ag.Instructions.State == "shared" {
							chip(c, Accent, pal.AccentSoft, pal.AccentInk, locales.GlobalInstructionsChipShared())
						} else {
							ui.Text(c, locales.NoInstructions()).FontSize(12).TextColor(pal.TextFaint).SingleLine()
						}
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
			listRow(c, agentTitle(ag.ID), false, caption, func() {
				a.installToolButton(c, ag.ID)
				if ag.ID == "antigravity" && !a.canInstallTool(ag.ID) {
					ui.Button(c.Key("antigravity-download"), locales.AntigravityDownload()).OnClick(func() {
						c.OpenURL("https://antigravity.google/download")
					})
				}
			})
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
				if ag.ID == "antigravity" {
					ui.Row(c).Children(func() {
						ui.Button(c.Key("antigravity-migration"), locales.AntigravityMigration()).OnClick(func() {
							c.OpenURL("https://antigravity.google/docs/cli/gcli-migration")
						})
					})
				}
				if ag.Instructions != nil {
					a.globalInstructionsCard(c, ag.ID, ag.Instructions)
				}
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.installCard(c, ag)
						a.agentFilesCard(c, ag)
						if ai.AccessKnown(ag.ID) {
							a.agentRulesCard(c, ag)
						}
					})
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.agentSkillsCard(c, ag.ID)
						a.agentServersCard(c, ag.ID)
						if ai.AccessKnown(ag.ID) {
							a.agentHooksCard(c, ag)
						}
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
		settingsCaption := tilde(ag.Dir, a.Home)
		if u, ok := a.agentDirSize(ag.Dir); ok {
			settingsCaption = tilde(ag.Dir, a.Home) + " · " + formatUsage(u)
		}
		stateCard(c, pal.TextFaint, locales.StateSettings(), locales.FilesCount(len(ag.Files)), settingsCaption)
	})
}

func (a *App) agentDirSize(dir string) (inventory.Usage, bool) {
	if a.Sizes == nil || dir == "" {
		return inventory.Usage{}, false
	}
	u, ok := a.Sizes.Paths[dir]
	return u, ok
}

// installCard lists the copies of the agent's command, by where PATH has
// them, and its app.
func (a *App) installCard(c *ui.Context, ag inventory.Agent) {
	listCard(c, locales.InstallationCard(), "", func() {
		if len(ag.Installations) == 0 && ag.App == "" {
			listRow(c, locales.ToolNotInstalled(agentTitle(ag.ID)), false, "", func() {
				a.installToolButton(c, ag.ID)
			})
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
				a.agentCopyMenu(c, ag.ID, in)
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
	aside := tilde(ag.Dir, a.Home)
	if u, ok := a.agentDirSize(ag.Dir); ok {
		aside += " · " + formatUsage(u)
	}
	listCard(c, locales.SettingsCard(), aside, func() {
		if len(ag.Files) == 0 {
			cardNote(c, locales.NoAgentFiles())
		}
		for _, f := range ag.Files {
			listRow(c, tilde(f.Path, a.Home), false, fileKind(f.Kind), func() {
				path := f.Path
				if f.Kind == inventory.FileSkills {
					if a.Shell == nil {
						return
					}
					ui.Row(c.Key("file:" + path)).Shrink(0).Children(func() {
						iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(path) })
					})
					return
				}
				if !a.canOpenFile() {
					return
				}
				ui.Row(c.Key("file:" + path)).Shrink(0).Children(func() {
					iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, path) })
				})
			})
		}
	})
}

// canOpenFile reports whether Hearth can open a file in an editor or the
// system app.
func (a *App) canOpenFile() bool {
	if a.Shell != nil {
		return true
	}
	_, ok := a.chosenEditor()
	return a.OpenInEditor != nil && ok
}

// openFile opens path in the editor chosen in Settings, or with the
// system's app when none is chosen or it is gone.
func (a *App) openFile(c *ui.Context, path string) {
	if e, ok := a.chosenEditor(); ok && a.OpenInEditor != nil {
		a.OpenInEditor(e.Path, e.Path, path, func(err error) {
			if err != nil {
				a.planErr = locales.OpenFailed(tilde(path, a.Home), err.Error())
			}
		})
		return
	}
	if a.Shell == nil {
		return
	}
	if err := a.Shell.OpenPath(path); err != nil {
		c.Toast(locales.OpenFailed(tilde(path, a.Home), err.Error()))
	}
}

// editorChoices are the editors files can open in: the common ones
// installed, then the IDEs the Editors page found with an app or a
// command, by name.
func (a *App) editorChoices() []run.App {
	choices := slices.Clone(a.EditorApps)
	if a.Snapshot == nil {
		return choices
	}
	for _, e := range a.editorsState().Editors {
		// An app bundle opens files on macOS; elsewhere the command does.
		path := e.App
		if !strings.HasSuffix(path, ".app") {
			path = e.CommandPath
		}
		if path != "" && !slices.ContainsFunc(choices, func(c run.App) bool { return c.Name == e.Name }) {
			choices = append(choices, run.App{Name: e.Name, Path: path})
		}
	}
	return choices
}

// chosenEditor is the editor chosen in Settings, while it is installed or a custom path/command.
func (a *App) chosenEditor() (run.App, bool) {
	if a.Editor == "" {
		return run.App{}, false
	}
	for _, e := range a.editorChoices() {
		if e.Name == a.Editor {
			return e, true
		}
	}
	if validCustomApp(a.Editor) {
		return run.App{Name: a.Editor, Path: a.Editor}, true
	}
	return run.App{}, false
}

func isCustomAppPath(p string) bool {
	trimmed := strings.TrimSpace(p)
	return filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "~") || strings.ContainsAny(trimmed, `/\`)
}

// isPathCommand reports whether s is a single executable name (without path
// separators, whitespace, or shell metacharacters) suitable for PATH lookup.
func isPathCommand(s string) bool {
	if s == "" || s == "." || s == ".." || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == '+' || r == '@':
		default:
			return false
		}
	}
	return true
}

// validCustomApp reports whether s is an absolute path or a bare PATH command name.
func validCustomApp(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || strings.ContainsAny(trimmed, "\"'\r\n\x00") {
		return false
	}
	return filepath.IsAbs(trimmed) || isPathCommand(trimmed)
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
	listCard(c, locales.SkillsTitle(), strconv.Itoa(len(skills)), func() {
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

// agentRulesCard lists what the agent may do without asking, must ask
// for or may not do: the user's settings, then the projects'.
func (a *App) agentRulesCard(c *ui.Context, ag inventory.Agent) {
	rules := slices.Clone(ag.Rules)
	files := map[string]bool{}
	for _, p := range a.Projects {
		for _, r := range p.Rules {
			if r.Agent == ag.ID {
				rules = append(rules, r)
			}
		}
	}
	for _, r := range rules {
		files[r.File] = true
	}
	aside := ""
	if len(rules) > 0 {
		aside = locales.RulesCount(len(rules)) + " · " + locales.FilesCount(len(files))
	}
	listCard(c, locales.RulesCard(), aside, func() {
		if len(rules) == 0 {
			cardNote(c, locales.NoRulesFor())
		}
		for _, r := range rules {
			listRow(c, r.Rule, true, strings.ToLower(scopeName(r.Project))+" · "+tilde(r.File, a.Home), func() {
				t := c.Theme()
				pal := Colors(t)
				label, color := locales.RuleAllowed(), pal.SuccessText
				switch r.Kind {
				case inventory.RuleMode:
					label, color = locales.RuleMode(), pal.TextFaint
				case inventory.RuleAsk:
					label, color = locales.RuleAsks(), pal.TextFaint
				case inventory.RuleDeny:
					label, color = locales.RuleDenied(), pal.DangerText
				}
				if r.Risky {
					aiTag(c, locales.RuleUnasked(), pal.WarningSoft, pal.WarningText)
					return
				}
				ui.Text(c, label).Font(FontMono).FontSize(11).TextColor(color).SingleLine()
			})
		}
	})
}

// agentHooksCard lists the commands the agent's settings run on its
// events: the user's, then the projects'.
func (a *App) agentHooksCard(c *ui.Context, ag inventory.Agent) {
	hooks := slices.Clone(ag.Hooks)
	for _, p := range a.Projects {
		for _, h := range p.Hooks {
			if h.Agent == ag.ID {
				hooks = append(hooks, h)
			}
		}
	}
	listCard(c, locales.HooksCard(), locales.HooksCount(len(hooks)), func() {
		if len(hooks) == 0 {
			cardNote(c, locales.NoHooksFor())
		}
		for _, h := range hooks {
			caption := strings.ToLower(scopeName(h.Project)) + " · " + tilde(h.File, a.Home)
			if h.Matcher != "" {
				caption = h.Matcher + " · " + caption
			}
			listRow(c, h.Command, true, caption, func() {
				ui.Text(c, h.Event).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			})
		}
		if slices.Contains(ops.SupportedHookAgents, ag.ID) && a.BuildPlan != nil {
			agentID := ag.ID
			installed := a.hasHearthHooks(agentID)
			listRow(c, locales.RadarHooksBannerTitle(), false, locales.RadarHooksBannerBody(), func() {
				if installed {
					pal := Colors(c.Theme())
					aiTag(c, locales.RadarHooksInstalled(), pal.SuccessSoft, pal.SuccessText)
					ui.Button(c.Key("agent-hook-rm:"+agentID), locales.RadarHooksRemove()).Height(controlHeight).OnClick(func() {
						a.planAgentHooks(agentID, false)
					})
				} else {
					ui.Button(c.Key("agent-hook-add:"+agentID), locales.RadarHooksInstall()).Height(controlHeight).OnClick(func() {
						a.planAgentHooks(agentID, true)
					})
				}
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
		a.pageLink(c, "overview-ai", locales.SkillsAndMcp(), skillsPage)
	}, func() {
		if len(found) == 0 {
			cardNote(c, locales.NoAgents())
		}
		for _, ag := range found {
			skills, servers := a.agentCounts(ag.ID)
			e := ui.ButtonBase(c.Key("overview-agent:"+ag.ID)).Gap(10).Padding(10, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(agentTitle(ag.ID)).Transition(colorFade)
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

// findAgent finds the agent id again for an operation's check; nil when
// the app cannot.
func (a *App) findAgent(id string) ops.FindAgent {
	d := a.DetectAgent
	if d == nil {
		return nil
	}
	return func(ctx context.Context) (inventory.Agent, error) { return d(ctx, id) }
}

// agentCopyMenu is the menu of in, a copy of the agent id: update it,
// remove it, through the package manager that installed it, or update it
// through npm. None when there is nothing to do.
func (a *App) agentCopyMenu(c *ui.Context, id string, in inventory.Installation) {
	pm, pkg, managed := a.copyPackage(in)
	managed = managed && a.packageCan(pm, pkg)
	npm, viaNpm := ops.AgentNpmUpdatePlan(id, agentTitle(id), in, a.findAgent(id))
	if !managed && !viaNpm {
		return
	}
	ui.Row(c.Key("menu:" + in.RealPath)).Shrink(0).Children(func() {
		iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(m *ui.Menu) {
			if managed {
				a.packageItems(m, pm, pkg)
			}
			if viaNpm && m.Item(locales.AgentUpdateNpm()).Chosen() {
				a.Propose(npm)
			}
		})
	})
}
