package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// skillFolder is what was read of the folder of the skill shown in the
// details: its files and SKILL.md.
type skillFolder struct {
	dir     string
	loaded  bool
	entries []inventory.Entry
	text    string
	err     string
}

// skillGroup is a scope's skills in the list: the user's or a project's.
type skillGroup struct {
	project string
	skills  []inventory.Skill
}

// skillKey tells a skill apart across scopes: its project ("" for user) plus
// its folder, which stays when a link comes or goes; for a broken link, the link.
func skillKey(sk inventory.Skill) string {
	path := sk.Dir
	if sk.Broken && len(sk.Paths) > 0 {
		path = sk.Paths[0].Path
	}
	return sk.Project + "\x00" + path
}

// projectDisplayNames returns readable unique labels for project directories,
// using the folder basename when unique and disambiguating collisions with
// parent path segments (for example "work/api" vs "personal/api").
func projectDisplayNames(dirs []string, home string) []string {
	out := make([]string, len(dirs))
	counts := map[string]int{}
	for i, dir := range dirs {
		out[i] = filepath.Base(dir)
		counts[out[i]]++
	}
	for i, dir := range dirs {
		if counts[out[i]] <= 1 {
			continue
		}
		parent := filepath.Base(filepath.Dir(dir))
		if parent != "" && parent != "." && parent != string(filepath.Separator) {
			out[i] = parent + "/" + out[i]
		} else {
			out[i] = tilde(dir, home)
		}
	}
	// If two projects share even parent/base, fall back to tilde paths for them.
	counts2 := map[string]int{}
	for _, name := range out {
		counts2[name]++
	}
	for i, dir := range dirs {
		if counts2[out[i]] > 1 {
			out[i] = tilde(dir, home)
		}
	}
	return out
}

// activeSkillProject returns a.skillProject when it is still a tracked project
// and the current scope includes projects, else "".
func (a *App) activeSkillProject() string {
	if a.skillScope == 1 || a.skillProject == "" {
		return ""
	}
	for _, p := range a.Projects {
		if p.Dir == a.skillProject {
			return a.skillProject
		}
	}
	return ""
}

