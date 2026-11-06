package ui

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// toolState returns what the last scan found of the tool by its ID, and
// whether it found a copy to show.
func (a *App) toolState(id string) (inventory.ToolState, bool) {
	var st inventory.ToolState
	found := false
	if a.Snapshot != nil {
		st, found = a.Snapshot.Tool(id)
	}
	return st, found && st.Err == "" && len(st.Installations) > 0
}

// toolEmpty is a tool page's body when there is no copy to show: the scan
// is under way, failed, or found none.
func (a *App) toolEmpty(c *ui.Context, id, name string) {
	text := locales.NotChecked()
	if a.Scanning {
		text = locales.Checking()
	}
	if a.Snapshot != nil {
		switch st, ok := a.Snapshot.Tool(id); {
		case ok && st.Err != "":
			text = locales.ToolCheckFailed(name, st.Err)
		case ok:
			text = locales.ToolNotInstalled(name)
		}
	}
	ui.Column(c).Grow(1).Center().Gap(8).Padding(0, 28).Children(func() {
		ui.Icon(c, toolIcon(id)).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
		ui.Text(c, text).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
	})
}

// toolHead is the top of a tool's page: its logo, its name with what
// badge adds beside it, and subtitle under them; on the right, the cards
// of where the tool stands.
func toolHead(c *ui.Context, id, name string, badge func(), subtitle string, cards func()) {
	headRow(c, func() {
		toolLogo(c, id, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(22).FontWeight(600).SingleLine()
				badge()
			})
			ui.Text(c, subtitle).FontSize(12.5).TextColor(c.Theme().TextMuted).SingleLine()
		})
	}, cards)
}

// headRow is the top of a page: lead, its tile and title, with the cards
// of where things stand beside it, or under it across the width in a
// narrow window.
func headRow(c *ui.Context, lead, cards func()) {
	if narrow(c) {
		ui.Column(c).Gap(14).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(lead)
			ui.Row(c).Gap(8).Children(cards)
		})
		return
	}
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		lead()
		ui.Row(c).Gap(8).Shrink(0).Children(cards)
	})
}

// comingSoon stands for a tab still to be built.
func comingSoon(c *ui.Context) {
	ui.Text(c, locales.ComingSoon()).TextColor(c.Theme().TextMuted).Padding(24, 0)
}

// toolNote is a line of explanation under a table, after an info icon.
func toolNote(c *ui.Context, text string) {
	t := c.Theme()
	ui.Row(c).Gap(8).Padding(0, 4).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(Colors(t).TextFaint)
		ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
	})
}

// globalsCard lists packages installed globally under title, aside saying
// by what, or says empty without any.
func globalsCard(c *ui.Context, title, empty string, pkgs []inventory.Package, aside string) {
	pal := Colors(c.Theme())
	listCard(c, title, aside, func() {
		for _, p := range pkgs {
			caption := p.Module
			if len(p.Commands) > 0 {
				caption = locales.PackageCommands(strings.Join(p.Commands, ", "))
			}
			listRow(c, p.Name, false, caption, func() {
				ui.Text(c, p.Version).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			})
		}
		if len(pkgs) == 0 {
			cardNote(c, empty)
		}
	})
}

// stateCard is one of the header's cards: a dot and a label, a value and
// a caption.
func stateCard(c *ui.Context, dot ui.Color, label, value, caption string) {
	t := c.Theme()
	pal := Colors(t)
	e := ui.Column(c).MinWidth(128).Gap(3).Padding(10, 14)
	if narrow(c) {
		e.Grow(1).Basis(0)
	}
	e.Radius(10).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
			ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, value).Font(FontMono).FontSize(14).SingleLine()
		ui.Text(c, caption).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
	})
}

// narrowWidth is the window's width under which a page's side column goes
// under the main one, which keeps names and paths in the tables whole.
const narrowWidth = 1180

// narrow reports a window too narrow for a side column.
func narrow(c *ui.Context) bool {
	w, _ := c.Size()
	return w > 0 && w < narrowWidth
}

