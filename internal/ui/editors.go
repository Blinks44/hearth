package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
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
	cmd := editorRow{name: locales.EditorsCommand(e.Command), command: true, onPath: e.CommandPath != ""}
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
		rows = append(rows, editorRow{name: name, about: strings.Join(about, " · "), path: e.Extensions})
	}
	if len(e.Superseded) > 0 {
		rows = append(rows, editorRow{name: locales.EditorsOld(), about: locales.EditorsOldAbout() + " · " + locales.EditorsFolders(len(e.Superseded)), paths: e.Superseded})
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
						ui.Text(c, locales.EditorsLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.EditorsFound(), strconv.Itoa(len(eds)), locales.EditorsFoundCaption())
					stateCard(c, t.Success, locales.EditorsCommands(), locales.EditorsOf(len(commands), len(eds)), strings.Join(commands, ", "))
					total := "…" // until the count reaches every folder
					if counted {
						total = formatBytes(size)
					}
					stateCard(c, t.Warning, locales.EditorsExtensionsCard(), total, locales.EditorsExtensions(extensions))
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
					ui.SearchField(c.Key("editors-filter"), &a.editorsFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					switch {
					case a.Snapshot == nil:
						toolNote(c, locales.CleanupWaiting())
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
// filtered by the editor's name.
func (a *App) editorsTable(c *ui.Context, eds []inventory.Editor) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(a.editorsFilter)
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for _, e := range eds {
			if !strings.Contains(strings.ToLower(e.Name+" "+e.Command), filter) {
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
				ui.Row(c.Key(e.ID+":"+r.name)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						clipped(c, r.name).FontSize(13)
						clipped(c, r.about).FontSize(10.5).TextColor(pal.TextFaint)
					})
					ui.Text(c, right).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
				})
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.CleanupNone())
		}
	})
}

// editorsHints are the editors' commands missing from the PATH, their
// extensions' old versions, and the way to Cleanup.
func (a *App) editorsHints(c *ui.Context, eds []inventory.Editor) {
	t := c.Theme()
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
				listRow(c, locales.EditorsNoCommand(e.Name), false, locales.EditorsNoCommandCaption(e.Command), func() {})
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