// skillsPage lists the skills of the user and of the projects, filtered by
// agent, scope, project, and text, with the chosen one's details beside them.
func (a *App) skillsPage(c *ui.Context, p page) {
	if a.skillCatalogOpen {
		a.skillCatalogView(c, p)
		return
	}
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if a.BuildPlan != nil {
				ui.Button(c.Key("skill-create"), "").Height(controlHeight).Label(locales.SkillCreateButton()).OnClick(func() {
					a.openSkillCreate(a.activeSkillProject())
				}).Children(func() {
					ui.Icon(c, Icon("plus")).Size(13, 13).TextColor(c.Theme().TextMuted)
					ui.Text(c, locales.SkillCreateButton()).SingleLine()
				})
			}
			ui.PrimaryButton(c.Key("skill-catalog"), locales.SkillCatalogTitle()).Height(controlHeight).OnClick(func() { a.openSkillCatalog(0) })
		}})
		all := a.allSkills()
		if len(all) == 0 {
			a.aiEmpty(c, "puzzle", locales.NoSkills())
			return
		}
		copies, differs := skillCopies(all)
		withCopies, inProjects := 0, 0
		for _, n := range copies {
			if n > 1 {
				withCopies++
			}
		}
		for _, sk := range all {
			if sk.Project != "" {
				inProjects++
			}
		}
		activeProject := a.activeSkillProject()
		groups := a.skillGroups()
		var shown []inventory.Skill
		for _, g := range groups {
			shown = append(shown, g.skills...)
		}
		chosen := slices.IndexFunc(shown, func(sk inventory.Skill) bool { return skillKey(sk) == a.skillChosen })
		if chosen < 0 && len(shown) > 0 {
			chosen = 0
		}
		filters := func() {
			aiIntro(c, "puzzle", locales.SkillsTitle(), locales.SkillsText(), []aiStat{
				{strconv.Itoa(len(all)), locales.StatTotal()},
				{strconv.Itoa(inProjects), locales.StatProjects()},
				{strconv.Itoa(withCopies), locales.StatWithCopies()},
			})
			bar := ui.Row(c).Gap(10).AlignItems(ui.Center)
			if narrow(c) {
				bar.Wrap()
			}
			bar.Children(func() {
				a.agentFilter(c, "skill-agent", &a.skillAgent, func(id string) int {
					n := 0
					for _, sk := range all {
						if !inScope(a.skillScope, sk.Project) {
							continue
						}
						if activeProject != "" && sk.Project != activeProject {
							continue
						}
						if id == "" || slices.Contains(skillAgents(sk), id) {
							n++
						}
					}
					return n
				})
				if len(a.Projects) > 0 && a.skillScope != 1 {
					dirs := make([]string, len(a.Projects))
					for i, pr := range a.Projects {
						dirs[i] = pr.Dir
					}
					names := projectDisplayNames(dirs, a.Home)
					options := append([]string{locales.SkillsAllProjects()}, names...)
					selected := options[0]
					if idx := slices.Index(dirs, activeProject); idx >= 0 {
						selected = options[idx+1]
					}
					ui.Select(c.Key("skill-project-filter"), &selected, options).Height(controlHeight).Width(170).OnChange(func() {
						if idx := slices.Index(options, selected); idx > 0 {
							a.skillProject = dirs[idx-1]
						} else {
							a.skillProject = ""
						}
					})
				}
				scopeSegments(c, "skill-scope", &a.skillScope)
			})
		}
		if narrow(c) {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(16).Padding(22, 28, 24, 28).Children(func() {
					filters()
					a.skillList(c, groups, copies, differs, shown, chosen)
					if withCopies > 0 {
						a.copiesHint(c, withCopies)
					}
					if chosen >= 0 {
						a.skillDetail(c, shown[chosen], all)
					}
				})
			})
			return
		}
		// Sticky split view: intro and filters stay fixed while the left list
		// and right detail pane scroll independently.
		ui.Column(c).Grow(1).MinHeight(0).Gap(16).Padding(22, 28, 0, 28).Children(func() {
			filters()
			ui.Row(c).Grow(1).MinHeight(0).Gap(18).AlignItems(ui.Stretch).Children(func() {
				ui.Scroll(c.Key("skills-list-scroll")).Grow(1).Basis(0).MinWidth(0).Children(func() {
					ui.Column(c).Gap(14).Padding(0, 0, 24, 0).Children(func() {
						a.skillList(c, groups, copies, differs, shown, chosen)
						if withCopies > 0 {
							a.copiesHint(c, withCopies)
						}
					})
				})
				ui.Scroll(c.Key("skills-detail-scroll")).Width(420).Shrink(0).Children(func() {
					ui.Column(c).Padding(0, 0, 24, 0).Children(func() {
						if chosen >= 0 {
							a.skillDetail(c, shown[chosen], all)
						}
					})
				})
			})
		})
	})
}

// skillCopies counts the folders of each skill name in each scope, broken
// links and skills synced from claude.ai aside, as issues counts them, and
// reports which groups have differing file sums.
func skillCopies(skills []inventory.Skill) (map[string]int, map[string]bool) {
	n := map[string]int{}
	sums := map[string]string{}
	differs := map[string]bool{}
	for _, sk := range skills {
		if !sk.Broken && !sk.Synced {
			key := sk.Project + "\x00" + sk.Name
			n[key]++
			if prev, ok := sums[key]; !ok {
				sums[key] = sk.Sum
			} else if prev != sk.Sum {
				differs[key] = true
			}
		}
	}
	return n, differs
}

// skillGroups are the skills shown: global/user skills first, then each
// project's skills in their order.
func (a *App) skillGroups() []skillGroup {
	activeProject := a.activeSkillProject()
	q := strings.ToLower(strings.TrimSpace(a.skillQuery))
	keep := func(sk inventory.Skill) bool {
		return inScope(a.skillScope, sk.Project) &&
			(activeProject == "" || sk.Project == activeProject) &&
			(a.skillAgent == "" || slices.Contains(skillAgents(sk), a.skillAgent)) &&
			(q == "" || strings.Contains(strings.ToLower(sk.Name+" "+sk.Description), q))
	}
	var groups []skillGroup
	add := func(project string, skills []inventory.Skill) {
		g := skillGroup{project: project}
		for _, sk := range skills {
			if keep(sk) {
				g.skills = append(g.skills, sk)
			}
		}
		if len(g.skills) > 0 {
			slices.SortStableFunc(g.skills, func(x, y inventory.Skill) int { return a.byTitle(x.Name, y.Name) })
			groups = append(groups, g)
		}
	}
	if a.Snapshot != nil {
		add("", a.Snapshot.Skills)
	}
	for _, p := range a.Projects {
		add(p.Dir, p.Skills)
	}
	return groups
}