// columns lays out a page's main column and its side column of cards:
// side by side, or one under the other in a narrow window. side may be nil.
func columns(c *ui.Context, gap float32, main, side func()) {
	wide := !narrow(c)
	var outer ui.Element
	if wide {
		outer = ui.Row(c).Gap(18).AlignItems(ui.Start)
	} else {
		outer = ui.Column(c).Gap(18).AlignItems(ui.Stretch)
	}
	outer.Children(func() {
		m := ui.Column(c).MinWidth(0).Gap(gap)
		if wide {
			m.Grow(1)
		}
		m.Children(main)
		if side == nil {
			return
		}
		s := ui.Column(c).Gap(14)
		if wide {
			s.Width(342).Shrink(0)
		}
		s.Children(side)
	})
}

// detailColumn is the column of the item chosen in a list beside it, as
// on the Skills and MCP pages; in a narrow window the two share the width.
func detailColumn(c *ui.Context) ui.Element {
	if narrow(c) {
		return ui.Column(c).Grow(1).Basis(0).MinWidth(0)
	}
	return ui.Column(c).Width(422).Shrink(0)
}

// toolTabs switches a tool page, *tab, between the installed versions,
// the ones available, the projects and PATH.
func toolTabs(c *ui.Context, key string, tab *int, installed int) {
	t := c.Theme()
	pal := Colors(t)
	labels := []string{locales.Installed(), locales.Available(), locales.ProjectsTitle(), "PATH"}
	ui.Row(c).Children(func() {
		seg := ui.SegmentedBase(c.Key(key), tab, len(labels))
		seg.Track.Gap(2).Padding(2).Radius(8).Background(pal.Muted).Children(func() {
			for i, label := range labels {
				s := seg.Segment(i).Gap(5).Padding(5, 12).Radius(6).AlignItems(ui.Center).Label(label)
				color := t.TextMuted
				if i == *tab {
					s.Background(t.Background).Shadow(0, 1, 2, 0, t.Text.Alpha(0.1))
					color = t.Text
				}
				s.Children(func() {
					ui.Text(c, label).FontSize(12.5).TextColor(color).SingleLine()
					if i == 0 {
						ui.Text(c, strconv.Itoa(installed)).FontSize(11.5).TextColor(pal.TextFaint)
					}
				})
			}
		})
	})
}

// versionTable lists the installed versions, rows indexing ins: the
// version and its channel, the source, the path and chips of its use.
// channel tells a copy's release channel, or how it came, and whether its
// support has ended. The row of the copy the terminal runs, active, stands
// out.
func (a *App) versionTable(c *ui.Context, ins []inventory.Installation, rows []int, active int, channel func(in inventory.Installation) (string, bool)) {
	home := a.Home
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 12, 8, 16).Background(pal.Subtle).Children(func() {
			for _, h := range []struct {
				title string
				width float32
			}{{locales.ColumnVersion(), 132}, {locales.ColumnSource(), 100}, {locales.ColumnPath(), 0}} {
				e := ui.Text(c, h.title).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
				if h.width > 0 {
					e.Width(h.width).Shrink(0)
				}
			}
		})
		for _, i := range rows {
			in := ins[i]
			ch, ended := channel(in)
			row := ui.Row(c).Gap(12).Padding(12, 12, 12, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if i == active {
				row.Background(pal.ActiveRow)
			}
			row.Children(func() {
				ui.Column(c).Width(132).Shrink(0).Gap(2).Children(func() {
					ui.Text(c, copyVersion(in)).Font(FontMono).FontSize(13.5).SingleLine()
					if ch != "" {
						ui.Text(c, ch).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				ui.Row(c).Width(100).Shrink(0).Children(func() { sourceBadge(c, in.Source) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					ui.Text(c, displayPath(in, home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
					ui.Row(c).Gap(5).Wrap().Children(func() {
						if in.Java != nil && in.Java.Home == a.javaHome() {
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.JavaHomeChip())
						}
						if in.Default {
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.ChipDefault())
						}
						switch {
						case i == active:
							chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipInPath())
						case in.OnPath > 0 && !inventory.NeededBySystem(in):
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipDuplicate())
						}
						if ended {
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipSupportEnded())
						}
						if inventory.NeededBySystem(in) {
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.ChipSystem())
						}
					})
				})
				a.rowActions(c, in)
			})
		}
	})
}

