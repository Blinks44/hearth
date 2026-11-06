package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// goPage is the Go page: the tool's header with where it stands, the
// versions installed, how the terminal finds go, the versions available,
// the version manager and the commands go install put in GOPATH/bin.
func (a *App) goPage(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("go")
		if !ok {
			a.toolEmpty(c, "go", "Go")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				goHeader(c, ins, rows, active)
				toolTabs(c, "go-tabs", &a.goTab, len(rows))
				switch a.goTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, goChannel)
						if slices.ContainsFunc(rows, func(i int) bool { return ins[i].Source.Manager == detect.GoToolchain }) {
							toolNote(c, locales.GoToolchainNote(st.Vars["GOTOOLCHAIN"]))
						}
						pathCard(c, "go", ins, active, a.Home)
					}, func() {
						a.goAvailable(c, ins, rows, active)
						a.managerCard(c, st, rows, active, nil)
						if active >= 0 {
							globalsCard(c, locales.GoTools(), locales.NoGoTools(), ins[active].Packages, "GOPATH/bin")
						}
					})
				case 1:
					a.goAvailable(c, ins, rows, active)
				case 3:
					pathCard(c, "go", ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// goHeader is Go's logo, name and how its versions are managed, and the
// cards of where it stands: installed, in the terminal, in a project.
func goHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "go", "Go", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.VersionsCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			via := sourceName(in.Source)
			if first := pathOrder(ins); len(first) > 0 && ins[first[0]].Source.Shim {
				via = ins[first[0]].Source.Manager
			}
			stateCard(c, t.Success, locales.InTerminal(), in.Version, "go → "+via)
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// goChannel tells how a Go copy came: a toolchain Go downloaded itself is
// marked; Go's lines have no named channels.
func goChannel(in inventory.Installation) (string, bool) {
	if in.Source.Manager == detect.GoToolchain {
		return locales.GoDownloaded(), false
	}
	return "", false
}

// goAvailable lists the newest release of each supported Go line, as
// go.dev says, with how each stands to the versions installed.
func (a *App) goAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	gv := a.GoVersions
	asOf := ""
	if gv != nil && !gv.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(gv.Fetched)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if gv == nil {
			text := locales.VersionsLoading()
			if a.GoVersionsErr != "" {
				text = locales.VersionsFailed(a.GoVersionsErr)
			}
			cardNote(c, text)
			return
		}
		for _, v := range gv.Releases {
			caption, update := goReleaseNote(v, ins, rows, active)
			listRow(c, v, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
			})
		}
	})
}

// goReleaseNote says how Go release v stands to the copies at rows: one
// installed, an update for the newest copy of its line (recommended when
// that is the one in use), or a newer line than any copy.
func goReleaseNote(v string, ins []inventory.Installation, rows []int, active int) (string, bool) {
	line := semver.MajorMinor("v" + v)
	newest := -1
	for _, i := range rows {
		in := ins[i]
		switch {
		case in.Version == v:
			return locales.NoteInstalled(), false
		case semver.MajorMinor("v"+in.Version) == line && (newest < 0 || semver.Compare("v"+in.Version, "v"+ins[newest].Version) > 0):
			newest = i
		}
	}
	switch {
	case newest < 0 && olderRelease(v, ins, rows):
		return "", false
	case newest < 0:
		return locales.NoteNewLine(), false
	case semver.Compare("v"+ins[newest].Version, "v"+v) < 0:
		return locales.NoteUpdateFor(copyVersion(ins[newest])), active >= 0 && ins[active].Version == ins[newest].Version
	}
	return "", false
}

// olderRelease reports that a copy at rows is newer than release v: v is
// then no new line to go to.
func olderRelease(v string, ins []inventory.Installation, rows []int) bool {
	return slices.ContainsFunc(rows, func(i int) bool { return semver.Compare("v"+ins[i].Version, "v"+v) > 0 })
}
