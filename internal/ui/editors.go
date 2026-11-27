package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// editorsPage is the page of code editors and IDEs.
const editorsPage = "/tools/editors"

// editorRow is a row of the editors table: the command, the extensions,
// their old versions or the caches. A command row says whether it is on
// the PATH; the others the space their path takes.
type editorRow struct {
	name, about string
	command     bool
	onPath      bool
	path        string
	paths       []string
	installed   []string
	addPath     *inventory.Editor
}

// editorsState is what the editors detector found; empty before a scan.
func (a *App) editorsState() inventory.EditorsState {
	if st, _ := a.Snapshot.Tool("editors"); st.Editors != nil {
		return *st.Editors
	}
	return inventory.EditorsState{}
}

// editorSize is the space the last count found at the row's paths.
func (a *App) editorSize(r editorRow) (int64, bool) {
	if r.command {
		return 0, false
	}
	paths := r.paths
	if r.path != "" {
		paths = append(paths, r.path)
	}
	var total int64
	for _, p := range paths {
		size, ok := a.pathSize(p)
		if !ok {
			return 0, false
		}
		total += size
	}
	return total, len(paths) > 0
}

// editorTitle is a group's title: the editor, its version, and Toolbox
// when it installed it.
func editorTitle(e inventory.Editor) string {
	parts := []string{strings.TrimSpace(e.Name + " " + e.Version)}
	if e.Toolbox {
		parts = append(parts, "Toolbox")
	}
	if e.App == "" {
		parts = append(parts, locales.EditorsNoApp())
	}
	return strings.Join(parts, " · ")
}

// editorRows are an editor's rows.
func (a *App) editorRows(e inventory.Editor, all []inventory.Editor) []editorRow {
	var rows []editorRow
	cmd := editorRow{name: locales.EditorsCommand(e.Command), command: true, onPath: e.CommandPath != "", path: e.CommandPath}
	if e.CommandPath == "" && e.CommandSource != "" {
		ed := e
		cmd.addPath = &ed
	}
	switch {
	case e.CommandPath != "" && strings.Contains(e.CommandPath, "Toolbox"):
		cmd.about = locales.EditorsToolboxScript()
	case e.CommandPath != "" && e.App != "" && strings.HasPrefix(e.CommandReal, e.App+string(filepath.Separator)) && e.CommandReal != e.CommandPath:
		target := tilde(e.CommandReal, a.Home)
		if strings.HasSuffix(e.App, ".app") {
			target = filepath.Base(e.App)
		}
		cmd.about = tilde(e.CommandPath, a.Home) + " → " + target
	case e.CommandPath != "":
		cmd.about = tilde(e.CommandPath, a.Home)
	case e.Toolbox:
		cmd.about = locales.EditorsNotOnPath() + " · " + locales.EditorsFromToolbox()
	default:
		cmd.about = locales.EditorsNotOnPath() + " · " + locales.EditorsFromApp(e.Name)
	}
	rows = append(rows, cmd)
	if e.Extensions != "" {
		about := []string{tilde(e.Extensions, a.Home)}
		if e.Config != "" {
			about = []string{locales.EditorsConfig(e.Config)}
		}
		// Extensions shared with VS Code, the one others are made of.
		for _, o := range all {
			if o.ID != "vscode" || e.ID == "vscode" || e.Config != "" {
				continue
			}
			shared := 0
			for _, id := range e.Installed {
				if slices.Contains(o.Installed, id) {
					shared++
				}
			}
			if shared > 0 {
				about = append(about, locales.EditorsAlsoIn(shared, o.Name))
			}
		}
		name := locales.EditorsExtensions(len(e.Installed))
		if e.Config != "" {
			name = locales.EditorsPlugins(len(e.Installed))
		}
		rows = append(rows, editorRow{name: name, about: strings.Join(about, " · "), path: e.Extensions, installed: e.Installed})
	}
	if len(e.Superseded) > 0 {
		var names []string
		for _, p := range e.Superseded {
			names = append(names, filepath.Base(p))
		}
		preview := strings.Join(names, ", ")
		if len(names) > 2 {
			preview = strings.Join(names[:2], ", ") + " (+" + strconv.Itoa(len(names)-2) + ")"
		}
		rows = append(rows, editorRow{
			name:  locales.EditorsOld(),
			about: locales.EditorsOldAbout() + " · " + locales.EditorsFolders(len(e.Superseded)) + " · " + preview,
			paths: e.Superseded,
		})
	}
	if e.Caches != "" {
		rows = append(rows, editorRow{name: locales.EditorsIndexCache(), about: tilde(filepath.Dir(e.Caches), a.Home) + " · " + locales.EditorsRestorable(), path: e.Caches})
	}
	return rows
}