// skillGroupExpanded reports whether the skill group for dir ("" for User) is
// expanded. User defaults to open; project groups default to collapsed unless
// explicitly opened, filtered to that project, searching, or the sole group.
func (a *App) skillGroupExpanded(dir string, groups []skillGroup) bool {
	if strings.TrimSpace(a.skillQuery) != "" {
		return true
	}
	if dir != "" && a.activeSkillProject() == dir {
		return true
	}
	if open, ok := a.skillGroupOpen[dir]; ok {
		return open
	}
	if dir == "" {
		return true
	}
	return len(groups) == 1
}

func (a *App) toggleSkillGroup(dir string, currentlyOpen bool) {
	if a.skillGroupOpen == nil {
		a.skillGroupOpen = map[string]bool{}
	}
	a.skillGroupOpen[dir] = !currentlyOpen
}

// skillList is the search field and the skills grouped by scope (global first,
// then collapsible project groups).
func (a *App) skillList(c *ui.Context, groups []skillGroup, copies map[string]int, differs map[string]bool, shown []inventory.Skill, chosen int) {
	t := c.Theme()
	pal := Colors(t)
	projectDirs := make([]string, 0, len(a.Projects))
	for _, p := range a.Projects {
		projectDirs = append(projectDirs, p.Dir)
	}
	projectLabels := projectDisplayNames(projectDirs, a.Home)
	labelForProject := func(dir string) string {
		if idx := slices.Index(projectDirs, dir); idx >= 0 {
			return locales.ScopeProjectOf(projectLabels[idx])
		}
		return scopeName(dir)
	}
	listFrame(c, func() {
		ui.Row(c).Padding(6, 10).Children(func() {
			ui.SearchField(c.Key("skill-search"), &a.skillQuery).Bind(&a.filterField).Placeholder(locales.SearchByName()).Grow(1)
		})
		if len(groups) == 0 {
			cardNote(c, locales.NothingMatches())
		}
		for _, g := range groups {
			dir := g.project
			expanded := a.skillGroupExpanded(dir, groups)
			chev := "chevron-down"
			if !expanded {
				chev = "chevron-right"
			}
			roots := a.roots(g)
			if dir == "" {
				ui.Column(c.Key("skill-group:user")).Gap(3).Padding(8, 14).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					toggleBtn := ui.ButtonBase(c.Key("skill-group-toggle:user")).Row().FillWidth().Gap(6).
						Justify(ui.Start).AlignItems(ui.Center).
						Label(scopeName("")).OnClick(func() {
						a.toggleSkillGroup("", expanded)
					})
					toggleBtn.Children(func() {
						ui.Icon(c, Icon(chev)).Size(13, 13).Shrink(0).TextColor(pal.TextFaint)
						ui.Text(c, strings.ToUpper(scopeName(""))).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Shrink(0).SingleLine()
						ui.Text(c, strconv.Itoa(len(g.skills))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Shrink(0).SingleLine()
						ui.Box(c).Grow(1).MinWidth(8).Height(1)
						if roots != "" {
							clipped(c, roots).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(1).MinWidth(0)
						}
					})
					if expanded {
						ui.Text(c, locales.SkillsScopeExplain()).FontSize(11).TextColor(t.TextMuted)
					}
				})
			} else {
				row := ui.Row(c.Key("skill-group:"+dir)).Gap(8).Padding(7, 10, 7, 14).AlignItems(ui.Center).
					Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
				row.Children(func() {
					toggleBtn := ui.ButtonBase(c.Key("skill-group-toggle:" + dir)).Row().Grow(1).MinWidth(0).Gap(6).
						Justify(ui.Start).AlignItems(ui.Center).
						Label(labelForProject(dir)).OnClick(func() {
						a.toggleSkillGroup(dir, expanded)
					})
					toggleBtn.Children(func() {
						ui.Icon(c, Icon(chev)).Size(13, 13).Shrink(0).TextColor(pal.TextFaint)
						ui.Text(c, strings.ToUpper(labelForProject(dir))).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Shrink(0).SingleLine()
						ui.Text(c, strconv.Itoa(len(g.skills))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Shrink(0).SingleLine()
						ui.Box(c).Grow(1).MinWidth(8).Height(1)
						if roots != "" {
							clipped(c, roots).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(1).MinWidth(0)
						}
					})
					iconButton(c.Key("skill-group-open:"+dir), "folder-git-2", locales.OpenProject()).OnClick(func() {
						a.router.Push(projectPath(dir))
					})
				})
			}
			if !expanded {
				continue
			}
			for _, sk := range g.skills {
				key := skillKey(sk)
				e := ui.ButtonBase(c.Key("skill:"+key)).Gap(10).Padding(9, 14, 9, 12).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(sk.Name)
				switch {
				case chosen >= 0 && skillKey(shown[chosen]) == key:
					e.Background(pal.ActiveRow)
				case e.Hovered():
					e.Background(pal.Subtle)
				}
				e.Children(func() {
					ui.Box(c).Size(27, 27).Shrink(0).Radius(7).Center().Background(pal.Subtle).
						BorderWidth(1).BorderColor(pal.Line).Children(func() {
						ui.Icon(c, Icon("book-open")).Size(15, 15).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, sk.Name).FontSize(13.5).SingleLine()
						if sk.Description != "" {
							ui.Text(c, sk.Description).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
						}
					})
					copyKey := sk.Project + "\x00" + sk.Name
					switch n := copies[copyKey]; {
					case sk.Broken:
						aiTag(c, locales.TagBroken(), pal.WarningSoft, pal.WarningText)
					case sk.Synced:
						aiTag(c, locales.TagSynced(), pal.Info.Alpha(0.12), pal.Info)
					case differs[copyKey]:
						aiTag(c, locales.TagDiffers(), pal.WarningSoft, pal.WarningText)
					case n > 1:
						aiTag(c, locales.ChipCopies(n), pal.WarningSoft, pal.WarningText)
					}
					agentAvatars(c, a.shownAgents(skillAgents(sk)))
				})
				if e.Clicked() {
					a.skillChosen = key
				}
			}
		}
	})
}

