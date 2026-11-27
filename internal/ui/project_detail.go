package ui

import (
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// projectHeaderActions renders quick actions (Editor, Terminal, README) before
// the overflow menu in the project page header.
func (a *App) projectHeaderActions(c *ui.Context, p projects.Project) {
	t := c.Theme()
	compact := narrow(c)
	quickBtn := func(key, icon, shortText, fullLabel string, onClick func()) {
		if compact {
			ui.Button(c.Key(key), "").Size(controlHeight, controlHeight).Padding(0).
				Label(fullLabel).Tooltip(fullLabel).OnClick(onClick).Children(func() {
				ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(t.TextMuted)
			})
			return
		}
		ui.Button(c.Key(key), "").Height(controlHeight).Label(fullLabel).Tooltip(fullLabel).
			OnClick(onClick).Children(func() {
			ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, shortText).FontSize(12.5).SingleLine()
		})
	}
	if a.canOpenFile() {
		quickBtn("project-open-editor", "external-link", locales.ProjectActionEditor(), locales.OpenInEditor(), func() {
			a.openFile(c, p.Dir)
		})
	}
	if a.OpenTerminalInDir != nil {
		dir := p.Dir
		quickBtn("project-open-terminal", "terminal", locales.ProjectActionTerminal(), locales.ProjectOpenTerminal(), func() {
			a.OpenTerminalInDir(dir, func(err error) {
				if err != nil {
					a.planErr = err.Error()
				}
			})
		})
	}
	if p.Readme != "" && a.canOpenFile() {
		readmePath := filepath.Join(p.Dir, p.Readme)
		quickBtn("project-open-readme", "book-open", "README", locales.ProjectOpenReadme(p.Readme), func() {
			a.openFile(c, readmePath)
		})
	}
}

func (a *App) projectServicesCount(p projects.Project) int {
	n := len(p.Databases) + a.projectRunningCounts(p) + len(a.projectComposeContainers(p)) + len(composeProjects(p))
	if _, linked := a.KubeLinks[p.Dir]; linked || len(p.Kube) > 0 {
		n++
	}
	return n
}

func (a *App) projectAICount(p projects.Project) int {
	return len(p.Skills) + len(p.MCP) + len(p.Rules) + len(p.Hooks)
}

func (a *App) projectTabs(c *ui.Context, p projects.Project) {
	servicesN := a.projectServicesCount(p)
	aiN := a.projectAICount(p)
	if aiN == 0 {
		aiN = len(p.AI)
	}
	countStr := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	pal := Colors(c.Theme())
	row := ui.Row(c).Gap(12).AlignItems(ui.Center)
	if narrow(c) {
		row.Wrap()
	}
	row.Children(func() {
		segments(c, "project-detail-tabs", &a.projectTab, []segment{
			{locales.ProjectTabOverview(), ""},
			{locales.ProjectTabRunServices(), countStr(servicesN)},
			{"AI", countStr(aiN)},
			{locales.ProjectTabFiles(), countStr(len(p.Files))},
		})
		if g := p.Git; g != nil && g.LastCommit != "" {
			ui.Box(c).Grow(1).MinWidth(8).Height(1)
			commit := g.LastCommit
			if !g.LastCommitAt.IsZero() {
				commit = locales.ProjectCommitAgo(ago(c, g.LastCommitAt), g.LastCommit)
			}
			clipped(c, commit).FontSize(11.5).TextColor(pal.TextFaint).Shrink(1)
		}
	})
}