// managerCard shows the version manager that keeps the tool and how it
// is set up, or that there is none and Hearth will use mise; and the rows
// extra adds for the copy in use, if any.
func (a *App) managerCard(c *ui.Context, st inventory.ToolState, rows []int, active int, extra func(*ui.Context, inventory.Installation)) {
	ins := st.Installations
	var m *inventory.Manager
	for i := range st.Managers {
		if m == nil || active >= 0 && st.Managers[i].Name == ins[active].Source.Manager {
			m = &st.Managers[i]
		}
	}
	title := locales.VersionManager()
	if m != nil {
		title += " · " + m.Name
	}
	aside := ""
	if m != nil {
		aside = m.Version
	}
	listCard(c, title, aside, func() {
		if m == nil {
			if names := sourceNames(ins, rows, active); len(names) > 0 {
				cardNote(c, locales.NoVersionManager(toolTitle(st.Tool), strings.Join(names, ", ")))
			} else {
				cardNote(c, locales.NoVersionManagerNone(toolTitle(st.Tool)))
			}
		} else {
			def := "—"
			for _, in := range ins {
				if in.Default && in.Source.Manager == m.Name {
					def = copyVersion(in)
				}
			}
			// rbenv and pyenv with no version picked run the system's.
			if def == "—" && inventory.FallsToSystem(m.Name) {
				def = "system"
			}
			// A manager that keeps no default, as uv, has no row of it.
			if def != "—" || len(m.Settings) == 0 {
				listRow(c, locales.DefaultVersion(), false, "", func() {
					ui.Text(c, def).Font(FontMono).FontSize(12).TextColor(Colors(c.Theme()).MonoText).SingleLine()
				})
			}
			if title, caption := hookup(*m); title != "" {
				listRow(c, title, false, caption, func() { stateText(c, true) })
			}
			if m.ConfigPath != "" {
				listRow(c, locales.ManagerGlobalConfig(), false, tilde(m.ConfigPath, a.Home), func() {})
			}
			for _, st := range managerSettings(*m) {
				listRow(c, st.title, false, st.caption, func() { stateText(c, st.on) })
			}
			// A manager that does not switch at all, as uv, says so by
			// its settings.
			if m.SwitchOnCD || len(m.Settings) == 0 {
				caption := locales.SwitchOff()
				if m.Name == "mise" {
					caption = locales.MiseSwitchUnknown()
				}
				if m.SwitchOnCD {
					caption = locales.ReadsFiles(strings.Join(m.Files, ", "))
				}
				listRow(c, locales.SwitchOnCd(), false, caption, func() { stateText(c, m.SwitchOnCD) })
			}
		}
		if active >= 0 && extra != nil {
			extra(c, ins[active])
		}
	})
}

// hookup says how a manager like mise is hooked in and who sees it: its
// shims, which apps see when they are on their PATH too, or its
// activation in the shell, which only the terminal sees. "" when neither
// is known.
func hookup(m inventory.Manager) (title, caption string) {
	switch {
	// mise activate may put the shims on the terminal's PATH too; the
	// activation is what switches there.
	case m.Activated && m.ShimsInGUI:
		return locales.HookActivate(m.Name), locales.MiseMixedHook()
	case m.Activated:
		return locales.HookActivate(m.Name), locales.SeenInTerminal()
	case m.Shims && m.ShimsInGUI:
		return locales.HookShims(), locales.SeenEverywhere()
	case m.Shims:
		return locales.HookShims(), locales.SeenInTerminal()
	case m.ShimsInGUI:
		return locales.HookShims(), locales.SeenInApps()
	}
	return "", ""
}

// listCard is a card of rows under a title, with a note on the right of
// the title when aside is not empty.
func listCard(c *ui.Context, title, aside string, rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			if aside != "" {
				ui.Text(c, aside).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
		rows()
	})
}

