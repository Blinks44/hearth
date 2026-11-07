package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) paths(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		if a.Snapshot == nil {
			toolNote(c, locales.EnvPathEmpty())
			return
		}
		term, gui := a.Snapshot.TerminalPath, a.Snapshot.GUIPath
		problems := pathProblemCount(term, gui)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pal := Colors(c.Theme())
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("terminal")).Size(24, 24) })
					ui.Column(c).Grow(1).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.EnvPathDescription()).FontSize(12).TextColor(c.Theme().TextMuted)
					})
				}, func() {
					stateCard(c, c.Theme().Success, locales.EnvPathTerminal(), strconv.Itoa(len(term)), locales.EnvPathEntries())
					stateCard(c, pal.TextFaint, locales.EnvPathApps(), strconv.Itoa(len(gui)), locales.EnvPathEntries())
					stateCard(c, c.Theme().Warning, locales.EnvPathProblems(), strconv.Itoa(problems), "")
				})
				if problems > 0 {
					a.pathBanner(c, term)
				}
				ui.Row(c).Gap(12).Children(func() {
					segments(c, "path-tabs", &a.pathTab, []segment{{locales.EnvPathTerminal(), strconv.Itoa(len(term))}, {locales.EnvPathApps(), strconv.Itoa(len(gui))}, {locales.EnvPathVariables(), strconv.Itoa(len(a.Snapshot.Variables))}, {locales.EnvPathDifferences(), ""}})
					ui.Box(c).Grow(1)
					ui.SearchField(c.Key("path-filter"), &a.pathFilter).Bind(&a.filterField).Placeholder(locales.EnvPathFilter()).Width(200)
				})
				columns(c, 14, func() {
					switch a.pathTab {
					case 0:
						a.pathTable(c, term, gui)
					case 1:
						a.pathTable(c, gui, term)
					case 2:
						a.pathVariables(c)
					case 3:
						a.pathDifferences(c, term, gui)
					}
					toolNote(c, locales.EnvPathReadOnly())
					if a.pathTab != 3 {
						a.pathDifferences(c, term, gui)
					}
				}, func() {
					if a.pathTab != 2 {
						a.pathVariables(c)
					}
					listCard(c, locales.EnvPathOrigins(), "", func() {
						cardNote(c, locales.EnvPathOriginNote())
						seen := map[string]bool{}
						for _, entry := range term {
							for _, src := range entry.Sources {
								if seen[src.File] {
									continue
								}
								seen[src.File] = true
								listRow(c, tilde(src.File, a.Home), false, "", func() {
									if src.Line > 0 && a.Shell != nil {
										iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, src.File) })
									}
								})
							}
						}
					})
				})
			})
		})
	})
}

func hasPath(entries []inventory.PathEntry, dir string) bool {
	for _, entry := range entries {
		if sysenv.SamePath(entry.Dir, dir) {
			return true
		}
	}
	return false
}

func (a *App) pathTable(c *ui.Context, entries, other []inventory.PathEntry) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.EnvPathOrder()).Width(60).SingleLine().FontSize(11.5)
			ui.Text(c, locales.EnvPathSource()).Width(120).FontSize(11.5)
			ui.Text(c, locales.EnvPathDirectory()).Grow(1).FontSize(11.5)
		})
		shown := 0
		for i, entry := range entries {
			if !strings.Contains(strings.ToLower(entry.Dir), strings.ToLower(a.pathFilter)) {
				continue
			}
			shown++
			ui.Row(c.Key("path-"+strconv.Itoa(i))).Gap(12).Padding(12, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "#"+strconv.Itoa(i+1)).Width(60).Font(FontMono).FontSize(11.5)
				ui.Column(c).Width(120).Gap(3).Children(func() {
					if len(entry.Sources) == 0 {
						ui.Text(c, locales.EnvPathUnknown()).FontSize(10.5).TextColor(pal.TextFaint)
					}
					for _, src := range entry.Sources {
						label := tilde(src.File, a.Home)
						if src.Line > 0 {
							label = filepath.Base(src.File) + ":" + strconv.Itoa(src.Line)
						}
						ui.Text(c, label).Font(FontMono).FontSize(10.5).SingleLine()
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					dir := tilde(entry.Dir, a.Home)
					if dir == "" {
						dir = "."
					}
					clipped(c, dir).Font(FontMono).FontSize(11.5)
					ui.Row(c).Gap(5).Wrap().Children(func() {
						for j := range i {
							if sysenv.SamePath(entries[j].Dir, entry.Dir) {
								if ops.SystemRepeat(entry) {
									chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.EnvPathDuplicate()+strconv.Itoa(j+1))
								} else {
									chip(c, c.Theme().Warning, pal.WarningSoft, pal.WarningText, locales.EnvPathDuplicate()+strconv.Itoa(j+1))
								}
								break
							}
						}
						switch {
						case entry.Missing && issues.OSDir(entry.Dir):
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.EnvPathMissing())
						case entry.Missing:
							chip(c, c.Theme().Warning, pal.WarningSoft, pal.WarningText, locales.EnvPathMissing())
						}
						if !filepath.IsAbs(entry.Dir) {
							chip(c, c.Theme().Warning, pal.WarningSoft, pal.WarningText, locales.EnvPathRelative())
						}
						if !hasPath(other, entry.Dir) {
							label := locales.EnvPathTerminalOnly()
							if a.pathTab == 1 {
								label = locales.EnvPathAppsOnly()
							}
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, label)
						}
					})
				})
				if a.pathTab == 0 && a.fixable(entries, i) {
					ui.Button(c.Key("path-fix:"+strconv.Itoa(i)), locales.PathRemove()).Label(locales.PathRemoveNamed(tilde(entry.Dir, a.Home))).OnClick(func() { a.fixPath([]string{entry.Dir}) })
				}
				iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(entry.Dir); c.Toast(locales.PathCopied()) })
			})
		}
		if shown == 0 {
			cardNote(c, locales.EnvPathNoMatches())
		}
	})
}