// projectDiskCard shows reinstallable dependency folders (node_modules, .venv,
// etc.) with proportional usage bars, total size, and a link to Cleanup.
func (a *App) projectDiskCard(c *ui.Context, p projects.Project) {
	if len(p.Installs) == 0 {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	var total, maxSize int64
	sizedAny := false
	sizes := make([]int64, len(p.Installs))
	hasSize := make([]bool, len(p.Installs))
	for i, d := range p.Installs {
		if sz, ok := a.pathSize(filepath.Join(p.Dir, d)); ok {
			sizes[i] = sz
			hasSize[i] = true
			total += sz
			if sz > maxSize {
				maxSize = sz
			}
			sizedAny = true
		}
	}
	ui.Column(c.Key("project-disk-card")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectDiskTitle()).FontSize(13.5).FontWeight(600).SingleLine().Shrink(0)
			if sizedAny {
				ui.Text(c, formatBytes(total)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine().Shrink(0)
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			pageLinkAction(c, "project-disk-cleanup", locales.ProjectDiskOpenCleanup(), func() {
				a.router.Push(cleanupPage)
			})
		})
		for i, d := range p.Installs {
			full := filepath.Join(p.Dir, d)
			szText := "…"
			var ratio float32
			if hasSize[i] {
				szText = formatBytes(sizes[i])
				if maxSize > 0 {
					ratio = float32(sizes[i]) / float32(maxSize)
					if ratio < 0.06 && sizes[i] > 0 {
						ratio = 0.06
					}
				}
			}
			ui.Row(c.Key("project-disk-row:"+d)).Gap(10).Padding(9, 14).MinHeight(40).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				clipped(c, d).Font(FontMono).FontSize(12.5).Grow(1).MinWidth(0)
				if sizedAny && !narrow(c) {
					const barTrackW = float32(64)
					ui.Box(c).Width(barTrackW).Height(4).Radius(2).Background(pal.Muted).Clip().Shrink(0).Children(func() {
						if ratio > 0 {
							ui.Box(c).Width(barTrackW * ratio).Height(4).Radius(2).Background(Accent)
						}
					})
				}
				ui.Text(c, szText).Font(FontMono).FontSize(12).TextColor(t.TextMuted).Width(60).TextAlign(ui.End).SingleLine().Shrink(0)
				if a.Shell != nil {
					iconButton(c.Key("project-disk-open:"+d), "folder-open", showInFolder()).OnClick(func() {
						a.Shell.ShowItemInFolder(full)
					})
				}
			})
		}
	})
}

func (a *App) projectServicesTab(c *ui.Context, p projects.Project) {
	hasCompose := len(composeProjects(p)) > 0
	_, kubeLinked := a.KubeLinks[p.Dir]
	hasRunning := a.projectRunningCounts(p) > 0 || len(a.projectComposeContainers(p)) > 0
	hasAny := hasRunning || len(p.Scripts) > 0 || hasCompose || len(p.Databases) > 0 || kubeLinked || len(p.Kube) > 0
	if !hasAny {
		listCard(c, locales.ProjectTabRunServices(), "", func() {
			cardNote(c, locales.ProjectServicesEmpty())
		})
		return
	}
	a.projectRunningCard(c, p)
	a.projectScriptsCard(c, p)
	a.projectDockerCard(c, p)
	a.projectDatabasesCard(c, p)
	a.projectKubeCard(c, p)
}

func (a *App) projectAITab(c *ui.Context, p projects.Project) {
	a.projectSkillsCard(c, p)
	a.projectMCPCard(c, p)
	a.projectInstructionsCard(c, p)
	a.projectAccessCard(c, p)
}

