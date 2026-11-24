package ui

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
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
	a.toolEmptyWith(c, id, name, nil)
}

// toolEmptyWith is toolEmpty with an optional extra action button rendered
// below the empty-state message.
func (a *App) toolEmptyWith(c *ui.Context, id, name string, action func()) {
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
		ui.Icon(c, mustIcon(toolIcon(id))).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
		ui.Text(c, text).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
		install, named := emptyInstalls[id]
		if !named {
			install = id
		}
		a.installPrimary(c, install)
		if action != nil {
			action()
		}
	})
}

// emptyInstalls are the catalog's tools a page offers to install when
// its scan found nothing, by the page's tool; the tool itself when not
// named.
var emptyInstalls = map[string]string{"packages": "homebrew", "cpp": "", "databases": "postgresql"}

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

// controlHeight is the standard height for adjacent buttons, search fields,
// selects, and segmented tracks in toolbars, headers, and dialog footers.
const controlHeight = 28

// stateCard is one of the header's cards: a dot and a label, a value and
// a caption.
func stateCard(c *ui.Context, dot ui.Color, label, value, caption string) {
	drawStateCard(c, dot, false, label, value, caption)
}

func stateCardBusy(c *ui.Context, label, value, caption string) {
	drawStateCard(c, Colors(c.Theme()).Info, true, label, value, caption)
}

func drawStateCard(c *ui.Context, dot ui.Color, busy bool, label, value, caption string) {
	t := c.Theme()
	pal := Colors(t)
	bg, border := pal.Subtle, pal.Line
	if busy {
		bg, border = pal.InfoSoft, pal.Info.Alpha(0.28)
	}
	e := ui.Column(c.Key("state-card:"+label)).Transition(stateCardMotion).MinWidth(128).Gap(3).Padding(10, 14)
	if narrow(c) {
		e.Grow(1).Basis(0)
	}
	e.Radius(10).Background(bg).
		BorderWidth(1).BorderColor(border).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if busy {
				ui.Spinner(c).Size(11, 11).TextColor(pal.Info)
			} else {
				ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
			}
			labelColor := t.TextMuted
			if busy {
				labelColor = pal.InfoText
			}
			ui.Text(c, label).FontSize(11.5).TextColor(labelColor).SingleLine()
		})
		ui.Text(c, value).Font(FontMono).FontSize(14).SingleLine().ContentTransition(ui.ContentTransition{Numeric: true})
		captionColor := pal.TextFaint
		if busy {
			captionColor = pal.InfoText
		}
		ui.Text(c, caption).FontSize(10.5).TextColor(captionColor).SingleLine()
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

// clipped is s on one line, cut with "…" where it does not fit; the
// pointer resting on it shows it whole.
func clipped(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).SingleLine().Tooltip(s)
}

