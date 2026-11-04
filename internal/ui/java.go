package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// javaPage is the Java page: the JDKs installed and their vendors, how
// the terminal finds java, JAVA_HOME, the Temurin lines available, the
// version manager and the build tools on PATH.
func (a *App) javaPage(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("java")
		if !ok {
			a.toolEmpty(c, "java", "Java")
			return
		}
		ins := st.Installations
		rows, active := listed(ins), inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.javaHeader(c, ins, rows, active)
				toolTabs(c, "java-tabs", &a.javaTab, len(rows))
				switch a.javaTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.versionTable(c, ins, rows, active, javaChannel)
							toolNote(c, locales.JavaHomeNote())
							a.javaPath(c, st, active)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.javaAvailable(c)
							managerCard(c, st, rows, active, nil)
							listCard(c, locales.JavaBuilders(), "", func() {
								found := false
								for _, cmd := range []string{"gradle", "mvn"} {
									if path := st.Vars[cmd]; path != "" {
										listRow(c, cmd, false, tilde(path, a.Home), func() {})
										found = true
									}
								}
								if !found {
									cardNote(c, locales.JavaNoBuilders())
								}
							})
						})
					})
				case 1:
					a.javaAvailable(c)
				case 3:
					a.javaPath(c, st, active)
				default:
					comingSoon(c)
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
func (a *App) javaHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	toolHead(c, "java", "Java", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.JavaCount(len(rows)), strings.Join(sourceNames(ins, rows, -1), " · "))
		v, via := "—", locales.NotInPath()
		if active >= 0 {
			v, via = copyVersion(ins[active]), "java → "+sourceName(ins[active].Source)
		}
		stateCard(c, c.Theme().Success, locales.InTerminal(), v, via)
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
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
func (a *App) javaAvailable(c *ui.Context) {
	listCard(c, locales.AvailableTitle(), "Temurin", func() {
		if a.JavaVersions == nil {
			text := locales.VersionsLoading()
			if a.JavaVersionsErr != "" {
				text = locales.VersionsFailed(a.JavaVersionsErr)
			}
			cardNote(c, text)
			return
		}
		for _, l := range a.JavaVersions.Lines {
			if l.Latest == "" || !l.End.IsZero() && !c.Now().Before(l.End) || !l.Release.IsZero() && c.Now().Before(l.Release) {
				continue
			}
			listRow(c, "Temurin "+l.Latest, true, locales.JavaReleaseLine(l.Cycle), func() {})
		}
	})
}
