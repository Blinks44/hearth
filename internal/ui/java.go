package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// javaPage is the Java page: the JDKs installed and their vendors, how
// the terminal finds java, JAVA_HOME, the Temurin lines available, the
// version manager and the build tools on PATH.
func (a *App) javaPage(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.installHeader(c, "java", a.javaLine(c.Now())) }})
		st, ok := a.toolState("java")
		if !ok {
			a.toolEmpty(c, "java", "Java")
			return
		}
		ins := st.Installations
		rows, active := listed(ins), inventory.TerminalCopy(ins)
		projs := a.toolProjects("java")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.javaHeader(c, ins, rows, active, projs)
				toolTabs(c, "java-tabs", &a.javaTab, len(rows), len(projs))
				switch a.javaTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, javaChannel)
						toolNote(c, locales.JavaHomeNote())
						a.javaPath(c, st, active)
					}, func() {
						a.javaAvailable(c, st)
						a.managerCard(c, st, rows, active, nil)
						listCard(c, locales.JavaBuilders(), "", func() {
							shown := 0
							for _, b := range []struct{ id, cmd string }{{"gradle", "gradle"}, {"maven", "mvn"}} {
								if path := st.Vars[b.cmd]; path != "" {
									listRow(c, b.cmd, false, tilde(path, a.Home), func() {
										a.pathActions(c, "java-"+b.cmd, path)
									})
									shown++
								} else if a.canInstallTool(b.id) {
									listRow(c, setupTitle(b.id), false, "", func() { a.installToolButton(c, b.id) })
									shown++
								}
							}
							if shown == 0 {
								cardNote(c, locales.JavaNoBuilders())
							}
						})
					})
				case 1:
					a.javaAvailable(c, st)
				case 2:
					a.toolProjectsTab(c, "java", st, projs)
				case 3:
					a.javaPath(c, st, active)
				}
			})
		})
	})
}

// javaHome is the JDK JAVA_HOME names in the terminal, symlinks resolved
// when it exists; "" when unset.
func (a *App) javaHome() string {
	if a.Snapshot != nil {
		if st, ok := a.Snapshot.Tool("java"); ok {
			if v := st.Vars["JAVA_HOME_REAL"]; v != "" {
				return v
			}
			return st.Vars["JAVA_HOME"]
		}
	}
	return ""
}

// javaHeader is Java's logo, name and how its versions are managed, and
// the cards of where it stands: installed, in the terminal, in a project.
func (a *App) javaHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int, projs []toolProject) {
	pal := Colors(c.Theme())
	toolHead(c, "java", "Java", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.JavaCount(len(rows)), strings.Join(sourceNames(ins, rows, -1), " · "))
		v, via := "—", locales.NotInPath()
		if active >= 0 {
			v, via = copyVersion(ins[active]), "java → "+sourceName(ins[active].Source)
		}
		stateCard(c, c.Theme().Success, locales.InTerminal(), v, via)
		a.toolProjectCard(c, "java", projs)
	})
}

// javaChannel is a JDK's vendor, for the version table.
func javaChannel(in inventory.Installation) (string, bool) {
	if in.Java != nil {
		return in.Java.Vendor, false
	}
	return "", false
}

// javaPath is how the terminal finds java, then JAVA_HOME and whether it
// names a different JDK.
func (a *App) javaPath(c *ui.Context, st inventory.ToolState, active int) {
	pathCard(c, "java", st.Installations, active, a.Home)
	listCard(c, "JAVA_HOME", "", func() {
		home := st.Vars["JAVA_HOME"]
		if home == "" {
			cardNote(c, locales.JavaHomeUnset())
			return
		}
		listRow(c, tilde(home, a.Home), true, "", func() {})
		if in := st.Installations; active >= 0 && in[active].Java != nil && in[active].Java.Home != a.javaHome() {
			ui.Column(c).Padding(0, 10, 10, 10).Children(func() { warningRow(c, locales.JavaHomeMismatch()) })
		}
	})
}

// javaAvailable lists Temurin's supported lines; other vendors have
// their own support policies.
func (a *App) javaAvailable(c *ui.Context, st inventory.ToolState) {
	listCard(c, locales.AvailableTitle(), a.availableAside(st, "Temurin"), func() {
		if a.JavaVersions == nil {
			text := locales.VersionsLoading()
			if a.JavaVersionsErr != "" {
				text = locales.VersionsFailed(a.JavaVersionsErr)
			}
			cardNote(c, text)
			return
		}
		for _, l := range a.JavaVersions.Lines {
			if l.Latest == "" || l.Ended(c.Now()) || !l.Release.IsZero() && c.Now().Before(l.Release) {
				continue
			}
			listRow(c, "Temurin "+l.Latest, true, locales.JavaReleaseLine(l.Cycle), func() { a.installButton(c, st, l.Cycle, "Temurin "+l.Latest) })
		}
	})
}

// javaLine is the newest Temurin line out and still supported, which
// Java installs by.
func (a *App) javaLine(now time.Time) string {
	if v := a.JavaVersions.Newest(now); v != "" {
		if l, ok := a.JavaVersions.Line(v); ok {
			return l.Cycle
		}
	}
	return ""
}