// shownAgents are those of ids on the computer: the avatars of agents not
// installed would only be noise.
func (a *App) shownAgents(ids []string) []string {
	found := a.foundIDs()
	var out []string
	for _, id := range ids {
		if slices.Contains(found, id) {
			out = append(out, id)
		}
	}
	return out
}

// roots are the skills directories of a group's skills, as "~/.claude/skills
// · ~/.agents/skills", relative to the project for a project's. Claude synced
// skills collapse their per-organization UUID subfolder to ".../skills/synced".
func (a *App) roots(g skillGroup) string {
	var out []string
	for _, sk := range g.skills {
		for _, p := range sk.Paths {
			dir := filepath.Dir(p.Path)
			if sk.Synced {
				parent := filepath.Dir(dir)
				if filepath.Base(parent) == "synced" {
					dir = parent
				}
			}
			root := tilde(dir, a.Home)
			if g.project != "" {
				if rel, err := filepath.Rel(g.project, dir); err == nil {
					root = filepath.ToSlash(rel)
				}
			}
			if !slices.Contains(out, root) {
				out = append(out, root)
			}
		}
	}
	return strings.Join(out, " · ")
}

// copiesHint tells how many skills have copies, and what to do about them.
func (a *App) copiesHint(c *ui.Context, n int) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c).Gap(10).Padding(12, 14).Radius(12).AlignItems(ui.Center).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Icon(c, Icon("sparkles")).Size(16, 16).Shrink(0).TextColor(Accent)
		ui.Text(c, locales.SkillsCopiesHint(n)).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
		if groups := a.skillCopyGroups(c); len(groups) > 0 && a.BuildPlan != nil && a.SkillBackups != "" {
			backups := a.SkillBackups
			ui.Button(c.Key("skill-merge"), locales.SkillMerge()).Shrink(0).OnClick(func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.SkillMergePlan(groups, backups, env, time.Now())
				}, a.proposeOrSay)
			})
		}
	})
}

// skillCopyGroups are the skills whose copies hold the same files, each
// with its copies: the ones a merge keeps one folder of.
func (a *App) skillCopyGroups(c *ui.Context) [][]inventory.Skill {
	var out [][]inventory.Skill
	for _, is := range a.issues(c) {
		if is.Kind == issues.SkillCopies {
			out = append(out, is.Skills)
		}
	}
	return out
}

// skillAgentControl is how the agent id gets sk, which via is the path it
// finds it by, nil for none: a button to connect it, or for a link a
// switch that disconnects it. The plan is made in the background: it looks at the
// disk, and says why when it cannot be made.
func (a *App) skillAgentControl(c *ui.Context, sk inventory.Skill, id string, via *inventory.SkillPath) {
	// Aider reads no skills at all; Claude writes a synced skill over.
	if a.BuildPlan == nil || sk.Broken || sk.Synced || id == "aider" {
		return
	}
	if via == nil {
		ui.Button(c.Key("connect:"+id), locales.SkillConnect()).Shrink(0).OnClick(func() {
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.SkillLinkPlan(sk, id, env, agentTitle)
			}, a.proposeOrSay)
		})
		return
	}
	// The skill's own folder is not a link to take away.
	if !via.Link {
		return
	}
	on, path := true, via.Path
	label := locales.SkillConnectedTo(agentTitle(id))
	toggle(c, "connected:"+id, &on).Label(label).Tooltip(label).Disabled(a.op.running()).OnChange(func() {
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			return ops.SkillUnlinkPlan(sk, path, env, agentTitle)
		}, a.proposeOrSay)
	})
}

