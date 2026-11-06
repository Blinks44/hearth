package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Blinks44/hearth/internal/inventory"
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

// skillKey tells a skill apart: the first path it is found by.
func skillKey(sk inventory.Skill) string { return sk.Paths[0].Path }

// skillsPage lists the skills of the user and of the projects, filtered by
// agent, scope and text, with the chosen one's details beside them.
func (a *App) skillsPage(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		all := a.allSkills()
		if len(all) == 0 {
			a.aiEmpty(c, "puzzle", locales.NoSkills())
			return
		}
		copies := skillCopies(all)
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
		groups := a.skillGroups()
		var shown []inventory.Skill
		for _, g := range groups {
			shown = append(shown, g.skills...)
		}
		chosen := slices.IndexFunc(shown, func(sk inventory.Skill) bool { return skillKey(sk) == a.skillChosen })
		if chosen < 0 && len(shown) > 0 {
			chosen = 0
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				aiIntro(c, "puzzle", locales.SkillsTitle(), locales.SkillsText(), []aiStat{
					{strconv.Itoa(len(all)), locales.StatTotal()},
					{strconv.Itoa(inProjects), locales.StatProjects()},
					{strconv.Itoa(withCopies), locales.StatWithCopies()},
				})
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					a.agentFilter(c, "skill-agent", &a.skillAgent, func(id string) int {
						n := 0
						for _, sk := range all {
							if id == "" || slices.Contains(skillAgents(sk), id) {
								n++
							}
						}
						return n
					})
					scopeSegments(c, "skill-scope", &a.skillScope)
				})
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.skillList(c, groups, copies, shown, chosen)
						if withCopies > 0 {
							a.copiesHint(c, withCopies)
						}
					})
					detailColumn(c).Children(func() {
						if chosen >= 0 {
							a.skillDetail(c, shown[chosen])
						}
					})
				})
			})
		})
	})
}

// skillCopies counts the folders of each skill name in each scope, broken
// links aside.
func skillCopies(skills []inventory.Skill) map[string]int {
	n := map[string]int{}
	for _, sk := range skills {
		if !sk.Broken {
			n[sk.Project+"\x00"+sk.Name]++
		}
	}
	return n
}

// skillGroups are the skills shown, the projects' first in their order,
// then the user's.
func (a *App) skillGroups() []skillGroup {
	q := strings.ToLower(strings.TrimSpace(a.skillQuery))
	keep := func(sk inventory.Skill) bool {
		return inScope(a.skillScope, sk.Project) &&
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
			groups = append(groups, g)
		}
	}
	for _, p := range a.Projects {
		add(p.Dir, p.Skills)
	}
	if a.Snapshot != nil {
		add("", a.Snapshot.Skills)
	}
	return groups
}

// skillList is the search field and the skills by scope.
func (a *App) skillList(c *ui.Context, groups []skillGroup, copies map[string]int, shown []inventory.Skill, chosen int) {
	t := c.Theme()
	pal := Colors(t)
	listFrame(c, func() {
		ui.Row(c).Padding(6, 10).Children(func() {
			ui.SearchField(c.Key("skill-search"), &a.skillQuery).Bind(&a.filterField).Placeholder(locales.SearchByName()).Grow(1)
		})
		if len(groups) == 0 {
			cardNote(c, locales.NothingMatches())
		}
		for _, g := range groups {
			groupTitle(c, scopeName(g.project), a.roots(g))
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
					if sk.Broken {
						aiTag(c, locales.TagBroken(), pal.WarningSoft, pal.WarningText)
					} else if n := copies[sk.Project+"\x00"+sk.Name]; n > 1 {
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
// · ~/.agents/skills", relative to the project for a project's.
func (a *App) roots(g skillGroup) string {
	var out []string
	for _, sk := range g.skills {
		for _, p := range sk.Paths {
			root := tilde(filepath.Dir(p.Path), a.Home)
			if g.project != "" {
				if rel, err := filepath.Rel(g.project, filepath.Dir(p.Path)); err == nil {
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
	})
}

// skillDetail is the chosen skill: what it is, where it is, the agents it
// is available to, its files and SKILL.md, and actions on it.
func (a *App) skillDetail(c *ui.Context, sk inventory.Skill) {
	t := c.Theme()
	pal := Colors(t)
	a.loadSkill(sk)
	listFrame(c, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sk.Name).FontSize(17).FontWeight(500).Shrink(1).MinWidth(0).SingleLine()
				aiTag(c, scopeName(sk.Project), pal.AccentSoft, pal.AccentInk)
			})
			if sk.Description != "" {
				ui.Text(c, sk.Description).FontSize(12.5).TextColor(t.TextMuted)
			}
			pathChip(c, tilde(sk.Dir, a.Home), sk.Dir)
			if sk.Err != "" {
				ui.Text(c, sk.Err).FontSize(12).TextColor(pal.WarningText)
			}
		})
		detailSection(c, locales.AvailableToAgents())
		for _, id := range a.foundIDs() {
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
				if via != nil && via.Link {
					aiTag(c, locales.TagSymlink(), pal.Info.Alpha(0.12), pal.Info)
				}
			})
		}
		detailSection(c, locales.FilesSection())
		a.skillFiles(c, sk)
		if a.Shell != nil && !sk.Broken {
			ui.Row(c).Gap(8).Padding(12, 16, 14, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c.Key("skill-open"), "").OnClick(func() { a.openFile(c, filepath.Join(sk.Dir, "SKILL.md")) }).Children(func() {
					ui.Icon(c, Icon("external-link")).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, locales.OpenInEditor()).SingleLine()
				})
				ui.Button(c.Key("skill-reveal"), "").OnClick(func() { a.Shell.ShowItemInFolder(sk.Dir) }).Children(func() {
					ui.Icon(c, Icon("folder-open")).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, showInFolder()).SingleLine()
				})
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