func (a *App) editorsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		eds := a.editorsState().Editors
		var commands []string
		extensions := 0
		var size int64
		counted := true
		for _, e := range eds {
			if e.CommandPath != "" {
				commands = append(commands, e.Command)
			}
			extensions += len(e.Installed)
			for _, r := range a.editorRows(e, eds) {
				if r.command {
					continue
				}
				s, ok := a.editorSize(r)
				size += s
				counted = counted && ok
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("code-xml")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.EditorsLead()).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.EditorsFound(), strconv.Itoa(len(eds)), locales.EditorsFoundCaption())
					cmdDot := pal.TextFaint
					if len(commands) > 0 {
						cmdDot = t.Success
					}
					stateCard(c, cmdDot, locales.EditorsCommands(), locales.EditorsOf(len(commands), len(eds)), strings.Join(commands, ", "))
					total := "…" // until the count reaches every folder
					if counted {
						total = formatBytes(size)
					}
					stateCard(c, pal.TextFaint, locales.EditorsExtensionsCard(), total, locales.EditorsExtensions(extensions))
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "editors-tabs", &a.editorsTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(eds))},
						{locales.EditorsExtensionsCard(), strconv.Itoa(extensions)},
						{locales.EditorsCommands(), strconv.Itoa(len(eds))},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("editors-filter"), &a.editorsFilter).Bind(&a.filterField).Placeholder(locales.EditorsFilterPlaceholder()).Height(controlHeight).Width(264)
				})
				columns(c, 12, func() {
					switch {
					case a.Snapshot == nil:
						if a.Scanning {
							toolNote(c, locales.Checking())
						} else {
							toolNote(c, locales.NotChecked())
						}
					case len(eds) == 0:
						toolNote(c, locales.EditorsNone())
					default:
						a.editorsTable(c, eds)
					}
				}, func() {
					a.editorsHints(c, eds)
					listCard(c, locales.EditorsUntouched(), "", func() {
						listRow(c, locales.EditorsSettings(), false, locales.EditorsSettingsCaption(), func() {})
						listRow(c, locales.EditorsOwnExtensions(), false, locales.EditorsOwnExtensionsCaption(), func() {})
						listRow(c, locales.EditorsAccounts(), false, locales.EditorsAccountsCaption(), func() {})
					})
				})
			})
		})
	})
}