// listRow is a row of a list card: a name, mono for a version, with a
// caption under it, and what end builds on the right.
func listRow(c *ui.Context, name string, mono bool, caption string, end func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).Padding(9, 12, 9, 14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			n := ui.Text(c, name).FontSize(13).SingleLine()
			if mono {
				n.Font(FontMono)
			}
			if caption != "" {
				ui.Text(c, caption).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
		end()
	})
}

// cardNote is a list card's text in place of rows.
func cardNote(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted).Padding(10, 14, 14, 14).
		BorderWidth(1, 0, 0, 0).BorderColor(Colors(c.Theme()).Line)
}

// stateText says whether a setting is on.
func stateText(c *ui.Context, on bool) {
	pal := Colors(c.Theme())
	if on {
		ui.Text(c, locales.StateOn()).FontSize(12).FontWeight(500).TextColor(pal.SuccessText).SingleLine()
		return
	}
	ui.Text(c, locales.StateOff()).FontSize(12).TextColor(pal.TextFaint).SingleLine()
}

// pathCard shows how the terminal finds command: the copies in its PATH
// in order, as which -a lists them, the first used and the rest shadowed;
// and warnings when apps started outside the terminal run another one,
// and when another copy lies unused behind the first.
func pathCard(c *ui.Context, command string, ins []inventory.Installation, active int, home string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(10).Padding(14).Radius(12).Background(t.Background).
		BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("terminal")).Size(15, 15).TextColor(t.TextMuted)
			ui.Text(c, locales.HowTerminalFinds(command)).FontSize(13.5).FontWeight(600).SingleLine()
		})
		entries := pathOrder(ins)
		ui.Column(c).Gap(2).Padding(10, 6, 6, 6).Radius(9).Background(pal.Subtle).
			BorderWidth(1).BorderColor(pal.Line).Font(FontMono).Children(func() {
			ui.Row(c).Gap(6).Padding(0, 0, 4, 10).Children(func() {
				ui.Text(c, "$").FontSize(12).TextColor(pal.TextFaint)
				ui.Text(c, whichAllCommand+" "+command).FontSize(12)
			})
			if len(entries) == 0 {
				ui.Text(c, locales.PathEmpty(command)).Font(FontSans).FontSize(12).TextColor(t.TextMuted).Padding(7, 10)
			}
			for n, i := range entries {
				in := ins[i]
				shown := in
				if in.Source.Shim && n == 0 && active >= 0 {
					shown = ins[active]
				}
				entry := ui.Row(c).Gap(10).Padding(7, 10).Radius(6).AlignItems(ui.Center)
				color, tag, tagBg, tagText := t.TextMuted, locales.TagShadowed(), pal.Muted, t.TextMuted
				if n == 0 {
					entry.Background(t.Background)
					color, tag, tagBg, tagText = t.Text, locales.TagUsed(), pal.SuccessSoft, pal.SuccessText
				}
				if in.Stub {
					tag, tagBg, tagText = locales.TagStub(), pal.WarningSoft, pal.WarningText
				}
				entry.Children(func() {
					ui.Text(c, strconv.Itoa(n+1)).FontSize(11.5).TextColor(pal.TextFaint)
					ui.Text(c, tilde(in.Path, home)).FontSize(12).TextColor(color).Grow(1).MinWidth(0).SingleLine()
					if shown.Version != "" {
						ui.Text(c, copyVersion(shown)).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					}
					ui.Text(c, tag).Font(FontSans).FontSize(10.5).FontWeight(500).TextColor(tagText).
						Padding(1, 6).Radius(5).Background(tagBg).SingleLine()
				})
			}
		})
		// That apps do not find the command is only worth knowing, as on
		// the overview.
		if active >= 0 && inventory.GUICopy(ins) < 0 {
			toolNote(c, locales.GuiNotFound(command))
		}
		for _, warning := range []string{guiWarning(ins, active), shadowWarning(ins, entries, home), stubWarning(ins, entries, home)} {
			if warning == "" {
				continue
			}
			warningRow(c, warning)
		}
	})
}

