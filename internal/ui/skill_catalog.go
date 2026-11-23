package ui

import (
	"bytes"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type skillCatalogCached struct {
	skills []skillcatalog.Skill
	dirs   map[string]string
}

type skillCatalogState struct {
	repo                 int // index into Repositories(), or len(Repositories()) for custom
	custom               *skillcatalog.Repository
	customURL            string
	urlErr               string
	loading              bool
	skills               []skillcatalog.Skill
	dirs                 map[string]string
	err                  string
	chosen, query, agent string
	project              string
	file                 string
	cancel               func()
}

func (s *skillCatalogState) activeRepo() skillcatalog.Repository {
	repos := skillcatalog.Repositories()
	if s.custom != nil && s.repo == len(repos) {
		return *s.custom
	}
	if s.repo >= 0 && s.repo < len(repos) {
		return repos[s.repo]
	}
	return repos[0]
}

// defaultCatalogAgent picks the first installed agent that supports skills,
// falling back to the first sorted directory entry when none are installed.
func (a *App) defaultCatalogAgent(dirs map[string]string) string {
	for _, id := range a.foundIDs() {
		if dirs[id] != "" {
			return id
		}
	}
	var ids []string
	for id := range dirs {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(x, y string) int { return strings.Compare(agentTitle(x), agentTitle(y)) })
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (a *App) openSkillCatalog(repo int) {
	a.openSkillCatalogRepo(repo, nil, false)
}

func (a *App) refreshSkillCatalog() {
	if a.skillCatalog == nil {
		a.openSkillCatalogRepo(0, nil, true)
		return
	}
	repos := skillcatalog.Repositories()
	if a.skillCatalog.custom != nil && a.skillCatalog.repo == len(repos) {
		custom := *a.skillCatalog.custom
		a.openSkillCatalogRepo(len(repos), &custom, true)
		return
	}
	a.openSkillCatalogRepo(a.skillCatalog.repo, nil, true)
}

func (a *App) openSkillCatalogRepo(repo int, custom *skillcatalog.Repository, force bool) {
	if a.skillCatalog != nil && a.skillCatalog.cancel != nil {
		a.skillCatalog.cancel()
	}
	repos := skillcatalog.Repositories()
	var target skillcatalog.Repository
	repoIdx := repo
	if custom != nil {
		target = *custom
		repoIdx = len(repos)
	} else {
		if repoIdx < 0 || repoIdx >= len(repos) {
			repoIdx = 0
		}
		target = repos[repoIdx]
	}
	state := &skillCatalogState{repo: repoIdx, loading: true}
	if a.skillCatalog != nil {
		state.agent = a.skillCatalog.agent
		state.project = a.skillCatalog.project
		state.customURL = a.skillCatalog.customURL
		if custom == nil {
			state.custom = a.skillCatalog.custom
		}
	}
	if custom != nil {
		c := *custom
		state.custom = &c
	}
	a.skillCatalog = state
	a.skillCatalogOpen = true

	key := target.Key()
	if force && a.skillCatalogCache != nil {
		delete(a.skillCatalogCache, key)
	}
	if !force {
		if cached, ok := a.skillCatalogCache[key]; ok {
			state.loading = false
			state.skills = cached.skills
			state.dirs = cached.dirs
			a.syncCatalogAgent(state, cached.dirs)
			return
		}
	}
	if a.LoadCatalog == nil {
		state.loading = false
		return
	}
	state.cancel = a.LoadCatalog(target, force, func(skills []skillcatalog.Skill, dirs map[string]string, err error) {
		if a.skillCatalog != state {
			return
		}
		state.loading = false
		state.skills, state.dirs = skills, dirs
		if err != nil {
			state.err = err.Error()
			return
		}
		if a.skillCatalogCache == nil {
			a.skillCatalogCache = map[string]skillCatalogCached{}
		}
		a.skillCatalogCache[key] = skillCatalogCached{skills: skills, dirs: dirs}
		a.syncCatalogAgent(state, dirs)
	})
}

func (a *App) syncCatalogAgent(s *skillCatalogState, dirs map[string]string) {
	if s.agent != "" && dirs[s.agent] != "" {
		return
	}
	if id := a.defaultCatalogAgent(dirs); id != "" {
		s.agent = id
	}
}

// catalogSkillInstalled reports whether sk is already installed in project
// ("" for user scope).
func (a *App) catalogSkillInstalled(sk skillcatalog.Skill, project string) bool {
	for _, have := range a.allSkills() {
		if have.Broken || have.Project != project {
			continue
		}
		if strings.EqualFold(filepath.Base(have.Dir), sk.Dir) || strings.EqualFold(have.Name, sk.Name) {
			return true
		}
	}
	return false
}

// catalogActiveProject validates s.project against currently registered projects
// without mutating s during render.
func (a *App) catalogActiveProject(s *skillCatalogState) string {
	if s.project == "" {
		return ""
	}
	for _, pr := range a.Projects {
		if pr.Dir == s.project {
			return s.project
		}
	}
	return ""
}

func (a *App) skillCatalogView(c *ui.Context, p page) {
	s := a.skillCatalog
	t, pal := c.Theme(), Colors(c.Theme())
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			ui.Button(c.Key("catalog-refresh"), locales.SkillCatalogRefresh()).Height(controlHeight).Disabled(s.loading).OnClick(func() { a.refreshSkillCatalog() })
			ui.PrimaryButton(c.Key("catalog-installed"), locales.SkillCatalogInstalled()).Height(controlHeight).OnClick(func() { a.skillCatalogOpen = false })
		}})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(16).Padding(22, 28, 24, 28).Children(func() {
				aiIntro(c, "puzzle", locales.SkillCatalogTitle(), locales.SkillCatalogText(), nil)
				repos := skillcatalog.Repositories()
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					var segs []segment
					for i, r := range repos {
						count := ""
						if i == s.repo && !s.loading && s.err == "" {
							count = strconv.Itoa(len(s.skills))
						}
						segs = append(segs, segment{r.Name, count})
					}
					if s.custom != nil {
						count := ""
						if s.repo == len(repos) && !s.loading && s.err == "" {
							count = strconv.Itoa(len(s.skills))
						}
						segs = append(segs, segment{s.custom.Slug, count})
					}
					tab := s.repo
					segments(c, "catalog-repository", &tab, segs).OnChange(func() {
						if tab == s.repo {
							return
						}
						if s.custom != nil && tab == len(repos) {
							custom := *s.custom
							a.openSkillCatalogRepo(len(repos), &custom, false)
							return
						}
						a.openSkillCatalog(tab)
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("catalog-search"), &s.query).Placeholder(locales.SearchByName()).Height(controlHeight).Width(272)
				})
				loadCustom := func() {
					repo, err := skillcatalog.ParseGitHubURL(s.customURL)
					if err != nil {
						s.urlErr = err.Error()
						return
					}
					s.urlErr = ""
					a.openSkillCatalogRepo(len(repos), &repo, false)
				}
				ui.Column(c).Gap(4).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c.Key("catalog-url"), &s.customURL).
							Placeholder(locales.SkillCatalogRepoPlaceholder()).
							Height(controlHeight).Grow(1).MinWidth(0).
							OnSubmit(loadCustom)
						ui.Button(c.Key("catalog-load"), locales.SkillCatalogLoad()).
							Height(controlHeight).Disabled(s.loading).OnClick(loadCustom)
					})
					if s.urlErr != "" {
						ui.Text(c, s.urlErr).FontSize(12).TextColor(pal.WarningText)
					}
				})
				if s.loading {
					cardNote(c, locales.Loading())
					return
				}
				if s.err != "" {
					listFrame(c, func() { cardNote(c, locales.SkillCatalogError()); cardNote(c, s.err) })
					return
				}
				var shown []skillcatalog.Skill
				q := strings.ToLower(strings.TrimSpace(s.query))
				for _, sk := range s.skills {
					if q == "" || strings.Contains(strings.ToLower(sk.Name+" "+sk.Description), q) {
						shown = append(shown, sk)
					}
				}
				chosen := slices.IndexFunc(shown, func(sk skillcatalog.Skill) bool { return sk.Dir == s.chosen })
				if chosen < 0 && len(shown) > 0 {
					chosen = 0
				}
				active := s.activeRepo()
				scopeProject := a.catalogActiveProject(s)
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						listFrame(c, func() {
							groupTitle(c, active.Slug, "GitHub")
							if len(shown) == 0 {
								cardNote(c, locales.NothingMatches())
							}
							for i, sk := range shown {
								installed := a.catalogSkillInstalled(sk, scopeProject)
								row := ui.ButtonBase(c.Key("catalog-skill:"+sk.Dir)).Label(sk.Name).Gap(10).Padding(9, 14, 9, 12).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
								if i == chosen {
									row.Background(pal.ActiveRow)
								} else if row.Hovered() {
									row.Background(pal.Subtle)
								}
								row.OnClick(func() { s.chosen = sk.Dir; s.file = "" }).Children(func() {
									ui.Box(c).Size(27, 27).Shrink(0).Radius(7).Center().Background(pal.Subtle).Children(func() { ui.Icon(c, Icon("book-open")).Size(15, 15).TextColor(t.TextMuted) })
									ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
										ui.Text(c, sk.Name).FontSize(13.5).SingleLine()
										ui.Text(c, sk.Description).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
									})
									if installed {
										aiTag(c, locales.Installed(), pal.SuccessSoft, pal.SuccessText)
									}
								})
							}
						})
						ui.Row(c).Gap(10).Padding(12, 14).Radius(12).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
							ui.Icon(c, Icon("sparkles")).Size(16, 16).Shrink(0).TextColor(Accent)
							ui.Text(c, locales.SkillCatalogHint()).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
						})
					})
					detailColumn(c).Children(func() {
						if chosen >= 0 {
							a.catalogSkillDetail(c, s, active, shown[chosen])
						}
					})
				})
			})
		})
	})
}