// skillDetail is the chosen skill: what it is, where it is, the agents it
// is available to, its files and SKILL.md, and actions on it.
func (a *App) skillDetail(c *ui.Context, sk inventory.Skill, all []inventory.Skill) {
	t := c.Theme()
	pal := Colors(t)
	a.loadSkill(sk)
	peers := skillCopyPeers(all, sk)
	listFrame(c, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sk.Name).FontSize(17).FontWeight(500).Shrink(1).MinWidth(0).SingleLine()
				aiTag(c, scopeName(sk.Project), pal.AccentSoft, pal.AccentInk)
				if sk.Synced {
					aiTag(c, locales.TagSynced(), pal.Info.Alpha(0.12), pal.Info)
				}
			})
			if sk.Description != "" {
				ui.Text(c, sk.Description).FontSize(12.5).TextColor(t.TextMuted)
			}
			pathChip(c, tilde(sk.Dir, a.Home), sk.Dir)
			if sk.Err != "" {
				ui.Text(c, sk.Err).FontSize(12).TextColor(pal.WarningText)
			}
			if sk.Synced {
				ui.Text(c, locales.SkillSyncedNote()).FontSize(12).TextColor(t.TextMuted)
			}
			if skillCopiesDiffer(peers) {
				ui.Row(c).Gap(8).Padding(8, 10).Radius(8).AlignItems(ui.Center).
					Background(pal.WarningSoft).Children(func() {
					ui.Text(c, locales.SkillDiffersBanner()).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
					if a.DiffSkills != nil {
						ui.Button(c.Key("skill-compare"), locales.SkillCompareCopies()).Shrink(0).OnClick(func() {
							a.openSkillDiff(sk, peers)
						})
					}
				})
			}
		})
		var unconnected []string
		foundIDs := a.foundIDs()
		for _, id := range foundIDs {
			if id == "aider" {
				continue
			}
			connected := false
			for _, p := range sk.Paths {
				if slices.Contains(p.Agents, id) {
					connected = true
					break
				}
			}
			if !connected {
				unconnected = append(unconnected, id)
			}
		}
		if a.BuildPlan != nil && !sk.Broken && !sk.Synced && len(unconnected) > 1 {
			targets := slices.Clone(unconnected)
			ui.Row(c).Padding(10, 16, 4, 16).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, strings.ToUpper(locales.AvailableToAgents())).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Grow(1).SingleLine()
				ui.Button(c.Key("skill-connect-all"), locales.SkillConnectAll()).Shrink(0).OnClick(func() {
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.SkillLinkAllPlan(sk, targets, env, agentTitle)
					}, a.proposeOrSay)
				})
			})
		} else {
			detailSection(c, locales.AvailableToAgents())
		}
		for _, id := range foundIDs {
			var via *inventory.SkillPath
			for i, p := range sk.Paths {
				if slices.Contains(p.Agents, id) {
					via = &sk.Paths[i]
					break
				}
			}
			ui.Row(c).Gap(10).Padding(5, 16).AlignItems(ui.Center).Children(func() {
				agentTile(c, id, 24)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, agentTitle(id)).FontSize(13).SingleLine()
					caption := locales.NotConnected()
					if via != nil {
						caption = tilde(via.Path, a.Home)
					}
					ui.Text(c, caption).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				})
				if via != nil {
					if via.Link {
						aiTag(c, locales.TagSymlink(), pal.Info.Alpha(0.12), pal.Info)
					} else {
						ui.Box(c).Shrink(0).Padding(2, 7).Radius(5).Background(pal.SuccessSoft).Tooltip(locales.SkillNativeHint()).Children(func() {
							ui.Text(c, locales.TagNative()).FontSize(10.5).FontWeight(500).TextColor(pal.SuccessText).SingleLine()
						})
					}
				}
				a.skillAgentControl(c, sk, id, via)
			})
		}
		detailSection(c, locales.FilesSection())
		a.skillFiles(c, sk)
		canRemoveBroken := sk.Broken && a.BuildPlan != nil
		canRemove := !sk.Broken && !sk.Synced && a.BuildPlan != nil
		canOpen := a.canOpenFile() && !sk.Broken
		canReveal := a.Shell != nil && !sk.Broken
		hasOpen := canOpen || canReveal
		if hasOpen || canRemove || canRemoveBroken {
			ui.Row(c).Gap(8).Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Wrap().Children(func() {
				if canOpen {
					ui.Button(c.Key("skill-open"), "").Label(locales.OpenInEditor()).Tooltip(locales.OpenInEditor()).Shrink(0).
						OnClick(func() { a.openFile(c, filepath.Join(sk.Dir, "SKILL.md")) }).Children(func() {
						ui.Icon(c, Icon("external-link")).Size(13, 13).TextColor(t.TextMuted)
						ui.Text(c, locales.OpenShort()).SingleLine()
					})
				}
				if canReveal {
					ui.Button(c.Key("skill-reveal"), "").Label(showInFolder()).Tooltip(showInFolder()).Shrink(0).
						OnClick(func() { a.Shell.ShowItemInFolder(sk.Dir) }).Children(func() {
						ui.Icon(c, Icon("folder-open")).Size(13, 13).TextColor(t.TextMuted)
						ui.Text(c, showInFolderShort()).SingleLine()
					})
				}
				if hasOpen && (canRemove || canRemoveBroken) {
					ui.Box(c).Grow(1).MinWidth(0).Height(1)
				}
				if canRemove {
					ui.Button(c.Key("skill-remove"), locales.SkillRemoveButton()).Shrink(0).OnClick(func() {
						a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
							return ops.SkillRemovePlan(sk, env, agentTitle)
						}, a.proposeOrSay)
					})
				}
				if canRemoveBroken {
					ui.Button(c.Key("skill-remove-broken"), locales.SkillRemoveBrokenButton()).Shrink(0).OnClick(func() {
						a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
							return ops.SkillRemoveBrokenPlan(sk, env)
						}, a.proposeOrSay)
					})
				}
			})
		}
	})
}