func (a *App) projectSkillsCard(c *ui.Context, p projects.Project) {
	t := c.Theme()
	pal := Colors(t)
	foundIDs := a.foundIDs()
	ui.Column(c.Key("project-ai-skills")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, locales.SkillsTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			if len(p.Skills) > 0 {
				ui.Text(c, strconv.Itoa(len(p.Skills))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if a.BuildPlan != nil {
				ui.Button(c.Key("project-skill-create"), locales.SkillCreateButton()).Height(controlHeight).OnClick(func() {
					a.openSkillCreate(p.Dir)
				})
				if a.LoadCatalog != nil || a.SearchSkillsSh != nil {
					ui.Button(c.Key("project-skill-install"), locales.ProjectSkillsInstall()).Height(controlHeight).OnClick(func() {
						a.openSkillCatalogForProject(p.Dir)
					})
				}
			}
			pageLinkAction(c, "project-skills-all", locales.ProjectSkillsOpenAll(), func() {
				a.skillScope = 2
				a.skillProject = p.Dir
				a.router.Push("/ai/skills")
			})
		})
		if len(p.Skills) == 0 {
			cardNote(c, locales.ProjectSkillsEmpty())
			return
		}
		skills := slices.Clone(p.Skills)
		slices.SortStableFunc(skills, func(x, y inventory.Skill) int { return a.byTitle(x.Name, y.Name) })
		for _, sk := range skills {
			skill := sk
			key := skillKey(skill)
			ui.Column(c.Key("proj-skill:"+key)).Gap(8).Padding(10, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					openRow := ui.ButtonBase(c.Key("proj-skill-open:" + key)).Row().Grow(1).MinWidth(0).Gap(10).
						AlignItems(ui.Center).Label(skill.Name).OnClick(func() {
						a.skillScope = 2
						a.skillProject = p.Dir
						a.skillChosen = key
						a.router.Push("/ai/skills")
					})
					openRow.Children(func() {
						ui.Box(c).Size(27, 27).Shrink(0).Radius(7).Center().Background(pal.Subtle).
							BorderWidth(1).BorderColor(pal.Line).Children(func() {
							ui.Icon(c, Icon("book-open")).Size(14, 14).TextColor(t.TextMuted)
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, skill.Name).FontSize(13.5).FontWeight(500).SingleLine()
								if skill.Broken {
									aiTag(c, locales.TagBroken(), pal.WarningSoft, pal.WarningText)
								}
							})
							if skill.Description != "" {
								clipped(c, skill.Description).FontSize(11.5).TextColor(t.TextMuted)
							}
						})
					})
					if a.canOpenFile() && !skill.Broken {
						iconButton(c.Key("proj-skill-edit:"+key), "external-link", locales.OpenInEditor()).OnClick(func() {
							a.openFile(c, filepath.Join(skill.Dir, "SKILL.md"))
						})
					}
					if a.Shell != nil && !skill.Broken {
						iconButton(c.Key("proj-skill-dir:"+key), "folder-open", showInFolder()).OnClick(func() {
							a.Shell.ShowItemInFolder(skill.Dir)
						})
					}
				})
				if len(foundIDs) > 0 {
					ui.Row(c).Gap(10).Padding(0, 0, 0, 37).AlignItems(ui.Center).Wrap().Children(func() {
						for _, id := range foundIDs {
							if id == "aider" {
								continue
							}
							var via *inventory.SkillPath
							for j, sp := range skill.Paths {
								if slices.Contains(sp.Agents, id) {
									via = &skill.Paths[j]
									break
								}
							}
							ui.Row(c.Key("proj-skill-ag:"+key+":"+id)).Gap(6).Padding(4, 8).Radius(7).
								AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
								agentLogo(c, id, 13)
								ui.Text(c, agentTitle(id)).FontSize(11.5).SingleLine()
								if via != nil {
									if via.Link {
										aiTag(c, locales.TagSymlink(), pal.Info.Alpha(0.12), pal.Info)
									} else {
										ui.Box(c).Shrink(0).Padding(2, 6).Radius(5).Background(pal.SuccessSoft).Tooltip(locales.SkillNativeHint()).Children(func() {
											ui.Text(c, locales.TagNative()).FontSize(10).FontWeight(500).TextColor(pal.SuccessText).SingleLine()
										})
									}
								}
								a.skillAgentControl(c, skill, id, via)
							})
						}
					})
				}
			})
		}
	})
}

func (a *App) projectMCPCard(c *ui.Context, p projects.Project) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key("project-ai-mcp")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.McpTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			if len(p.MCP) > 0 {
				ui.Text(c, strconv.Itoa(len(p.MCP))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			a.pageLink(c, "project-mcp-all", locales.ProjectMcpOpenAll(), "/ai/mcp")
		})
		if len(p.MCP) == 0 {
			cardNote(c, locales.ProjectMcpEmpty())
			return
		}
		for i, s := range p.MCP {
			caption := launch(s)
			if s.Agent != "" {
				if caption != "" {
					caption = agentTitle(s.Agent) + " · " + caption
				} else {
					caption = agentTitle(s.Agent)
				}
			}
			listRow(c, s.Name, false, caption, func() {
				if s.PlainSecret() {
					aiTag(c, locales.McpSecretTitle(), pal.WarningSoft, pal.WarningText)
				}
				switch {
				case s.Disabled:
					chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.TagOff())
				case s.Missing:
					chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.CommandMissing())
				default:
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, s.Transport)
				}
				if s.File != "" && a.canOpenFile() {
					cfg := s.File
					iconButton(c.Key("proj-mcp-open:"+strconv.Itoa(i)), "external-link", locales.OpenInEditor()).OnClick(func() {
						a.openFile(c, cfg)
					})
				}
			})
		}
	})
}

