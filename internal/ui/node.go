package ui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// Node's brand green, on a tile tinted with it.
var (
	nodeGreen = ui.Hex("#5FA04E")
	nodeTile  = [2]struct{ bg, border ui.Color }{
		{ui.Hex("#EEF6EA"), ui.Hex("#DDEBD6")}, // light
		{ui.Hex("#1F2B1C"), ui.Hex("#2F4128")}, // dark
	}
)

// node is the Node.js page: the tool's header with where it stands, and
// the installed versions.
func (a *App) node(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.nodeState()
		if !ok {
			a.nodeEmpty(c)
			return
		}
		rows := listed(st.Installations)
		active := terminalIndex(st.Installations)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				nodeHeader(c, a.NodeVersions, st.Installations, rows, active)
				a.nodeTabs(c, len(rows))
				switch a.nodeTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							versionTable(c, a.NodeVersions, st.Installations, rows, active, a.Home)
							pathCard(c, st.Installations, active, a.Home)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.availableCard(c, st.Installations, rows)
							managerCard(c, st, rows, active)
							if active >= 0 {
								globalsCard(c, st.Installations[active])
							}
						})
					})
				case 1:
					a.availableCard(c, st.Installations, rows)
				case 3:
					pathCard(c, st.Installations, active, a.Home)
				default:
					ui.Text(c, locales.ComingSoon()).TextColor(c.Theme().TextMuted).Padding(24, 0)
				}
			})
		})
	})
}

// nodeState returns what the last scan found of Node, and whether it found
// a copy to show.
func (a *App) nodeState() (inventory.ToolState, bool) {
	var st inventory.ToolState
	found := false
	if a.Snapshot != nil {
		st, found = a.Snapshot.Tool("node")
	}
	return st, found && st.Err == "" && len(st.Installations) > 0
}

// nodeEmpty is the page's body when there is no copy to show: the scan
// is under way, failed, or found none.
func (a *App) nodeEmpty(c *ui.Context) {
	text := locales.NotChecked()
	if a.Scanning {
		text = locales.Checking()
	}
	if a.Snapshot != nil {
		switch st, ok := a.Snapshot.Tool("node"); {
		case ok && st.Err != "":
			text = locales.ToolCheckFailed("Node.js", st.Err)
		case ok:
			text = locales.ToolNotInstalled("Node.js")
		}
	}
	ui.Column(c).Grow(1).Center().Gap(8).Padding(0, 28).Children(func() {
		ui.Icon(c, Brand("nodedotjs")).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
		ui.Text(c, text).TextColor(c.Theme().TextMuted).TextAlign(ui.Center)
	})
}

