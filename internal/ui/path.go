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
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if a.Snapshot != nil && a.BuildPlan != nil {
				ui.PrimaryButton(c.Key("path-add"), locales.PathAdd()).OnClick(func() {
					a.pathAddInput, a.pathAddErr, a.pathAddOpen = "", "", true
				})
			}
		}})
		a.pathAddDialog(c)
		if a.Snapshot == nil {
			toolNote(c, locales.EnvPathEmpty())
			return
		}
		term, gui := a.Snapshot.TerminalPath, a.Snapshot.GUIPath
		problems := pathProblemCount(term, gui)
		st := a.Snapshot.Startup
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
					if st != nil {
						startupCard(c, st)
					}
				})
				if problems > 0 {
					a.pathBanner(c, term)
				}
				ui.Row(c).Gap(12).Children(func() {
					tabs := []segment{{locales.EnvPathTerminal(), strconv.Itoa(len(term))}, {locales.EnvPathApps(), strconv.Itoa(len(gui))}, {locales.EnvPathVariables(), strconv.Itoa(len(a.Snapshot.Variables))}, {locales.EnvPathDifferences(), ""}}
					if st != nil {
						tabs = append(tabs, segment{locales.StartupLabel(), seconds(st.Took)})
					} else if a.pathTab == startupTab {
						a.pathTab = 0
					}
					segments(c, "path-tabs", &a.pathTab, tabs)
					ui.Box(c).Grow(1)
					if a.pathTab != startupTab {
						ui.SearchField(c.Key("path-filter"), &a.pathFilter).Bind(&a.filterField).Placeholder(locales.EnvPathFilter()).Height(controlHeight).Width(200)
					}
				})
				if a.pathTab == startupTab {
					columns(c, 14, func() { a.startupLines(c, st) }, func() { a.startupSide(c, st) })
					return
				}
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

// startupTab is the index of the PATH page's tab of the shell's start.
const startupTab = 4

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
						full := tilde(src.File, a.Home)
						label := full
						if src.Line > 0 {
							label = filepath.Base(src.File) + ":" + strconv.Itoa(src.Line)
							full = full + ":" + strconv.Itoa(src.Line)
						}
						ui.Text(c, label).Font(FontMono).FontSize(10.5).SingleLine().Tooltip(full)
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
				if a.pathTab == 0 && a.BuildPlan != nil && ops.PathFirstable(entries, i, a.Home) {
					iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(m *ui.Menu) {
						if m.Item(locales.PathFirst()).Chosen() {
							a.putFirst(entry.Dir)
						}
					})
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
			return
		}
		q := strings.ToLower(strings.TrimSpace(a.pathFilter))
		shown := 0
		for _, v := range a.Snapshot.Variables {
			if q != "" && !strings.Contains(strings.ToLower(v.Name), q) &&
				!strings.Contains(strings.ToLower(v.Terminal), q) &&
				!strings.Contains(strings.ToLower(v.GUI), q) {
				continue
			}
			shown++
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
		if shown == 0 {
			cardNote(c, locales.EnvPathNoMatches())
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
		if ops.AppsPathSettable && a.BuildPlan != nil {
			a.appsPath(c, count > 0)
		}
	})
}