func (a *App) projectInstructionsCard(c *ui.Context, p projects.Project) {
	t := c.Theme()
	pal := Colors(t)
	state := p.Instructions.State()
	canConnect := a.BuildPlan != nil && (state == "agents" || state == "claude" || state == "same")
	stateChip := func() {
		switch state {
		case "linked", "same":
			chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, instructionProjectState(p.Instructions))
		case "different", "unreadable":
			chip(c, t.Warning, pal.WarningSoft, pal.WarningText, instructionProjectState(p.Instructions))
		default:
			chip(c, pal.TextFaint, pal.Muted, t.TextMuted, instructionProjectState(p.Instructions))
		}
	}
	ui.Column(c.Key("project-ai-instructions")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, locales.InstructionsTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			if state != "none" || len(p.AI) > 0 {
				stateChip()
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if canConnect {
				ui.Button(c.Key("project-instr-connect"), locales.InstructionsConnect()).Height(controlHeight).OnClick(func() {
					a.connectInstructions(p.Dir)
				})
			}
			pageLinkAction(c, "project-instr-open", locales.ProjectInstructionsOpen(), func() {
				a.instructionsChosen = p.Dir
				a.router.Push(instructionsPage)
			})
		})
		if state == "none" && len(p.AI) == 0 {
			cardNote(c, instructionProjectState(p.Instructions))
			return
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
						iconButton(c.Key("proj-ai-tab-open:"+name), "folder-open", showInFolder()).OnClick(func() {
							if err := a.Shell.OpenPath(full); err != nil {
								c.Toast(locales.OpenFailed(tilde(full, a.Home), err.Error()))
							}
						})
					}
				default:
					if a.canOpenFile() {
						iconButton(c.Key("proj-ai-tab-open:"+name), "external-link", locales.OpenInEditor()).OnClick(func() {
							a.openFile(c, full)
						})
					}
				}
			})
		}
	})
}