// loadSkill reads the folder of sk in the background, once, when it is not
// the one read last.
func (a *App) loadSkill(sk inventory.Skill) {
	if a.LoadSkill == nil || sk.Broken || a.skillFolder.dir == sk.Dir {
		return
	}
	dir := sk.Dir
	a.skillFolder = skillFolder{dir: dir}
	a.LoadSkill(dir, func(entries []inventory.Entry, text string, err error) {
		if a.skillFolder.dir != dir {
			return
		}
		a.skillFolder = skillFolder{dir: dir, loaded: true, entries: entries, text: text}
		if err != nil {
			a.skillFolder.err = err.Error()
		}
	})
}

// skillFiles are the files in the skill's folder and a preview of its
// SKILL.md: the frontmatter as it is written, the rest as it reads.
func (a *App) skillFiles(c *ui.Context, sk inventory.Skill) {
	t := c.Theme()
	pal := Colors(t)
	note := func(text string, color ui.Color) {
		ui.Text(c, text).FontSize(12).TextColor(color).Padding(4, 16, 10, 16)
	}
	f := a.skillFolder
	switch {
	case sk.Broken:
		note(locales.BrokenLink(tilde(sk.Dir, a.Home)), pal.WarningText)
		return
	case a.LoadSkill == nil:
		return
	case !f.loaded || f.dir != sk.Dir:
		note(locales.Loading(), pal.TextFaint)
		return
	}
	for _, e := range f.entries {
		ui.Row(c).Gap(8).Padding(4, 16).AlignItems(ui.Center).Children(func() {
			icon, name, aside := "file-text", e.Name, locales.SizeKb(int((e.Size+1023)/1024))
			if e.Dir {
				icon, name, aside = "folder-open", e.Name+"/", locales.FilesCount(e.Files)
			}
			ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0).SingleLine()
			ui.Text(c, aside).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		})
	}
	if f.err != "" {
		note(f.err, pal.WarningText)
	}
	if f.text == "" {
		return
	}
	front, body := splitFrontmatter(f.text)
	ui.Column(c).Padding(6, 16, 14, 16).Children(func() {
		ui.Scroll(c).MaxHeight(280).Radius(8).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
			ui.Column(c).Gap(8).Padding(10, 12).Children(func() {
				if front != "" {
					ui.Text(c, front).Font(FontMono).FontSize(11.5).TextColor(pal.MonoText)
				}
				if strings.TrimSpace(body) != "" {
					markdown.View(c, body, &a.markdown)
				}
			})
		})
	})
}