func (a *App) catalogSkillDetail(c *ui.Context, s *skillCatalogState, active skillcatalog.Repository, sk skillcatalog.Skill) {
	t, pal := c.Theme(), Colors(c.Theme())
	project := a.catalogActiveProject(s)
	installed := a.catalogSkillInstalled(sk, project)
	listFrame(c, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sk.Name).FontSize(17).FontWeight(500).Shrink(1).MinWidth(0).SingleLine()
				aiTag(c, active.Name, pal.AccentSoft, pal.AccentInk)
				if installed {
					aiTag(c, locales.Installed(), pal.SuccessSoft, pal.SuccessText)
				}
			})
			ui.Text(c, sk.Description).FontSize(12.5).TextColor(t.TextMuted).MaxLines(4)
			pathChip(c, sk.Repository+" · "+sk.Dir, sk.URL())
		})
		detailSection(c, locales.SkillCatalogFor())
		// Optional scope selector when the user has registered projects.
		var scopes, scopeLabels []string
		scopeLabel := locales.SkillCatalogScopeUser()
		if len(a.Projects) > 0 {
			scopes = make([]string, 0, len(a.Projects)+1)
			scopeLabels = make([]string, 0, len(a.Projects)+1)
			scopes = append(scopes, "")
			scopeLabels = append(scopeLabels, locales.SkillCatalogScopeUser())
			for _, pr := range a.Projects {
				scopes = append(scopes, pr.Dir)
				scopeLabels = append(scopeLabels, filepath.Base(pr.Dir))
			}
			if si := slices.Index(scopes, project); si >= 0 {
				scopeLabel = scopeLabels[si]
			}
		}

		dirs := s.dirs
		if project != "" {
			dirs = ai.ProjectSkillDirectories(project)
		}
		var ids []string
		for id := range dirs {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(x, y string) int { return strings.Compare(agentTitle(x), agentTitle(y)) })
		value := slices.Index(ids, s.agent)
		if value < 0 && len(ids) > 0 {
			if def := a.defaultCatalogAgent(dirs); def != "" {
				value = slices.Index(ids, def)
			}
			if value < 0 {
				value = 0
			}
		}
		agent := ""
		if value >= 0 {
			agent = ids[value]
		}
		var allAgents []string
		for _, id := range a.foundIDs() {
			if dirs[id] != "" {
				allAgents = append(allAgents, id)
			}
		}
		ui.Column(c).Gap(6).Padding(6, 16, 8, 16).Children(func() {
			if len(ids) == 0 {
				cardNote(c, locales.SkillCatalogNoAgent())
				return
			}
			labels := make([]string, len(ids))
			for i, id := range ids {
				labels[i] = agentTitle(id)
			}
			agentLabel := agentTitle(agent)
			if len(scopes) > 0 {
				ui.Row(c).Gap(8).Children(func() {
					ui.Select(c.Key("catalog-agent"), &agentLabel, labels).Label(locales.SkillCatalogFor()).Grow(1).MinWidth(0).OnChange(func() {
						if i := slices.Index(labels, agentLabel); i >= 0 {
							s.agent = ids[i]
						}
					})
					ui.Select(c.Key("catalog-scope"), &scopeLabel, scopeLabels).Label(locales.SkillCatalogScope()).Grow(1).MinWidth(0).OnChange(func() {
						if i := slices.Index(scopeLabels, scopeLabel); i >= 0 {
							s.project = scopes[i]
						}
					})
				})
			} else {
				ui.Select(c.Key("catalog-agent"), &agentLabel, labels).Label(locales.SkillCatalogFor()).FillWidth().OnChange(func() {
					if i := slices.Index(labels, agentLabel); i >= 0 {
						s.agent = ids[i]
					}
				})
			}
			ui.Text(c, tilde(filepath.Join(dirs[agent], sk.Dir), a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		})
		detailSection(c, locales.FilesSection())
		ui.Scroll(c.Key("catalog-files:" + sk.Dir)).MaxHeight(84).Children(func() {
			ui.Column(c).Children(func() {
				for _, f := range sk.Files {
					ui.ButtonBase(c.Key("catalog-file:"+f.Path)).Label(f.Path).Gap(8).Padding(4, 16).OnClick(func() { s.file = f.Path }).Children(func() {
						ui.Icon(c, Icon("file-text")).Size(14, 14).TextColor(t.TextMuted)
						clipped(c, f.Path).Font(FontMono).FontSize(11.5).Grow(1)
					})
				}
			})
		})
		ui.Column(c).Padding(6, 16, 12, 16).Children(func() {
			ui.Scroll(c.Key("catalog-preview:" + sk.Dir)).Height(160).Radius(8).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
				// Plain text keeps repository instructions readable without fetching images.
				ui.Text(c, catalogFileText(sk, s.file)).Font(FontMono).FontSize(11.5).TextColor(pal.MonoText).Padding(10, 12)
			})
		})
		ui.Row(c).Gap(8).Wrap().Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.PrimaryButton(c.Key("catalog-plan"), locales.SkillCatalogPlan()).Height(controlHeight).Disabled(agent == "" || a.BuildPlan == nil).OnClick(func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.SkillInstallPlanFor(sk, agent, project, env, agentTitle)
				}, a.proposeOrSay)
			})
			if len(allAgents) > 1 && a.BuildPlan != nil {
				targets := slices.Clone(allAgents)
				ui.Button(c.Key("catalog-plan-all"), locales.SkillConnectAll()).Height(controlHeight).OnClick(func() {
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.SkillInstallAllPlanFor(sk, targets, project, env, agentTitle)
					}, a.proposeOrSay)
				})
			}
			ui.Button(c.Key("catalog-github"), locales.SkillCatalogGitHub()).Height(controlHeight).OnClick(func() { c.OpenURL(sk.URL()) })
		})
	})
}

// catalogFileText previews text files locally; binary files remain downloadable
// as part of the plan but are not passed to the text renderer.
func catalogFileText(sk skillcatalog.Skill, name string) string {
	if name == "" {
		name = "SKILL.md"
	}
	for _, f := range sk.Files {
		if f.Path == name {
			if !utf8.Valid(f.Data) || bytes.IndexByte(f.Data, 0) >= 0 {
				return locales.SkillCatalogBinary()
			}
			const limit = 64 * 1024
			if len(f.Data) > limit {
				text := string(f.Data[:limit])
				for !utf8.ValidString(text) {
					text = text[:len(text)-1]
				}
				return text + "\n" + locales.SkillCatalogTruncated()
			}
			return string(f.Data)
		}
	}
	return sk.Text()
}