// projectAccessCard shows project-level agent permissions (allow/ask/deny) and
// hooks in a compact standalone card, separate from prose instructions.
func (a *App) projectAccessCard(c *ui.Context, p projects.Project) {
	if len(p.Rules) == 0 && len(p.Hooks) == 0 {
		return
	}
	t := c.Theme()
	pal := Colors(t)

	type ruleGroup struct {
		agent            string
		file             string
		allow, ask, deny int
		total            int
	}
	var ruleGroups []ruleGroup
	ruleIndex := map[string]int{}
	var seenAgents []string
	recordAgent := func(id string) {
		if id != "" && !slices.Contains(seenAgents, id) {
			seenAgents = append(seenAgents, id)
		}
	}
	for _, r := range p.Rules {
		recordAgent(r.Agent)
		k := r.Agent + "\x00" + r.File
		idx, ok := ruleIndex[k]
		if !ok {
			idx = len(ruleGroups)
			ruleIndex[k] = idx
			ruleGroups = append(ruleGroups, ruleGroup{agent: r.Agent, file: r.File})
		}
		g := &ruleGroups[idx]
		g.total++
		switch r.Kind {
		case inventory.RuleAllow:
			g.allow++
		case inventory.RuleAsk:
			g.ask++
		case inventory.RuleDeny:
			g.deny++
		}
	}

	type hookGroup struct {
		agent string
		file  string
		total int
	}
	var hookGroups []hookGroup
	hookIndex := map[string]int{}
	for _, h := range p.Hooks {
		recordAgent(h.Agent)
		k := h.Agent + "\x00" + h.File
		idx, ok := hookIndex[k]
		if !ok {
			idx = len(hookGroups)
			hookIndex[k] = idx
			hookGroups = append(hookGroups, hookGroup{agent: h.Agent, file: h.File})
		}
		hookGroups[idx].total++
	}

	targetRoute := "/ai/agents"
	if len(seenAgents) == 1 && slices.ContainsFunc(a.agents(), func(ag inventory.Agent) bool { return ag.ID == seenAgents[0] }) {
		targetRoute = agentPath(seenAgents[0])
	}

	ui.Column(c.Key("project-ai-access")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ProjectRulesTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, strconv.Itoa(len(p.Rules)+len(p.Hooks))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			a.pageLink(c, "project-access-open", locales.ProjectAgentsOpen(), targetRoute)
		})
		for i, g := range ruleGroups {
			title := locales.ProjectRulesTitle()
			if g.agent != "" {
				title = agentTitle(g.agent)
			}
			var parts []string
			if rel := projectRelPath(p.Dir, g.file, a.Home); rel != "" {
				parts = append(parts, rel)
			}
			if g.allow > 0 {
				parts = append(parts, locales.ProjectRulesAllow(g.allow))
			}
			if g.ask > 0 {
				parts = append(parts, locales.ProjectRulesAsk(g.ask))
			}
			if g.deny > 0 {
				parts = append(parts, locales.ProjectRulesDeny(g.deny))
			}
			if g.allow == 0 && g.ask == 0 && g.deny == 0 {
				parts = append(parts, locales.ProjectRulesCount(g.total))
			}
			file := g.file
			listRow(c, title, false, strings.Join(parts, " · "), func() {
				if file != "" && a.canOpenFile() {
					iconButton(c.Key("proj-rule-open:"+strconv.Itoa(i)), "external-link", locales.OpenInEditor()).OnClick(func() {
						a.openFile(c, file)
					})
				}
			})
		}
		for i, g := range hookGroups {
			title := locales.HooksCard()
			if g.agent != "" {
				title = agentTitle(g.agent) + " · " + locales.HooksCard()
			}
			var parts []string
			if rel := projectRelPath(p.Dir, g.file, a.Home); rel != "" {
				parts = append(parts, rel)
			}
			parts = append(parts, locales.ProjectHooksCount(g.total))
			file := g.file
			listRow(c, title, false, strings.Join(parts, " · "), func() {
				if file != "" && a.canOpenFile() {
					iconButton(c.Key("proj-hook-open:"+strconv.Itoa(i)), "external-link", locales.OpenInEditor()).OnClick(func() {
						a.openFile(c, file)
					})
				}
			})
		}
	})
}

func projectRelPath(projectDir, file, home string) string {
	if file == "" {
		return ""
	}
	if projectDir != "" {
		if rel, err := filepath.Rel(projectDir, file); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return tilde(file, home)
}

func (a *App) projectFilesTab(c *ui.Context, p projects.Project) {
	a.filesCard(c, p)
	a.projectDiskCard(c, p)
	if p.Readme != "" {
		full := filepath.Join(p.Dir, p.Readme)
		listCard(c, p.Readme, "", func() {
			listRow(c, p.Readme, true, tilde(full, a.Home), func() {
				if a.canOpenFile() {
					iconButton(c.Key("proj-files-readme"), "external-link", locales.OpenInEditor()).OnClick(func() {
						a.openFile(c, full)
					})
				}
			})
		})
	}
}

// projectDatabaseStartPlan finds a stopped local service matching ref.Kind
// that Hearth can start directly.
func (a *App) projectDatabaseStartPlan(ref projects.DatabaseReference, servers []inventory.Database) (ops.Plan, bool) {
	for _, db := range servers {
		if db.Kind != ref.Kind || db.Status != "stopped" || db.Source == "docker" || db.Service == "" {
			continue
		}
		if plan, ok := a.servicePlan(db); ok {
			return plan, true
		}
	}
	return ops.Plan{}, false
}

func portsPathWithFilter(filter string) string {
	if filter == "" {
		return portsPage
	}
	return portsPage + "?filter=" + url.QueryEscape(filter)
}
