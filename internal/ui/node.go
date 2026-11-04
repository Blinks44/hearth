package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
		rows := versions(st.Installations)
		active := terminalIndex(st.Installations)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				nodeHeader(c, st.Installations, rows, active)
				a.nodeTabs(c, len(rows))
				if a.nodeTab == 0 {
					versionTable(c, st.Installations, rows, active, a.Home)
				} else {
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
func nodeHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
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
				if active >= 0 && nodeChannel(ins[active].Version) == "LTS" {
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
func versionTable(c *ui.Context, ins []inventory.Installation, rows []int, active int, home string) {
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
			row := ui.Row(c).Gap(12).Padding(12, 12, 12, 16).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if i == active {
				row.Background(pal.ActiveRow)
			}
			row.Children(func() {
				ui.Column(c).Width(112).Shrink(0).Gap(2).Children(func() {
					ui.Text(c, versionText(in.Version)).Font(FontMono).FontSize(13.5).SingleLine()
					if ch := nodeChannel(in.Version); ch != "" {
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
					})
				})
			})
		}
	})
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

// versions returns the installations the table lists, by index: all but
// the managers' shims, which stand for another copy.
func versions(ins []inventory.Installation) []int {
	var rows []int
	for i, in := range ins {
		if !in.Source.Shim {
			rows = append(rows, i)
		}
	}
	return rows
}

// terminalIndex returns the installation the terminal runs, or -1 for
// none: the first in PATH, or when that is a shim, the version its
// manager picks by default.
func terminalIndex(ins []inventory.Installation) int {
	for i, in := range ins {
		if in.OnPath != 1 {
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

// nodeChannel tells a Node version's release line: even majors become
// LTS, odd ones stay Current. An even major is Current for its first six
// months too, which the release schedule will tell (S3-03).
func nodeChannel(version string) string {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	switch {
	case err != nil:
		return ""
	case n%2 == 0:
		return "LTS"
	}
	return "Current"
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
