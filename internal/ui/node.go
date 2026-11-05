package ui

import (
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
		st, ok := a.toolState("node")
		if !ok {
			a.toolEmpty(c, "node", "Node.js")
			return
		}
		rows := listed(st.Installations)
		active := inventory.TerminalCopy(st.Installations)
		channel := func(in inventory.Installation) (string, bool) { return channel(a.NodeVersions, in.Version, c.Now()) }
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				nodeHeader(c, a.NodeVersions, st.Installations, rows, active)
				toolTabs(c, "node-tabs", &a.nodeTab, len(rows))
				switch a.nodeTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.versionTable(c, st.Installations, rows, active, channel)
							pathCard(c, "node", st.Installations, active, a.Home)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.availableCard(c, st.Installations, rows)
							a.managerCard(c, st, rows, active, corepackRow)
							if active >= 0 {
								nodeGlobals(c, st.Installations[active])
							}
						})
					})
				case 1:
					a.availableCard(c, st.Installations, rows)
				case 3:
					pathCard(c, "node", st.Installations, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// nodeHeader is the logo, the name and how the versions are managed, and
// on the right the cards of where Node stands: installed, in the
// terminal, in a project.
func nodeHeader(c *ui.Context, nv *versions.Node, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	badge := func() {
		if active >= 0 && isLTS(nv, ins[active].Version, c.Now()) {
			ui.Text(c, "LTS").FontSize(10.5).FontWeight(600).TextColor(pal.SuccessText).
				Padding(2, 6).Radius(5).Background(pal.SuccessSoft)
		}
	}
	toolHead(c, "node", "Node.js", badge, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.VersionsCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), versionText(in.Version), whichCommand+" node → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
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

// corepackRow says whether Corepack is on for the Node version in use.
func corepackRow(c *ui.Context, in inventory.Installation) {
	listRow(c, "Corepack", false, locales.CorepackCaption(), func() {
		if !hasPackage(in, "corepack") && !in.Corepack {
			ui.Text(c, locales.CorepackMissing()).FontSize(12).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			return
		}
		stateText(c, in.Corepack)
	})
}

// nodeGlobals lists the packages installed globally with the Node
// version in use, but those that come with it.
func nodeGlobals(c *ui.Context, in inventory.Installation) {
	globalsCard(c, locales.GlobalPackages(), locales.NoGlobalPackages(), userPackages(in.Packages, in.Version), "npm -g · "+versionText(in.Version))
}

// userPackages are the global packages but those that come with Node
// version node: npm, and Corepack before Node 25, after which it is one
// the user installed with npm install -g.
func userPackages(all []inventory.Package, node string) []inventory.Package {
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimPrefix(node, "v"), ".", 2)[0])
	bundledCorepack := major == 0 || major < 25
	var pkgs []inventory.Package
	for _, p := range all {
		if p.Name != "npm" && (p.Name != "corepack" || !bundledCorepack) {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
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

// channel tells a Node version's release line at now, as "LTS · Jod" or
// "Current", and whether its support has ended, by the release schedule
// in nv. Without the schedule, even majors are taken for LTS and odd ones
// for Current, though an even one is Current for its first months.
func channel(nv *versions.Node, version string, now time.Time) (string, bool) {
	major := versions.Major(version)
	if nv != nil {
		if l, ok := nv.Lines[major]; ok {
			ended := l.Ended(now)
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
	if len(names) == 0 {
		return ""
	}
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