// warningRow is a warning inside a card.
func warningRow(c *ui.Context, text string) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(10).Padding(10, 10, 10, 12).Radius(9).AlignItems(ui.Center).Background(pal.WarningSoft).Children(func() {
		ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
		ui.Text(c, text).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
	})
}

// pathOrder returns the installations in the terminal's PATH, by index,
// in its order.
func pathOrder(ins []inventory.Installation) []int {
	var order []int
	for i, in := range ins {
		if in.OnPath > 0 {
			order = append(order, i)
		}
	}
	sort.Slice(order, func(a, b int) bool { return ins[order[a]].OnPath < ins[order[b]].OnPath })
	return order
}

// guiWarning tells, when apps started outside the terminal run another
// version of command than the terminal, which one; "" when they agree or
// find none.
func guiWarning(ins []inventory.Installation, active int) string {
	if active < 0 {
		return ""
	}
	gui := inventory.GUICopy(ins)
	switch {
	case gui < 0:
	case ins[gui].Version != ins[active].Version:
		return locales.GuiRunsOther(sourceName(ins[gui].Source), copyVersion(ins[gui]), copyVersion(ins[active]))
	}
	return ""
}

// shadowWarning tells of a copy in the terminal's PATH, entries in its
// order, that lies unused behind the first; "" for none. A shim first
// stands for its manager's copies, which are not unused.
func shadowWarning(ins []inventory.Installation, entries []int, home string) string {
	if len(entries) < 2 {
		return ""
	}
	first := ins[entries[0]]
	for _, i := range entries[1:] {
		in := ins[i]
		if in.Source.Shim || in.Stub || inventory.NeededBySystem(in) || first.Source.Shim && in.Source.Manager == first.Source.Manager {
			continue
		}
		return locales.CopyShadowed(sourceName(in.Source), copyVersion(in), tilde(filepath.Dir(first.Path), home))
	}
	return ""
}

// rowActions are a version's actions: copy its path, show it in the file
// manager, and a menu with those, and npm's settings for Node.
func (a *App) rowActions(c *ui.Context, in inventory.Installation) {
	if a.Shell == nil {
		return
	}
	path := copyPath(in)
	copyIt := func() {
		c.WriteClipboard(path)
		c.Toast(locales.PathCopied())
	}
	reveal := func() { a.Shell.ShowItemInFolder(in.RealPath) }
	ui.Row(c.Key("actions:" + in.RealPath)).Gap(2).Shrink(0).Children(func() {
		iconButton(c, "copy", locales.CopyPath()).OnClick(copyIt)
		iconButton(c, "folder-open", showInFolder()).OnClick(reveal)
		iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(m *ui.Menu) {
			if m.Item(locales.CopyPath()).Chosen() {
				copyIt()
			}
			if m.Item(showInFolder()).Chosen() {
				reveal()
			}
			if a.Home == "" || in.Tool != "node" {
				return
			}
			m.Separator()
			if m.Item(locales.OpenNpmrc()).Chosen() {
				npmrc := filepath.Join(a.Home, ".npmrc")
				if err := a.Shell.OpenPath(npmrc); err != nil {
					c.Toast(locales.OpenFailed(tilde(npmrc, a.Home), err.Error()))
				}
			}
		})
	})
}

// iconButton is a small button showing a Lucide icon, named by label.
func iconButton(c *ui.Context, icon, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Size(24, 24).Radius(6).Center().Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(Colors(t).Muted)
	}
	return b.Children(func() {
		ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(t.TextMuted)
	})
}

// copyPath is the path a version's copy action copies: what the table
// shows, in full.
func copyPath(in inventory.Installation) string {
	return displayPath(in, "")
}

// sourceBadge names what installed a copy: its manager in a badge, or a
// dash when nothing is known.
func sourceBadge(c *ui.Context, s inventory.Source) {
	pal := Colors(c.Theme())
	if s.Kind == inventory.KindUnknown {
		ui.Text(c, "—").TextColor(pal.TextFaint)
		return
	}
	ui.Text(c, sourceName(s)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).SingleLine().
		Padding(2, 7).Radius(5).Background(c.Theme().Background).BorderWidth(1).BorderColor(pal.ControlBorder)
}