// editorsTable is an editor's group each, its rows of the chosen tab,
// filtered by the editor's name, command, or installed extension IDs.
func (a *App) editorsTable(c *ui.Context, eds []inventory.Editor) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(strings.TrimSpace(a.editorsFilter))
	cands := a.candidates(c)
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for _, e := range eds {
			editorMatch := filter == "" || strings.Contains(strings.ToLower(e.Name+" "+e.Command), filter)
			var extMatches []string
			if filter != "" {
				for _, id := range e.Installed {
					if strings.Contains(strings.ToLower(id), filter) {
						extMatches = append(extMatches, id)
					}
				}
			}
			if !editorMatch && len(extMatches) == 0 {
				continue
			}
			rows := slices.DeleteFunc(a.editorRows(e, eds), func(r editorRow) bool {
				return a.editorsTab == 1 && r.command || a.editorsTab == 2 && !r.command
			})
			if len(rows) == 0 {
				continue
			}
			var size int64
			for _, r := range rows {
				if s, ok := a.editorSize(r); ok {
					size += s
				}
			}
			ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, strings.ToUpper(editorTitle(e))).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
				if size > 0 {
					ui.Text(c, formatBytes(size)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
					if a.Shell != nil {
						ui.Box(c).Width(50).Shrink(0)
					}
				}
			})
			for _, r := range rows {
				right := "…"
				switch s, ok := a.editorSize(r); {
				case r.command && r.onPath:
					right = locales.EditorsOnPath()
				case r.command:
					right = locales.EditorsNo()
				case ok:
					right = formatBytes(s)
				}
				rowKey := e.ID + ":" + r.name
				expandKey := ""
				if len(r.installed) > 0 {
					expandKey = "ext:" + e.ID
				} else if len(r.paths) > 0 {
					expandKey = "old:" + e.ID
				}
				autoExpand := len(r.installed) > 0 && !editorMatch && len(extMatches) > 0
				expanded := autoExpand || (expandKey != "" && a.editorsExpanded[expandKey])
				var cleanCand *cleanup.Candidate
				if a.BuildPlan != nil {
					for i := range cands {
						cd := cands[i]
						if len(r.paths) > 0 && cd.Kind == cleanup.OldExtensions && slices.Equal(cd.Paths, r.paths) && ops.Cleans(cd, a.Snapshot) {
							cleanCand = &cands[i]
							break
						}
						if r.path != "" && r.path == e.Caches && cd.Kind == cleanup.Cache && len(cd.Paths) == 1 && cd.Paths[0] == r.path && ops.Cleans(cd, a.Snapshot) {
							cleanCand = &cands[i]
							break
						}
					}
				}
				ui.Row(c.Key(rowKey)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						clipped(c, r.name).FontSize(13)
						clipped(c, r.about).FontSize(10.5).TextColor(pal.TextFaint)
					})
					if r.addPath != nil && a.BuildPlan != nil {
						ed := *r.addPath
						ui.Button(c.Key("add-path:"+e.ID), locales.EditorsAddToPath()).Shrink(0).OnClick(func() { a.addEditorToPath(ed) })
					} else {
						if cleanCand != nil {
							cand := *cleanCand
							title := a.candidateTitle(cand)
							ui.Button(c.Key("clean:"+rowKey), locales.RemoveEllipsis()).Height(rowControlHeight).Shrink(0).OnClick(func() {
								snap, target := a.Snapshot, a.dockerTarget()
								a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
									return ops.CleanupPlan([]cleanup.Candidate{cand}, []string{title}, snap, env, target, nil)
								}, a.proposeOrSay)
							})
						}
						if expandKey != "" && !autoExpand {
							k := expandKey
							icon, label := "chevron-down", locales.EditorsShowList()
							if expanded {
								icon, label = "chevron-up", locales.EditorsHideList()
							}
							iconButton(c.Key("toggle:"+k), icon, label).OnClick(func() {
								if a.editorsExpanded == nil {
									a.editorsExpanded = map[string]bool{}
								}
								a.editorsExpanded[k] = !a.editorsExpanded[k]
							})
						}
						ui.Text(c, right).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
						a.pathActions(c, rowKey, r.path)
					}
				})
				if expanded && len(r.installed) > 0 {
					list := slices.Clone(r.installed)
					if !editorMatch && len(extMatches) > 0 {
						list = extMatches
					}
					ui.Column(c.Key("ext-list:"+e.ID)).Gap(6).Padding(8, 16, 10, 28).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Text(c, r.name).FontSize(11).TextColor(pal.TextFaint).Grow(1)
							ui.Button(c.Key("copy-ext:"+e.ID), locales.EditorsCopyList()).Height(26).FontSize(11.5).Shrink(0).OnClick(func() {
								c.WriteClipboard(strings.Join(list, "\n"))
								c.Toast(locales.EditorsListCopied())
							})
						})
						for _, id := range list {
							clipped(c, id).Font(FontMono).FontSize(11).TextColor(pal.MonoText)
						}
					})
				}
				if expanded && len(r.paths) > 0 {
					ui.Column(c.Key("old-list:"+e.ID)).Gap(4).Padding(6, 16, 8, 28).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
						for _, p := range r.paths {
							dir := p
							ui.Row(c.Key("old:" + dir)).Gap(8).AlignItems(ui.Center).Children(func() {
								clipped(c, filepath.Base(dir)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).Grow(1).MinWidth(0)
								a.pathActions(c, "old:"+dir, dir)
							})
						}
					})
				}
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.NothingMatches())
		}
	})
}