// appsPath offers to give apps the terminal's PATH when they get another,
// and to stop when Hearth's launch agent gives it to them.
func (a *App) appsPath(c *ui.Context, differs bool) {
	agent := a.Snapshot.AppsAgent != ""
	if !differs && !agent {
		return
	}
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).Padding(10, 12, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
		caption := ""
		if agent {
			caption = locales.AppsPathGiven()
		}
		ui.Text(c, caption).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
		if differs {
			ui.Button(c.Key("apps-path"), locales.AppsPathSet()).OnClick(func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) { return ops.AppsPathPlan(env) }, a.proposeOrSay)
			})
		}
		if agent {
			ui.Button(c.Key("apps-path-stop"), locales.AppsPathStop()).OnClick(func() { a.Propose(ops.AppsPathStopPlan(a.Home)) })
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
	if a.pathFixSnap != a.Snapshot {
		a.pathFixErr, a.pathFixAllNone = "", false
		a.pathFixSnap = a.Snapshot
	}
	pal := Colors(c.Theme())
	fixable := false
	for i := range term {
		fixable = fixable || a.fixable(term, i)
	}
	ui.Column(c).Gap(6).Padding(10, 10, 10, 12).Radius(9).Background(pal.WarningSoft).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
			ui.Text(c, locales.EnvPathWarnings()).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
			if fixable && !a.pathFixAllNone {
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
	if a.PlanPath == nil {
		return false
	}
	for _, p := range ops.PathProblems(term) {
		if sysenv.SamePath(p.Dir, term[i].Dir) {
			if !ops.PathEntryFixable(term[i], a.Home, p.Repeat) {
				return false
			}
			return !p.Repeat || slices.ContainsFunc(term[:i], func(e inventory.PathEntry) bool { return sysenv.SamePath(e.Dir, p.Dir) })
		}
	}
	return false
}

// fixPath offers the plan that fixes dirs of the terminal's PATH, all its
// problems when dirs is nil, once it is made.
func (a *App) fixPath(dirs []string) {
	a.pathFixErr = ""
	a.pathFixSnap = a.Snapshot
	if dirs == nil {
		a.pathFixAllNone = false
	}
	a.PlanPath(a.Snapshot.TerminalPath, dirs, func(p ops.Plan, err error) {
		switch {
		case errors.Is(err, ops.ErrNoFix):
			a.pathFixErr = locales.PathNoFix()
			if dirs == nil {
				a.pathFixAllNone = true
			}
		case err != nil:
			a.pathFixErr = err.Error()
		default:
			a.Propose(p)
		}
	})
}

// putFirst offers the plan that puts dir first in the terminal's PATH,
// once it is made.
func (a *App) putFirst(dir string) {
	term := a.Snapshot.TerminalPath
	a.BuildPlan(func(env sysenv.Env, terminal ops.Terminal) (ops.Plan, error) {
		p, err := ops.PathFirstPlan(term, dir, env, terminal)
		if errors.Is(err, ops.ErrNoFix) {
			err = errors.New(locales.PathNoFix())
		}
		return p, err
	}, a.proposeOrSay)
}

// pathAddDialog asks for a directory to add to the terminal's PATH and
// proposes ops.PathAddPlan after validating it in the background.
func (a *App) pathAddDialog(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	dialogBase(c.Key("path-add-dialog"), &a.pathAddOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(480).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		submit := func() {
			raw := strings.TrimSpace(a.pathAddInput)
			if raw == "" || a.BuildPlan == nil || a.Snapshot == nil {
				return
			}
			term := a.Snapshot.TerminalPath
			a.pathAddErr = ""
			a.BuildPlan(func(env sysenv.Env, terminal ops.Terminal) (ops.Plan, error) {
				return ops.PathAddPlan(term, raw, env, terminal)
			}, func(p ops.Plan, err error) {
				if err != nil {
					a.pathAddErr = err.Error()
					return
				}
				a.pathAddOpen = false
				a.Propose(p)
			})
		}
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 16, 20).Children(func() {
				ui.Text(c, locales.PathAddTitle()).FontSize(18).FontWeight(600)
				ui.Text(c, locales.PathAddLead()).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
					prev := a.pathAddInput
					in := ui.TextInput(c.Key("path-add-input"), &a.pathAddInput).Grow(1).Placeholder(locales.PathAddPlaceholder()).Label(locales.PathAddTitle()).AutoFocus()
					if a.pathAddInput != prev {
						a.pathAddErr = ""
					}
					if in.Submitted() {
						submit()
					}
					if a.PickPath != nil {
						ui.Button(c.Key("path-add-browse"), locales.PathAddBrowse()).Height(controlHeight).OnClick(func() {
							a.PickPath(locales.PathAddBrowse(), true, func(path string, err error) {
								if err != nil {
									a.pathAddErr = err.Error()
									return
								}
								if path != "" {
									a.pathAddInput = path
									a.pathAddErr = ""
								}
							})
						})
					}
				})
				if a.pathAddErr != "" {
					ui.Text(c, a.pathAddErr).FontSize(12).TextColor(t.Danger).Margin(6, 0, 0, 0)
				}
			})
			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c.Key("path-add-cancel"), locales.Cancel()).Height(controlHeight).OnClick(func() { a.pathAddOpen = false })
				ui.PrimaryButton(c.Key("path-add-next"), locales.Next()).Height(controlHeight).Disabled(strings.TrimSpace(a.pathAddInput) == "").OnClick(submit)
			})
		})
	})
}