// chip is a status: a dot and a text on a soft background.
func chip(c *ui.Context, dot, bg, text ui.Color, label string) {
	ui.Row(c).Gap(5).Padding(2, 6).Radius(6).AlignItems(ui.Center).Background(bg).Children(func() {
		ui.Box(c).Size(5, 5).Radius(2.5).Background(dot)
		ui.Text(c, label).FontSize(11).TextColor(text).SingleLine()
	})
}

// listed returns the installations the table lists, by index: all but
// the managers' shims, which stand for another copy.
func listed(ins []inventory.Installation) []int {
	var rows []int
	for i, in := range ins {
		if !in.Source.Shim && !in.Stub {
			rows = append(rows, i)
		}
	}
	return rows
}

// sourceNames lists where the copies at rows came from, each once, the
// active one's first.
func sourceNames(ins []inventory.Installation, rows []int, active int) []string {
	var names []string
	seen := map[string]bool{}
	add := func(s inventory.Source) {
		if n := sourceName(s); !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if active >= 0 {
		add(ins[active].Source)
	}
	for _, i := range rows {
		add(ins[i].Source)
	}
	return names
}

// sourceName is what put a copy there, in a word: the manager, or the
// kind of place for a copy no manager keeps.
func sourceName(s inventory.Source) string {
	switch {
	case s.Manager == inventory.ScriptInstall:
		return locales.SourceScript()
	case s.Manager != "":
		return s.Manager
	case s.Kind == inventory.KindInstaller:
		return locales.SourceInstaller()
	case s.Kind == inventory.KindSystem:
		return locales.SourceSystem()
	}
	return locales.SourceOther()
}

// displayPath is where a copy lies, home shortened to ~: a version
// manager's directory of the version, or the path found.
func displayPath(in inventory.Installation, home string) string {
	p := in.Path
	if in.Source.Kind == inventory.KindVersionManager && in.Source.Root != "" {
		p = in.Source.Root
	}
	return tilde(p, home)
}

// tilde shortens home at the start of p to ~.
func tilde(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/") || strings.HasPrefix(p, home+`\`)) {
		return "~" + p[len(home):]
	}
	return p
}

// versionText shows a version as Node does, v22.12.0.
func versionText(v string) string {
	if v == "" {
		return locales.UnknownVersion()
	}
	return "v" + v
}

// copyVersion shows a copy's version as its tool does: go1.23.4 for Go,
// 3.13.1 for Python, stable 1.84.0 for a Rust toolchain, v22.12.0 for
// the rest.
func copyVersion(in inventory.Installation) string {
	switch {
	case in.Version == "":
		return versionText(in.Version)
	case in.Tool == "dotnet":
		return "SDK " + in.Version
	case in.Tool == "dotnet-runtime":
		return "Runtime " + in.Version
	case in.Tool == "go":
		return "go" + in.Version
	case in.Tool == "docker" || in.Tool == "python" || in.Tool == "php" || in.Tool == "java" || in.Tool == "ruby":
		return in.Version
	case in.Tool == "rust":
		switch ch := rustChannel(in); ch {
		case "nightly":
			return ch
		case "stable", "beta":
			return ch + " " + in.Version
		}
		return in.Version
	}
	return versionText(in.Version)
}

// stubWarning tells of a stub in the terminal's PATH, entries in its
// order, that does not run the tool: what it does instead; "" for none.
func stubWarning(ins []inventory.Installation, entries []int, home string) string {
	for _, i := range entries {
		if in := ins[i]; in.Stub {
			if in.Tool == "java" {
				return locales.JavaStubWarning()
			}
			if strings.Contains(strings.ToLower(in.Path), `\windowsapps\`) {
				if in.OnPath == 1 {
					return locales.StubStoreFirstWarning(tilde(in.Path, home))
				}
				return locales.StubStoreWarning(tilde(in.Path, home))
			}
			return locales.StubCltWarning(in.Path)
		}
	}
	return ""
}