// nodeHeader is the logo, the name and how the versions are managed, and
// on the right the cards of where Node stands: installed, in the
// terminal, in a project.
func nodeHeader(c *ui.Context, nv *versions.Node, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	tile := nodeTile[0]
	if t.Dark {
		tile = nodeTile[1]
	}
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(48, 48).Shrink(0).Radius(12).Center().Background(tile.bg).
			BorderWidth(1).BorderColor(tile.border).Children(func() {
			ui.Icon(c, Brand("nodedotjs")).Size(26, 26).TextColor(nodeGreen)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Node.js").FontSize(22).FontWeight(600).SingleLine()
				if active >= 0 && isLTS(nv, ins[active].Version, c.Now()) {
					ui.Text(c, "LTS").FontSize(10.5).FontWeight(600).TextColor(pal.SuccessText).
						Padding(2, 6).Radius(5).Background(pal.SuccessSoft)
				}
			})
			ui.Text(c, managedBy(ins, rows, active)).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Row(c).Gap(8).Shrink(0).Children(func() {
			stateCard(c, pal.TextFaint, locales.Installed(), locales.VersionsCount(len(rows)),
				strings.Join(sourceNames(ins, rows, -1), " · "))
			if active >= 0 {
				in := ins[active]
				stateCard(c, t.Success, locales.InTerminal(), versionText(in.Version), "which node → "+sourceName(in.Source))
			} else {
				stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
			}
			stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
		})
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

// nodeTabs switches between the installed versions, the ones available,
// the projects and PATH.
func (a *App) nodeTabs(c *ui.Context, installed int) {
	t := c.Theme()
	pal := Colors(t)
	labels := []string{locales.Installed(), locales.Available(), locales.ProjectsTitle(), "PATH"}
	ui.Row(c).Children(func() {
		seg := ui.SegmentedBase(c.Key("node-tabs"), &a.nodeTab, len(labels))
		seg.Track.Gap(2).Padding(2).Radius(8).Background(pal.Muted).Children(func() {
			for i, label := range labels {
				s := seg.Segment(i).Gap(5).Padding(5, 12).Radius(6).AlignItems(ui.Center).Label(label)
				color := t.TextMuted
				if i == a.nodeTab {
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
// The row of the copy the terminal runs, active, stands out.
func versionTable(c *ui.Context, nv *versions.Node, ins []inventory.Installation, rows []int, active int, home string) {
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
			ch, ended := channel(nv, in.Version, c.Now())
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
			})
		}
	})
}

// availableCard lists the newest version of each supported line, as the
// last version list says, with how each stands to what is installed.
func (a *App) availableCard(c *ui.Context, ins []inventory.Installation, rows []int) {
	pal := Colors(c.Theme())
	nv := a.NodeVersions
	asOf := ""
	if nv != nil && !nv.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(nv.Fetched)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if nv == nil {
			text := locales.VersionsLoading()
			if a.NodeVersionsErr != "" {
				text = locales.VersionsFailed(a.NodeVersionsErr)
			}
			cardNote(c, text)
			return
		}
		now := c.Now()
		recommended, _ := nv.Recommended()
		var installed []string
		for _, i := range rows {
			installed = append(installed, ins[i].Version)
		}
		for _, r := range nv.Latest(now) {
			caption, _ := channel(nv, r.Version, now)
			if note := releaseNote(r, nv.Lines[versions.Major(r.Version)], installed, now); note != "" {
				caption += " · " + note
			}
			listRow(c, "v"+r.Version, true, caption, func() {
				if r.Version == recommended.Version {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
			})
		}
	})
}

// managerCard shows the version manager that keeps Node and how it is
// set up, or that there is none and Hearth will use mise; and whether
// Corepack is on for the version in use.
func managerCard(c *ui.Context, st inventory.ToolState, rows []int, active int) {
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
	listCard(c, title, "", func() {
		if m == nil {
			cardNote(c, locales.NoVersionManager(strings.Join(sourceNames(ins, rows, active), ", ")))
		} else {
			def := "—"
			for _, in := range ins {
				if in.Default && in.Source.Manager == m.Name {
					def = versionText(in.Version)
				}
			}
			listRow(c, locales.DefaultVersion(), false, "", func() {
				ui.Text(c, def).Font(FontMono).FontSize(12).TextColor(Colors(c.Theme()).MonoText).SingleLine()
			})
			caption := locales.SwitchOff()
			if m.SwitchOnCD {
				caption = locales.ReadsFiles(strings.Join(m.Files, ", "))
			}
			listRow(c, locales.SwitchOnCd(), false, caption, func() { stateText(c, m.SwitchOnCD) })
		}
		if active < 0 {
			return
		}
		in := ins[active]
		listRow(c, "Corepack", false, locales.CorepackCaption(), func() {
			if !hasPackage(in, "corepack") && !in.Corepack {
				ui.Text(c, locales.CorepackMissing()).FontSize(12).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
				return
			}
			stateText(c, in.Corepack)
		})
	})
}

// globalsCard lists the packages installed globally with the Node
// version in use, but npm and Corepack, which come with it.
func globalsCard(c *ui.Context, in inventory.Installation) {
	pal := Colors(c.Theme())
	listCard(c, locales.GlobalPackages(), "npm -g · "+versionText(in.Version), func() {
		shown := 0
		for _, p := range in.Packages {
			if p.Name == "npm" || p.Name == "corepack" {
				continue
			}
			shown++
			caption := ""
			if len(p.Commands) > 0 {
				caption = locales.PackageCommands(strings.Join(p.Commands, ", "))
			}
			listRow(c, p.Name, false, caption, func() {
				ui.Text(c, p.Version).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			})
		}
		if shown == 0 {
			cardNote(c, locales.NoGlobalPackages())
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

func hasPackage(in inventory.Installation, name string) bool {
	for _, p := range in.Packages {
		if p.Name == name {
			return true
		}
	}
	return false
}

func isLTS(nv *versions.Node, version string, now time.Time) bool {
	ch, _ := channel(nv, version, now)
	return strings.HasPrefix(ch, "LTS")
}

// releaseNote says how release r stands to the installed versions: the
// one installed, an update for an older one of its line, or, for a line
// in maintenance, until when it is supported.
func releaseNote(r versions.Release, line versions.Line, installed []string, now time.Time) string {
	newest := ""
	for _, v := range installed {
		switch {
		case v == r.Version:
			return locales.NoteInstalled()
		case versions.Major(v) == versions.Major(r.Version) && (newest == "" || semver.Compare("v"+v, "v"+newest) > 0):
			newest = v
		}
	}
	switch {
	case newest != "":
		return locales.NoteUpdateFor("v" + newest)
	case !line.Maintenance.IsZero() && !now.Before(line.Maintenance) && !line.End.IsZero():
		return locales.NoteSupportUntil(line.End)
	}
	return ""
}

// pathCard shows how the terminal finds node: the copies in its PATH in
// order, as which -a lists them, the first used and the rest shadowed;
// and a warning when apps started outside the terminal run another one.
func pathCard(c *ui.Context, ins []inventory.Installation, active int, home string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(10).Padding(14).Radius(12).Background(t.Background).
		BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("terminal")).Size(15, 15).TextColor(t.TextMuted)
			ui.Text(c, locales.HowTerminalFinds("node")).FontSize(13.5).FontWeight(600).SingleLine()
		})
		entries := pathOrder(ins)
		ui.Column(c).Gap(2).Padding(10, 6, 6, 6).Radius(9).Background(pal.Subtle).
			BorderWidth(1).BorderColor(pal.Line).Font(FontMono).Children(func() {
			ui.Row(c).Gap(6).Padding(0, 0, 4, 10).Children(func() {
				ui.Text(c, "$").FontSize(12).TextColor(pal.TextFaint)
				ui.Text(c, "which -a node").FontSize(12)
			})
			if len(entries) == 0 {
				ui.Text(c, locales.PathEmpty("node")).Font(FontSans).FontSize(12).TextColor(t.TextMuted).Padding(7, 10)
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
		if warning := guiWarning(ins, active); warning != "" {
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
// version of node than the terminal, which one; "" when they agree.
func guiWarning(ins []inventory.Installation, active int) string {
	if active < 0 {
		return ""
	}
	gui := runIndex(ins, func(in inventory.Installation) inventory.PathRank { return in.InGUI })
	switch {
	case gui < 0:
		return locales.GuiNotFound("node")
	case ins[gui].Version != ins[active].Version:
		return locales.GuiRunsOther(sourceName(ins[gui].Source), versionText(ins[gui].Version), versionText(ins[active].Version))
	}
	return ""
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

// terminalIndex returns the installation the terminal runs, or -1 for
// none.
func terminalIndex(ins []inventory.Installation) int {
	return runIndex(ins, func(in inventory.Installation) inventory.PathRank { return in.OnPath })
}

// runIndex returns the installation a PATH runs, by each one's rank in
// it, or -1 for none: the first, or when that is a shim, the version its
// manager picks by default.
func runIndex(ins []inventory.Installation, rank func(inventory.Installation) inventory.PathRank) int {
	for i, in := range ins {
		if rank(in) != 1 {
			continue
		}
		if !in.Source.Shim {
			return i
		}
		for j, d := range ins {
			if d.Default && !d.Source.Shim && d.Source.Manager == in.Source.Manager {
				return j
			}
		}
	}
	return -1
}

// channel tells a Node version's release line at now, as "LTS · Jod" or
// "Current", and whether its support has ended, by the release schedule
// in nv. Without the schedule, even majors are taken for LTS and odd ones
// for Current, though an even one is Current for its first months.
func channel(nv *versions.Node, version string, now time.Time) (string, bool) {
	major := versions.Major(version)
	if nv != nil {
		if l, ok := nv.Lines[major]; ok {
			ended := !l.End.IsZero() && !now.Before(l.End)
			switch {
			case !l.LTS.IsZero() && !now.Before(l.LTS) && l.Codename != "":
				return "LTS · " + l.Codename, ended
			case !l.LTS.IsZero() && !now.Before(l.LTS):
				return "LTS", ended
			}
			return "Current", ended
		}
	}
	switch {
	case major < 0:
		return "", false
	case major%2 == 0:
		return "LTS", false
	}
	return "Current", false
}

// managedBy says how the versions are managed: by the version manager in
// use, if any, and where the other copies came from.
func managedBy(ins []inventory.Installation, rows []int, active int) string {
	vm := ""
	if active >= 0 && ins[active].Source.Kind == inventory.KindVersionManager {
		vm = ins[active].Source.Manager
	}
	for _, i := range rows {
		if vm == "" && ins[i].Source.Kind == inventory.KindVersionManager {
			vm = ins[i].Source.Manager
		}
	}
	names := sourceNames(ins, rows, active)
	if vm == "" {
		return locales.InstalledFrom(strings.Join(names, ", "))
	}
	var rest []string
	for _, n := range names {
		if n != vm {
			rest = append(rest, n)
		}
	}
	if len(rest) == 0 {
		return locales.ManagedBy(vm)
	}
	return locales.ManagedBy(vm) + " · " + locales.AlsoFrom(strings.Join(rest, ", "))
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
