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
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		toolLogo(c, id, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(22).FontWeight(600).SingleLine()
				badge()
			})
			ui.Text(c, subtitle).FontSize(12.5).TextColor(c.Theme().TextMuted).SingleLine()
		})
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

// globalsCard lists packages installed globally, aside saying by what.
func globalsCard(c *ui.Context, pkgs []inventory.Package, aside string) {
	pal := Colors(c.Theme())
	listCard(c, locales.GlobalPackages(), aside, func() {
		for _, p := range pkgs {
			caption := ""
			if len(p.Commands) > 0 {
				caption = locales.PackageCommands(strings.Join(p.Commands, ", "))
			}
			listRow(c, p.Name, false, caption, func() {
				ui.Text(c, p.Version).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			})
		}
		if len(pkgs) == 0 {
			cardNote(c, locales.NoGlobalPackages())
		}
	})
}

// stateCard is one of the header's cards: a dot and a label, a value and
// a caption.
func stateCard(c *ui.Context, dot ui.Color, label, value, caption string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).MinWidth(128).Gap(3).Padding(10, 14).Radius(10).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
			ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, value).Font(FontMono).FontSize(14).SingleLine()
		ui.Text(c, caption).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
	})
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
// channel tells a version's release channel, and whether its support has
// ended. The row of the copy the terminal runs, active, stands out.
func (a *App) versionTable(c *ui.Context, ins []inventory.Installation, rows []int, active int, channel func(version string) (string, bool)) {
	home := a.Home
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 12, 8, 16).Background(pal.Subtle).Children(func() {
			for _, h := range []struct {
				title string
				width float32
			}{{locales.ColumnVersion(), 112}, {locales.ColumnSource(), 64}, {locales.ColumnPath(), 0}} {
				e := ui.Text(c, h.title).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
				if h.width > 0 {
					e.Width(h.width).Shrink(0)
				}
			}
		})
		for _, i := range rows {
			in := ins[i]
			ch, ended := channel(in.Version)
			row := ui.Row(c).Gap(12).Padding(12, 12, 12, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if i == active {
				row.Background(pal.ActiveRow)
			}
			row.Children(func() {
				ui.Column(c).Width(112).Shrink(0).Gap(2).Children(func() {
					ui.Text(c, versionText(in.Version)).Font(FontMono).FontSize(13.5).SingleLine()
					if ch != "" {
						ui.Text(c, ch).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				ui.Row(c).Width(64).Shrink(0).Children(func() { sourceBadge(c, in.Source) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					ui.Text(c, displayPath(in, home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
					ui.Row(c).Gap(5).Wrap().Children(func() {
						if in.Default {
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.ChipDefault())
						}
						switch {
						case i == active:
							chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipInPath())
						case in.OnPath > 0:
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipDuplicate())
						}
						if ended {
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipSupportEnded())
						}
					})
				})
				a.rowActions(c, in)
			})
		}
	})
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
				ui.Text(c, "which -a "+command).FontSize(12)
			})
			if len(entries) == 0 {
				ui.Text(c, locales.PathEmpty(command)).Font(FontSans).FontSize(12).TextColor(t.TextMuted).Padding(7, 10)
			}
			for n, i := range entries {
				in := ins[i]
				version := in.Version
				if in.Source.Shim && n == 0 && active >= 0 {
					version = ins[active].Version
				}
				entry := ui.Row(c).Gap(10).Padding(7, 10).Radius(6).AlignItems(ui.Center)
				color, tag, tagBg, tagText := t.TextMuted, locales.TagShadowed(), pal.Muted, t.TextMuted
				if n == 0 {
					entry.Background(t.Background)
					color, tag, tagBg, tagText = t.Text, locales.TagUsed(), pal.SuccessSoft, pal.SuccessText
				}
				entry.Children(func() {
					ui.Text(c, strconv.Itoa(n+1)).FontSize(11.5).TextColor(pal.TextFaint)
					ui.Text(c, tilde(in.Path, home)).FontSize(12).TextColor(color).Grow(1).MinWidth(0).SingleLine()
					if version != "" {
						ui.Text(c, "v"+version).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					}
					ui.Text(c, tag).Font(FontSans).FontSize(10.5).FontWeight(500).TextColor(tagText).
						Padding(1, 6).Radius(5).Background(tagBg).SingleLine()
				})
			}
		})
		for _, warning := range []string{guiWarning(command, ins, active), shadowWarning(ins, entries, home)} {
			if warning == "" {
				continue
			}
			ui.Row(c).Gap(10).Padding(10, 10, 10, 12).Radius(9).AlignItems(ui.Center).Background(pal.WarningSoft).Children(func() {
				ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(t.Warning)
				ui.Text(c, warning).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
			})
		}
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
// version of command than the terminal, which one; "" when they agree.
func guiWarning(command string, ins []inventory.Installation, active int) string {
	if active < 0 {
		return ""
	}
	gui := inventory.GUICopy(ins)
	switch {
	case gui < 0:
		return locales.GuiNotFound(command)
	case ins[gui].Version != ins[active].Version:
		return locales.GuiRunsOther(sourceName(ins[gui].Source), versionText(ins[gui].Version), versionText(ins[active].Version))
	}
	return ""
}

// shadowWarning tells of a copy in the terminal's PATH, entries in its
// order, that lies unused behind the first and could go; "" for none. A
// manager's shim first picks among its copies, and they go with it.
func shadowWarning(ins []inventory.Installation, entries []int, home string) string {
	if len(entries) < 2 || ins[entries[0]].Source.Shim {
		return ""
	}
	first := ins[entries[0]]
	for _, i := range entries[1:] {
		if in := ins[i]; !in.Source.Shim {
			return locales.CopyShadowed(sourceName(in.Source), versionText(in.Version), tilde(filepath.Dir(first.Path), home))
		}
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
		if !in.Source.Shim {
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