// splitFrontmatter splits SKILL.md into its frontmatter, the "---" lines
// with them, and the rest.
func splitFrontmatter(text string) (front, body string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return "", text
	}
	fm, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", text
	}
	return "---\n" + fm + "\n---", strings.TrimPrefix(after, "\n")
}

// pathChip is a path in mono with a button copying full.
func pathChip(c *ui.Context, shown, full string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c).AlignSelf(ui.Start).Gap(6).Padding(3, 4, 3, 8).Radius(6).AlignItems(ui.Center).MaxWidth(390).
		BorderWidth(1).BorderColor(pal.ControlBorder).Children(func() {
		ui.Icon(c, Icon("folder-open")).Size(13, 13).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, shown).Font(FontMono).FontSize(11).TextColor(pal.MonoText).Shrink(1).MinWidth(0).SingleLine()
		iconButton(c, "copy", locales.CopyPath()).OnClick(func() {
			c.WriteClipboard(full)
			c.Toast(locales.PathCopied())
		})
	})
}

func skillCopyPeers(all []inventory.Skill, sk inventory.Skill) []inventory.Skill {
	if sk.Broken || sk.Synced {
		return nil
	}
	var out []inventory.Skill
	for _, s := range all {
		if !s.Broken && !s.Synced && s.Project == sk.Project && s.Name == sk.Name {
			out = append(out, s)
		}
	}
	return out
}

func skillCopiesDiffer(peers []inventory.Skill) bool {
	if len(peers) < 2 {
		return false
	}
	for _, p := range peers[1:] {
		if p.Sum != peers[0].Sum {
			return true
		}
	}
	return false
}

type skillDiffState struct {
	open      bool
	name      string
	base      inventory.Skill
	peers     []inventory.Skill
	baseDir   string
	otherDirs []string
	other     string
	otherLbl  string
	loading   bool
	diffs     []ai.SkillFileDiff
	err       string
}

func (a *App) openSkillDiff(sk inventory.Skill, peers []inventory.Skill) {
	var others []string
	for _, p := range peers {
		if p.Dir != sk.Dir && !slices.Contains(others, p.Dir) {
			others = append(others, p.Dir)
		}
	}
	if len(others) == 0 {
		return
	}
	d := &skillDiffState{
		open:      true,
		name:      sk.Name,
		base:      sk,
		peers:     slices.Clone(peers),
		baseDir:   sk.Dir,
		otherDirs: others,
		other:     others[0],
		otherLbl:  tilde(others[0], a.Home),
	}
	a.skillDiff = d
	a.loadSkillDiff(d)
}

func (a *App) loadSkillDiff(d *skillDiffState) {
	if a.DiffSkills == nil {
		return
	}
	d.loading = true
	d.err = ""
	d.diffs = nil
	baseDir, otherDir := d.baseDir, d.other
	a.DiffSkills(baseDir, otherDir, func(diffs []ai.SkillFileDiff, err error) {
		if a.skillDiff != d || d.baseDir != baseDir || d.other != otherDir {
			return
		}
		d.loading = false
		d.diffs = diffs
		if err != nil {
			d.err = err.Error()
		}
	})
}

