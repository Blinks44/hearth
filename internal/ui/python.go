package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// python is the Python page: the tool's header with where it stands, the
// copies installed, how the terminal finds python, the versions
// available, the version manager and the tools uv and pipx installed.
func (a *App) python(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.installHeader(c, "python", a.PythonVersions.Newest(c.Now())) }})
		st, ok := a.toolState("python")
		if !ok {
			a.toolEmpty(c, "python", "Python")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pythonHeader(c, ins, rows, active)
				toolTabs(c, "python-tabs", &a.pythonTab, len(rows))
				switch a.pythonTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, a.pythonChannel(c))
						if slices.ContainsFunc(rows, func(i int) bool { return ins[i].Source.Kind == inventory.KindSystem }) {
							toolNote(c, locales.PythonSystemNote())
						}
						pathCard(c, pythonCommand, ins, active, a.Home)
					}, func() {
						a.pythonAvailable(c, st, rows, active)
						a.managerCard(c, st, rows, active, nil)
						if active >= 0 {
							globalsCard(c, locales.PythonTools(), locales.NoPythonTools(), ins[active].Packages, "uv tool · pipx")
						}
					})
				case 1:
					a.pythonAvailable(c, st, rows, active)
				case 3:
					pathCard(c, pythonCommand, ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// pythonHeader is Python's logo, name and how its versions are managed,
// and the cards of where it stands: installed, in the terminal, in a
// project.
func pythonHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "python", "Python", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(in), pythonCommand+" → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// pythonChannel tells until when a copy's line gets security fixes, and
// whether they have ended, by the release lines known.
func (a *App) pythonChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	return func(in inventory.Installation) (string, bool) {
		if a.PythonVersions == nil {
			return "", false
		}
		l, ok := a.PythonVersions.Line(in.Version)
		switch {
		case !ok || l.End.IsZero():
			return "", false
		case !c.Now().Before(l.End):
			return "", true
		}
		return locales.SupportUntilYear(strconv.Itoa(l.End.Year())), false
	}
}

// pythonAvailable lists the newest release of each line still supported,
// newest first, with how each stands to the copies installed.
func (a *App) pythonAvailable(c *ui.Context, st inventory.ToolState, rows []int, active int) {
	ins := st.Installations
	pal := Colors(c.Theme())
	pv := a.PythonVersions
	asOf := ""
	if pv != nil && !pv.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(pv.Fetched)
	}
	listCard(c, locales.AvailableTitle(), a.availableAside(st, asOf), func() {
		if pv == nil {
			text := locales.VersionsLoading()
			if a.PythonVersionsErr != "" {
				text = locales.VersionsFailed(a.PythonVersionsErr)
			}
			cardNote(c, text)
			return
		}
		now := c.Now()
		for _, l := range pv.Lines {
			if l.Latest == "" || !l.End.IsZero() && !now.Before(l.End) || !l.Release.IsZero() && now.Before(l.Release) {
				continue
			}
			caption, update := pythonReleaseNote(l.Latest, l.Cycle, ins, rows, active)
			listRow(c, l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
				a.installButton(c, st, l.Latest, l.Latest)
			})
		}
	})
}

// pythonReleaseNote says how release v of line cycle stands to the copies
// at rows: one installed, an update for the newest copy of its line
// (recommended when that is the one in use), or a newer line than any copy.
func pythonReleaseNote(v, cycle string, ins []inventory.Installation, rows []int, active int) (string, bool) {
	newest := -1
	for _, i := range rows {
		in := ins[i]
		switch {
		case in.Version == v:
			return locales.NoteInstalled(), false
		case strings.HasPrefix(in.Version, cycle+".") && (newest < 0 || semver.Compare("v"+in.Version, "v"+ins[newest].Version) > 0):
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

// setting is a manager's setting as its card shows it.
type setting struct {
	title, caption string
	on             bool
}

// managerSettings are the settings of m that change which version runs,
// as known: uv's of downloading Pythons and preferring its own.
func managerSettings(m inventory.Manager) []setting {
	var out []setting
	if v, ok := m.Settings["python-downloads"]; ok {
		out = append(out, setting{locales.UvDownloads(), locales.UvDownloadsCaption(), v == "automatic"})
	}
	if v, ok := m.Settings["python-preference"]; ok {
		out = append(out, setting{locales.UvPrefer(), locales.UvPreferCaption(), v == "managed" || v == "only-managed"})
	}
	return out
}