func (a *App) pathVariables(c *ui.Context) {
	listCard(c, locales.EnvPathToolVariables(), "", func() {
		if len(a.Snapshot.Variables) == 0 {
			cardNote(c, locales.EnvPathNoVariables())
		}
		for _, v := range a.Snapshot.Variables {
			listRow(c, v.Name, true, "", func() {})
			t, g := tilde(v.Terminal, a.Home), tilde(v.GUI, a.Home)
			if t == "" {
				t = locales.EnvPathUnset()
			}
			if g == "" {
				g = locales.EnvPathUnset()
			}
			cardNote(c, locales.EnvPathTerminal()+": "+t)
			if v.Terminal != v.GUI {
				cardNote(c, locales.EnvPathApps()+": "+g)
			}
		}
	})
}

func (a *App) pathDifferences(c *ui.Context, term, gui []inventory.PathEntry) {
	listCard(c, locales.EnvPathDifferences(), "", func() {
		count := 0
		for _, side := range []struct {
			entries, other []inventory.PathEntry
			label          string
		}{{term, gui, locales.EnvPathTerminalOnly()}, {gui, term, locales.EnvPathAppsOnly()}} {
			var dirs []string
			for i, e := range side.entries {
				if hasPath(side.other, e.Dir) || hasPath(side.entries[:i], e.Dir) {
					continue
				}
				dirs = append(dirs, tilde(e.Dir, a.Home))
			}
			if len(dirs) > 0 {
				count += len(dirs)
				listRow(c, side.label, false, strings.Join(dirs, " · "), func() {})
			}
		}
		if count == 0 {
			cardNote(c, locales.EnvPathNoDifference())
		}
	})
}

// pathProblemCount counts the directories with a problem in the terminal's
// or the apps' PATH, each once, though both environments usually share it.
func pathProblemCount(term, gui []inventory.PathEntry) int {
	var dirs []inventory.PathEntry
	for _, entries := range [][]inventory.PathEntry{term, gui} {
		for i, e := range entries {
			if (e.Missing && !issues.OSDir(e.Dir) || !filepath.IsAbs(e.Dir) || hasPath(entries[:i], e.Dir) && !ops.SystemRepeat(e)) && !hasPath(dirs, e.Dir) {
				dirs = append(dirs, e)
			}
		}
	}
	return len(dirs)
}

// pathBanner says the PATH has problems, with a button that fixes those
// Hearth can in the terminal's, and why it could not when it could not.
func (a *App) pathBanner(c *ui.Context, term []inventory.PathEntry) {
	pal := Colors(c.Theme())
	fixable := false
	for i := range term {
		fixable = fixable || a.fixable(term, i)
	}
	ui.Column(c).Gap(6).Padding(10, 10, 10, 12).Radius(9).Background(pal.WarningSoft).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
			ui.Text(c, locales.EnvPathWarnings()).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
			if fixable {
				// Dark, as in the design: the page's one accent stays for
				// the header.
				ui.Button(c.Key("path-fix"), locales.PathFix()).Background(c.Theme().Text).TextColor(c.Theme().Background).
					BorderColor(c.Theme().Text).OnClick(func() { a.fixPath(nil) })
			}
		})
		if a.pathFixErr != "" {
			ui.Text(c, a.pathFixErr).FontSize(11.5).TextColor(pal.WarningText).Padding(0, 0, 0, 26)
		}
	})
}

// fixable reports that the i-th entry of the terminal's PATH is a problem
// Hearth may fix where it is declared: the repeat, not the first, or a
// directory that is not there.
func (a *App) fixable(term []inventory.PathEntry, i int) bool {
	if a.PlanPath == nil || !ops.PathFixable(term[i], a.Home) {
		return false
	}
	for _, p := range ops.PathProblems(term) {
		if sysenv.SamePath(p.Dir, term[i].Dir) {
			return !p.Repeat || slices.ContainsFunc(term[:i], func(e inventory.PathEntry) bool { return sysenv.SamePath(e.Dir, p.Dir) })
		}
	}
	return false
}

// fixPath offers the plan that fixes dirs of the terminal's PATH, all its
// problems when dirs is nil, once it is made.
func (a *App) fixPath(dirs []string) {
	a.pathFixErr = ""
	a.PlanPath(a.Snapshot.TerminalPath, dirs, func(p ops.Plan, err error) {
		switch {
		case errors.Is(err, ops.ErrNoFix):
			a.pathFixErr = locales.PathNoFix()
		case err != nil:
			a.pathFixErr = err.Error()
		default:
			a.Propose(p)
		}
	})
}