func (a *App) skillDiffDialog(c *ui.Context) {
	d := a.skillDiff
	if d == nil || !d.open {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	dialogBase(c.Key("skill-diff"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(680).MaxHeight(620).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Line)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.SkillDiffTitle(d.name)).FontSize(18).FontWeight(600)
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, tilde(d.baseDir, a.Home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				if len(d.otherDirs) > 1 {
					labels := make([]string, len(d.otherDirs))
					for i, dir := range d.otherDirs {
						labels[i] = tilde(dir, a.Home)
					}
					ui.Select(c.Key("skill-diff-other"), &d.otherLbl, labels).FillWidth().OnChange(func() {
						if i := slices.Index(labels, d.otherLbl); i >= 0 && d.otherDirs[i] != d.other {
							d.other = d.otherDirs[i]
							a.loadSkillDiff(d)
						}
					})
				} else {
					ui.Text(c, tilde(d.other, a.Home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				}
			})
			ui.Scroll(c.Key("skill-diff-scroll")).MaxHeight(420).Children(func() {
				ui.Column(c).Gap(12).Children(func() {
					switch {
					case d.loading:
						ui.Text(c, locales.Loading()).FontSize(12.5).TextColor(t.TextMuted)
					case d.err != "":
						ui.Text(c, d.err).FontSize(12.5).TextColor(pal.WarningText)
					case len(d.diffs) == 0:
						ui.Text(c, locales.SkillDiffIdentical()).FontSize(12.5).TextColor(t.TextMuted)
					default:
						for _, f := range d.diffs {
							ui.Column(c).Gap(6).Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Text(c, f.Path).Font(FontMono).FontSize(12).FontWeight(600).Shrink(1).MinWidth(0).SingleLine()
									switch f.Status {
									case "added":
										aiTag(c, locales.SkillDiffAdded(), pal.SuccessSoft, pal.SuccessText)
									case "removed":
										aiTag(c, locales.SkillDiffRemoved(), pal.DangerSoft, pal.DangerText)
									default:
										aiTag(c, locales.SkillDiffModified(), pal.WarningSoft, pal.WarningText)
									}
								})
								switch {
								case f.Binary:
									ui.Text(c, locales.SkillDiffBinary()).FontSize(12).TextColor(t.TextMuted)
								case f.TooLarge:
									ui.Text(c, locales.SkillDiffTooLarge()).FontSize(12).TextColor(t.TextMuted)
								default:
									diff := (ops.FileEdit{Before: f.Before, After: f.After}).PreviewDiff()
									compareDiffCard(c, "", diff)
								}
							})
						}
					}
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if a.BuildPlan != nil && a.SkillBackups != "" && len(d.peers) > 1 {
					keep, peers, backups := d.base, slices.Clone(d.peers), a.SkillBackups
					ui.PrimaryButton(c.Key("skill-diff-merge"), locales.SkillKeepAndMerge()).OnClick(func() {
						d.open = false
						a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
							return ops.SkillMergeKeepPlan(keep, peers, backups, env, time.Now())
						}, a.proposeOrSay)
					})
				}
				ui.Button(c, locales.SettingsClose()).OnClick(func() { d.open = false })
			})
		})
	})
}

type skillCreateState struct {
	open        bool
	name        string
	description string
	project     string
	err         string
}

func (a *App) openSkillCreate(project string) {
	a.skillCreate = &skillCreateState{
		open:    true,
		project: project,
	}
}

func (a *App) skillCreateDialog(c *ui.Context) {
	st := a.skillCreate
	if st == nil || !st.open {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	submit := func() {
		name := strings.TrimSpace(st.name)
		if name == "" {
			st.err = locales.SkillCreateInvalidName()
			return
		}
		st.err = ""
		desc, project := st.description, st.project
		ids := a.foundIDs()
		st.open = false
		if a.BuildPlan != nil {
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.SkillCreatePlan(name, desc, project, ids, env, agentTitle)
			}, a.proposeOrSay)
		}
	}
	dialogBase(c.Key("skill-create-dialog"), &st.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(480).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Padding(20).Gap(14).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.SkillCreateTitle()).FontSize(18).FontWeight(600)
				ui.Text(c, locales.SkillCreateNote()).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.SkillCreateName()).FontSize(12).FontWeight(500)
				ui.TextInput(c.Key("skill-create-name"), &st.name).
					Placeholder(locales.SkillCreateNamePlaceholder()).
					Height(controlHeight).FillWidth().OnSubmit(submit)
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.SkillCreateDesc()).FontSize(12).FontWeight(500)
				ui.TextInput(c.Key("skill-create-desc"), &st.description).
					Placeholder(locales.SkillCreateDescPlaceholder()).
					Height(controlHeight).FillWidth().OnSubmit(submit)
			})
			if len(a.Projects) > 0 {
				dirs := make([]string, len(a.Projects))
				for i, pr := range a.Projects {
					dirs[i] = pr.Dir
				}
				names := projectDisplayNames(dirs, a.Home)
				scopes := append([]string{""}, dirs...)
				labels := append([]string{locales.SkillCatalogScopeUser()}, names...)
				selected := labels[0]
				if idx := slices.Index(scopes, st.project); idx >= 0 {
					selected = labels[idx]
				}
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, locales.SkillCatalogScope()).FontSize(12).FontWeight(500)
					ui.Select(c.Key("skill-create-scope"), &selected, labels).Height(controlHeight).FillWidth().OnChange(func() {
						if idx := slices.Index(labels, selected); idx >= 0 {
							st.project = scopes[idx]
						}
					})
				})
			}
			if st.err != "" {
				ui.Text(c, st.err).FontSize(12).TextColor(pal.WarningText)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { st.open = false })
				ui.PrimaryButton(c.Key("skill-create-submit"), locales.SkillCatalogPlan()).Height(controlHeight).
					Disabled(strings.TrimSpace(st.name) == "").OnClick(submit)
			})
		})
	})
	if !st.open {
		a.skillCreate = nil
	}
}
