package ui

import (
	"bytes"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type skillCatalogState struct {
	repo                 int
	tab                  int // the repository chosen in the toolbar; repo is the one loaded
	loading              bool
	skills               []skillcatalog.Skill
	dirs                 map[string]string
	err                  string
	chosen, query, agent string
	agentLabel           string
	file                 string
	cancel               func()
}

func (a *App) openSkillCatalog(repo int) {
	if a.skillCatalog != nil && a.skillCatalog.cancel != nil {
		a.skillCatalog.cancel()
	}
	state := &skillCatalogState{repo: repo, tab: repo, loading: true, agent: "claude"}
	if a.skillCatalog != nil {
		state.agent = a.skillCatalog.agent
		state.agentLabel = a.skillCatalog.agentLabel
	}
	a.skillCatalog = state
	a.skillCatalogOpen = true
	if a.LoadCatalog == nil {
		state.loading = false
		return
	}
	state.cancel = a.LoadCatalog(skillcatalog.Repositories()[repo], func(skills []skillcatalog.Skill, dirs map[string]string, err error) {
		if a.skillCatalog != state {
			return
		}
		state.loading = false
		state.skills, state.dirs = skills, dirs
		if err != nil {
			state.err = err.Error()
		}
	})
}

func (a *App) skillCatalogView(c *ui.Context, p page) {
	s := a.skillCatalog
	t, pal := c.Theme(), Colors(c.Theme())
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			ui.Button(c.Key("catalog-refresh"), locales.SkillCatalogRefresh()).Disabled(s.loading).OnClick(func() { a.openSkillCatalog(s.repo) })
			ui.PrimaryButton(c.Key("catalog-installed"), locales.SkillCatalogInstalled()).OnClick(func() { a.skillCatalogOpen = false })
		}})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				aiIntro(c, "puzzle", locales.SkillCatalogTitle(), locales.SkillCatalogText(), nil)
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					var segs []segment
					for i, r := range skillcatalog.Repositories() {
						count := ""
						if i == s.repo && !s.loading && s.err == "" {
							count = strconv.Itoa(len(s.skills))
						}
						segs = append(segs, segment{r.Name, count})
					}
					segments(c, "catalog-repository", &s.tab, segs)
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("catalog-search"), &s.query).Placeholder(locales.SearchByName()).Width(240)
				})
				if s.tab != s.repo {
					a.openSkillCatalog(s.tab)
					return
				}
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
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						listFrame(c, func() {
							groupTitle(c, skillcatalog.Repositories()[s.repo].Slug, "GitHub")
							if len(shown) == 0 {
								cardNote(c, locales.NothingMatches())
							}
							for i, sk := range shown {
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
							a.catalogSkillDetail(c, s, shown[chosen])
						}
					})
				})
			})
		})
	})
}

func (a *App) catalogSkillDetail(c *ui.Context, s *skillCatalogState, sk skillcatalog.Skill) {
	t, pal := c.Theme(), Colors(c.Theme())
	listFrame(c, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, sk.Name).FontSize(17).FontWeight(500).SingleLine()
				aiTag(c, skillcatalog.Repositories()[s.repo].Name, pal.AccentSoft, pal.AccentInk)
			})
			ui.Text(c, sk.Description).FontSize(12.5).TextColor(t.TextMuted).MaxLines(4)
			pathChip(c, sk.Repository+" · "+sk.Dir, sk.URL())
		})
		detailSection(c, locales.SkillCatalogFor())
		var ids []string
		for id := range s.dirs {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(a, b string) int { return strings.Compare(agentTitle(a), agentTitle(b)) })
		value := slices.Index(ids, s.agent)
		if value < 0 && len(ids) > 0 {
			value = 0
		}
		agent := ""
		if value >= 0 {
			agent = ids[value]
		}
		ui.Column(c).Gap(6).Padding(6, 16, 10, 16).Children(func() {
			if len(ids) == 0 {
				cardNote(c, locales.SkillCatalogNoAgent())
				return
			}
			labels := make([]string, len(ids))
			for i, id := range ids {
				labels[i] = agentTitle(id)
			}
			if s.agentLabel == "" {
				s.agentLabel = agentTitle(agent)
			}
			ui.Select(c.Key("catalog-agent"), &s.agentLabel, labels).Label(locales.SkillCatalogFor()).FillWidth().OnChange(func() {
				if i := slices.Index(labels, s.agentLabel); i >= 0 {
					s.agent = ids[i]
				}
			})
			ui.Text(c, tilde(filepath.Join(s.dirs[agent], sk.Dir), a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
		})
		detailSection(c, locales.FilesSection())
		ui.Scroll(c.Key("catalog-files:" + sk.Dir)).MaxHeight(100).Children(func() {
			ui.Column(c).Children(func() {
				for _, f := range sk.Files {
					ui.ButtonBase(c.Key("catalog-file:"+f.Path)).Label(f.Path).Gap(8).Padding(4, 16).OnClick(func() { s.file = f.Path }).Children(func() {
						ui.Icon(c, Icon("file-text")).Size(14, 14).TextColor(t.TextMuted)
						clipped(c, f.Path).Font(FontMono).FontSize(11.5).Grow(1)
					})
				}
			})
		})
		ui.Column(c).Padding(6, 16, 14, 16).Children(func() {
			ui.Scroll(c.Key("catalog-preview:" + sk.Dir)).Height(210).Radius(8).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
				// Plain text keeps repository instructions readable without fetching images.
				ui.Text(c, catalogFileText(sk, s.file)).Font(FontMono).FontSize(11.5).TextColor(pal.MonoText).Padding(10, 12)
			})
		})
		ui.Row(c).Gap(8).Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.PrimaryButton(c.Key("catalog-plan"), locales.SkillCatalogPlan()).Disabled(agent == "" || a.BuildPlan == nil).OnClick(func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.SkillInstallPlan(sk, agent, env, agentTitle)
				}, a.proposeOrSay)
			})
			ui.Button(c.Key("catalog-github"), locales.SkillCatalogGitHub()).OnClick(func() { c.OpenURL(sk.URL()) })
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