// nameWidth is the width of a table's column of names, as packages' and
// containers': a share of the table, for its names are as long as people
// make them.
func nameWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	table := w - 245 - 56 // the sidebar and the page's padding
	if !narrow(c) {
		table -= 342 + 18 // the side column
	}
	return min(max(table*0.3, 130), 240)
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
func toolTabs(c *ui.Context, key string, tab *int, installed, projectsCount int) {
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
					} else if i == 2 && projectsCount > 0 {
						ui.Text(c, strconv.Itoa(projectsCount)).FontSize(11.5).TextColor(pal.TextFaint)
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
					clipped(c, displayPath(in, home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted)
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

// managerCard shows every version manager found for the tool and how each
// is set up, with the active copy's manager first, or says there is none
// and Hearth will use mise; extra adds rows for the copy in use on the
// first card.
func (a *App) managerCard(c *ui.Context, st inventory.ToolState, rows []int, active int, extra func(*ui.Context, inventory.Installation)) {
	ins := st.Installations
	if len(st.Managers) == 0 {
		listCard(c, locales.VersionManager(), "", func() {
			if names := sourceNames(ins, rows, active); len(names) > 0 {
				note := locales.NoVersionManager(toolTitle(st.Tool), strings.Join(names, ", "))
				// PHP and the like do not install through mise.
				if _, ok := ops.VersionPlan(ops.VersionInstall, ops.VersionTarget{Tool: st.Tool, Manager: "mise", Version: "0"}, nil); !ok {
					note = locales.NoVersionManagerOwn(toolTitle(st.Tool), strings.Join(names, ", "))
				}
				cardNote(c, note)
			} else {
				cardNote(c, locales.NoVersionManagerNone(toolTitle(st.Tool)))
			}
			if active >= 0 && extra != nil {
				extra(c, ins[active])
			}
		})
		return
	}
	ordered := make([]inventory.Manager, 0, len(st.Managers))
	if active >= 0 {
		for _, m := range st.Managers {
			if m.Name == ins[active].Source.Manager {
				ordered = append(ordered, m)
			}
		}
	}
	for _, m := range st.Managers {
		if active >= 0 && m.Name == ins[active].Source.Manager {
			continue
		}
		ordered = append(ordered, m)
	}
	for idx, m := range ordered {
		listCard(c, locales.VersionManager()+" · "+m.Name, m.Version, func() {
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
			if title, caption := hookup(m); title != "" {
				listRow(c, title, false, caption, func() { stateText(c, true) })
			}
			// Hooked in for the apps alone, mise still does not switch
			// versions in the terminal.
			if m.Name == "mise" && !m.Activated && !m.Shims && a.BuildPlan != nil {
				listRowWrapped(c, locales.MiseHook(), locales.MiseNotHooked(), func() {
					ui.Button(c.Key("mise-setup:"+st.Tool), locales.MiseSetUp()).Shrink(0).OnClick(a.setUpMise)
				})
			}
			if m.ConfigPath != "" {
				cfgPath := m.ConfigPath
				listRow(c, locales.ManagerGlobalConfig(), false, tilde(cfgPath, a.Home), func() {
					ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
						if a.canOpenFile() {
							iconButton(c.Key("mgr-open:"+m.Name), "external-link", locales.OpenFile(filepath.Base(cfgPath))).
								OnClick(func() { a.openFile(c, cfgPath) })
						}
						a.pathActions(c, "mgr-cfg:"+m.Name, cfgPath)
					})
				})
			}
			for _, setting := range managerSettings(m) {
				listRow(c, setting.title, false, setting.caption, func() { stateText(c, setting.on) })
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
			if idx == 0 && active >= 0 && extra != nil {
				extra(c, ins[active])
			}
		})
	}
}

// setUpMise offers the plan that installs mise and hooks it in, once it
// is made.
func (a *App) setUpMise() {
	a.BuildPlan(ops.MisePlan, a.proposeOrSay)
}

// proposeOrSay offers p, or says why there is none.
func (a *App) proposeOrSay(p ops.Plan, err error) {
	if err != nil {
		a.planErr = err.Error()
		return
	}
	a.Propose(p)
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
	var headerAside func()
	if aside != "" {
		headerAside = func() {
			ui.Text(c, aside).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
		}
	}
	listCardHeader(c, title, headerAside, rows)
}

// listCardHeader is a list card whose header right side is built by aside.
func listCardHeader(c *ui.Context, title string, aside func(), rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			if aside != nil {
				aside()
			}
		})
		rows()
	})
}

// listRow is a row of a list card: a name, mono for a version, with a
// caption under it, and what end builds on the right.
func listRow(c *ui.Context, name string, mono bool, caption string, end func()) {
	drawListRow(c, name, mono, caption, false, end)
}

func listRowWrapped(c *ui.Context, name, caption string, end func()) {
	drawListRow(c, name, false, caption, true, end)
}

func drawListRow(c *ui.Context, name string, mono bool, caption string, wrap bool, end func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).MinHeight(46).Padding(8, 12, 8, 14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			n := ui.Text(c, name).FontSize(13)
			if wrap {
				n.Wrap()
			} else {
				n.SingleLine()
				if name != "" {
					n.Tooltip(name)
				}
			}
			if mono {
				n.Font(FontMono)
			}
			if caption != "" {
				e := ui.Text(c, caption).FontSize(11.5).TextColor(pal.TextFaint)
				if wrap {
					e.Wrap()
				} else {
					e.SingleLine().Tooltip(caption)
				}
			}
		})
		end()
	})
}

// toggle is a switch, green when on as the designs have it: the theme's
// accent, Hearth's orange, is for buttons.
func toggle(c *ui.Context, key any, on *bool) ui.Element {
	sw := ui.Switch(c.Key(key), on)
	if *on {
		sw.Background(c.Theme().Success)
	}
	return sw
}