// editorsHints are the editors' commands missing from the PATH, their
// extensions' old versions, and the way to Cleanup.
func (a *App) editorsHints(c *ui.Context, eds []inventory.Editor) {
	t := c.Theme()
	pal := Colors(t)
	var old int64
	cleanable := false
	for _, cand := range a.candidates(c) {
		if cand.Kind == cleanup.OldExtensions {
			old += cand.Frees()
		}
		if cand.Kind == cleanup.OldExtensions || cand.Kind == cleanup.Cache && cand.ID == "jetbrains" {
			cleanable = true
		}
	}
	listCard(c, locales.EditorsHints(), "", func() {
		shown := 0
		for _, e := range eds {
			if e.App != "" && e.CommandPath == "" {
				ed := e
				if ed.CommandSource != "" && a.BuildPlan != nil {
					ui.Column(c.Key("hint:"+ed.ID)).Gap(8).Padding(10, 12, 10, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Gap(2).Children(func() {
							ui.Text(c, locales.EditorsNoCommand(ed.Name)).FontSize(12.5)
							ui.Text(c, locales.EditorsNoCommandCaption(ed.Command)).FontSize(11).TextColor(pal.TextFaint)
						})
						ui.Button(c.Key("hint-add-path:"+ed.ID), locales.EditorsAddToPath()).OnClick(func() { a.addEditorToPath(ed) })
					})
				} else {
					listRow(c, locales.EditorsNoCommand(e.Name), false, locales.EditorsNoCommandCaption(e.Command), func() {})
				}
				shown++
			}
		}
		if old > 0 {
			listRow(c, locales.EditorsOld(), false, locales.EditorsRemovable(formatBytes(old)), func() {})
			shown++
		}
		if !cleanable {
			if shown == 0 {
				cardNote(c, locales.EditorsNoHints())
			}
			return
		}
		b := ui.ButtonBase(c.Key("editors-cleanup")).Column().AlignItems(ui.Start).Gap(1).Padding(9, 12, 9, 14).BorderWidth(1, 0, 0, 0).BorderColor(Colors(t).Line).
			Label(locales.MobileOpenCleanup()).OnClick(func() {
			a.cleanupTab = 0
			a.router.Push(cleanupPage)
		})
		if b.Hovered() {
			b.Background(Colors(t).Subtle)
		}
		b.Children(func() {
			ui.Text(c, locales.MobileOpenCleanup()).FontSize(13).TextColor(Accent).SingleLine()
			ui.Text(c, locales.MobileOpenCleanupCaption()).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
	})
}

// addEditorToPath proposes the plan that adds e's CLI command to PATH.
func (a *App) addEditorToPath(e inventory.Editor) {
	if a.BuildPlan == nil {
		return
	}
	a.BuildPlan(func(env sysenv.Env, term ops.Terminal) (ops.Plan, error) {
		return ops.EditorPathPlan(e, env, term)
	}, a.proposeOrSay)
}