// stepper is a bordered numeric control [- | value | +] with centered minus
// and plus icon buttons, shared between Colima resource limits and Kubernetes
// replica scaling.
func stepper(c *ui.Context, value string, valueMinWidth float32, lessKey, moreKey, lessLabel, moreLabel string, lessOff, moreOff bool, onLess, onMore func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c).Height(podActionHeight).Radius(6).Clip().BorderWidth(1).BorderColor(pal.ControlBorder).AlignItems(ui.Center).
		Children(func() {
			step := func(key, icon, label string, off bool, click func()) {
				b := ui.ButtonBase(c.Key(key)).Size(28, podActionHeight).Center().Label(label).Tooltip(label).
					Disabled(off).OnClick(click)
				color := t.Text
				switch {
				case off:
					color = pal.TextFaint
				case b.Hovered():
					b.Background(pal.Muted)
				}
				b.Children(func() { ui.Icon(c, Icon(icon)).Size(13, 13).TextColor(color) })
			}
			step(lessKey, "minus", lessLabel, lessOff, onLess)
			ui.Row(c).Height(podActionHeight).MinWidth(valueMinWidth).Padding(0, 8).AlignItems(ui.Center).Justify(ui.Center).
				BorderWidth(0, 1, 0, 1).BorderColor(pal.ControlBorder).Children(func() {
				ui.Text(c, value).Font(FontMono).FontSize(12.5).SingleLine()
			})
			step(moreKey, "plus", moreLabel, moreOff, onMore)
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
					clipped(c, tilde(in.Path, home)).FontSize(12).TextColor(color).Grow(1).MinWidth(0)
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
	warningWith(c, text, nil)
}

// warningWith is warningRow with what action adds after the text, as a
// button; nil adds nothing.
func warningWith(c *ui.Context, text string, action func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(10).Padding(10, 10, 10, 12).Radius(9).AlignItems(ui.Center).Background(pal.WarningSoft).Children(func() {
		ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
		ui.Text(c, text).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0)
		if action != nil {
			action()
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
			if pm, pkg, ok := a.copyPackage(in); ok && a.packageCan(pm, pkg) {
				m.Separator()
				a.packageItems(m, pm, pkg)
			} else {
				a.versionMenu(m, in)
			}
			a.moveMenu(m, in)
			if in.Tool != "node" || a.Home == "" {
				return
			}
			m.Separator()
			if m.Item(locales.OpenNpmrc()).Chosen() {
				a.openFile(c, filepath.Join(a.Home, ".npmrc"))
			}
		})
	})
}

// pathActions are a path's copy and reveal icon buttons, or an empty
// spacer of the same width when path is empty so table columns stay aligned.
// When Shell is nil, nothing is drawn.
func (a *App) pathActions(c *ui.Context, key, path string) {
	if a.Shell == nil {
		return
	}
	if path == "" {
		ui.Box(c).Size(50, 24).Shrink(0)
		return
	}
	ui.Row(c.Key("path-actions:" + key)).Gap(2).Shrink(0).Children(func() {
		iconButton(c, "copy", locales.CopyPath()).OnClick(func() {
			c.WriteClipboard(path)
			c.Toast(locales.PathCopied())
		})
		iconButton(c, "folder-open", showInFolder()).OnClick(func() {
			a.Shell.ShowItemInFolder(path)
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
func chip(c *ui.Context, dot, bg, text ui.Color, label string) ui.Element {
	return ui.Row(c).Gap(5).Padding(2, 6).Radius(6).AlignItems(ui.Center).Background(bg).Children(func() {
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
	if in.Source.Kind == inventory.KindVersionManager && in.Source.Root != "" && in.Tool != "dotnet" && in.Tool != "dotnet-runtime" {
		p = in.Source.Root
	}
	return tilde(p, home)
}

// tilde shortens home at the start of p to ~.
func tilde(p, home string) string {
	if home == "" {
		return p
	}
	if p == home || strings.HasPrefix(p, home+"/") || strings.HasPrefix(p, home+`\`) {
		return "~" + p[len(home):]
	}
	hp, pp := filepath.ToSlash(home), filepath.ToSlash(p)
	if pp == hp || strings.HasPrefix(pp, hp+"/") {
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
	case in.Tool == "docker" || in.Tool == "python" || in.Tool == "php" || in.Tool == "java" || in.Tool == "ruby" || in.Tool == "cpp":
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
			return locales.StubCltWarning(in.Path, filepath.Base(in.Path))
		}
	}
	return ""
}
