package ui

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
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
						a.versionTable(c, ins, rows, active, a.javaChannel(c))
						toolNote(c, locales.JavaHomeNote())
						a.javaPath(c, st, active)
					}, func() {
						a.javaAvailable(c, st, rows, active)
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
									listRow(c, setupTitle(b.id), false, "", func() { a.installToolIconButton(c, b.id) })
									shown++
								}
							}
							if shown == 0 {
								cardNote(c, locales.JavaNoBuilders())
							}
						})
					})
				case 1:
					a.javaAvailable(c, st, rows, active)
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
		v, via, dot := "—", locales.NotInPath(), pal.TextFaint
		if active >= 0 {
			v, via, dot = copyVersion(ins[active]), "java → "+sourceName(ins[active].Source), c.Theme().Success
		}
		stateCard(c, dot, locales.InTerminal(), v, via)
		a.toolProjectCard(c, "java", projs)
	})
}

// javaChannel returns a JDK's vendor and whether its Temurin line has
// reached end of life, for the version table.
func (a *App) javaChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	now := c.Now()
	return func(in inventory.Installation) (string, bool) {
		vendor := ""
		if in.Java != nil {
			vendor = in.Java.Vendor
		}
		ended := false
		if a.JavaVersions != nil && in.Version != "" {
			l, ok := a.JavaVersions.Line(in.Version)
			if !ok && strings.HasPrefix(in.Version, "1.") {
				l, ok = a.JavaVersions.Line(strings.TrimPrefix(in.Version, "1."))
			}
			ended = ok && l.Ended(now)
		}
		return vendor, ended
	}
}

// javaPath is how the terminal finds java, then JAVA_HOME and whether it
// names a different JDK.
func (a *App) javaPath(c *ui.Context, st inventory.ToolState, active int) {
	pathCard(c, "java", st.Installations, active, a.Home)
	listCard(c, "JAVA_HOME", "", func() {
		home := st.Vars["JAVA_HOME"]
		activeHome := ""
		if active >= 0 && st.Installations[active].Java != nil {
			activeHome = st.Installations[active].Java.Home
		}
		if home == "" {
			if activeHome != "" && a.BuildPlan != nil {
				listRow(c, locales.JavaHomeUnset(), false, tilde(activeHome, a.Home), func() {
					ui.Button(c.Key("java-home-set"), locales.EnvVarSetButton()).Shrink(0).OnClick(func() {
						a.setEnvVar("JAVA_HOME", activeHome)
					})
				})
				return
			}
			cardNote(c, locales.JavaHomeUnset())
			return
		}
		mismatch := activeHome != "" && activeHome != a.javaHome()
		listRow(c, tilde(home, a.Home), true, "", func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if mismatch && a.BuildPlan != nil {
					ui.Button(c.Key("java-home-fix"), locales.EnvVarFixButton()).Shrink(0).OnClick(func() {
						a.setEnvVar("JAVA_HOME", activeHome)
					})
				}
				a.pathActions(c, "java-home", home)
			})
		})
		if mismatch {
			ui.Column(c).Padding(0, 10, 10, 10).Children(func() { warningRow(c, locales.JavaHomeMismatch()) })
		}
	})
}

func (a *App) setEnvVar(name, value string) {
	if a.BuildPlan == nil {
		return
	}
	a.BuildPlan(func(env sysenv.Env, term ops.Terminal) (ops.Plan, error) {
		return ops.SetEnvVarPlan(name, value, env, term)
	}, a.proposeOrSay)
}

func (a *App) activeJavaHome() string {
	if a.Snapshot == nil {
		return ""
	}
	st, ok := a.Snapshot.Tool("java")
	if !ok {
		return ""
	}
	if i := inventory.TerminalCopy(st.Installations); i >= 0 && st.Installations[i].Java != nil && st.Installations[i].Java.Home != "" {
		return st.Installations[i].Java.Home
	}
	for _, in := range st.Installations {
		if in.Java != nil && in.Java.Home != "" {
			return in.Java.Home
		}
	}
	return ""
}

// javaAvailable lists Temurin's supported lines; other vendors have
// their own support policies.
func (a *App) javaAvailable(c *ui.Context, st inventory.ToolState, rows []int, active int) {
	ins := st.Installations
	pal := Colors(c.Theme())
	listCard(c, locales.AvailableTitle(), a.availableAside(st, "Temurin"), func() {
		if a.JavaVersions == nil {
			text := locales.VersionsLoading()
			if a.JavaVersionsErr != "" {
				text = locales.VersionsFailed(a.JavaVersionsErr)
			}
			cardNote(c, text)
			a.customVersionRow(c, st, "21")
			return
		}
		for _, l := range a.JavaVersions.Lines {
			if l.Latest == "" || l.Ended(c.Now()) || !l.Release.IsZero() && c.Now().Before(l.Release) {
				continue
			}
			caption := locales.JavaReleaseLine(l.Cycle)
			note, update, installed := javaReleaseNote(l.Latest, l.Cycle, ins, rows, active)
			if note != "" {
				caption += " · " + note
			}
			listRow(c, "Temurin "+l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
				if !installed {
					a.javaInstallButton(c, st, l.Cycle, "Temurin "+l.Latest)
				}
			})
		}
		a.customVersionRow(c, st, "21")
	})
}

func (a *App) javaInstallButton(c *ui.Context, st inventory.ToolState, cycle, label string) {
	manager := a.installManager(st)
	if manager == "" && a.BuildPlan == nil {
		return
	}
	if _, ok := a.versionPlan(ops.VersionInstall, st.Tool, cmp.Or(manager, "mise"), cycle, "", ""); !ok {
		return
	}
	iconButton(c.Key("install:"+cycle), "download", locales.InstallNamed(label)).
		OnClick(func() { a.installVersion(st.Tool, cycle) })
}

func javaReleaseNote(latest, cycle string, ins []inventory.Installation, rows []int, active int) (note string, update, installed bool) {
	newest := -1
	for _, i := range rows {
		v := ins[i].Version
		if v == "" {
			continue
		}
		major := javaMajor(v)
		if compareJavaVersion(v, latest) == 0 {
			return locales.NoteInstalled(), false, true
		}
		if major == cycle {
			if newest < 0 || compareJavaVersion(v, ins[newest].Version) > 0 {
				newest = i
			}
		}
	}
	if newest >= 0 && compareJavaVersion(ins[newest].Version, latest) < 0 {
		return locales.NoteUpdateFor(copyVersion(ins[newest])), active >= 0 && ins[active].Version == ins[newest].Version, false
	}
	return "", false, false
}

func javaMajor(v string) string {
	parts := javaVersionParts(v)
	if len(parts) == 0 {
		return ""
	}
	return strconv.Itoa(parts[0])
}

func compareJavaVersion(a, b string) int {
	pa, pb := javaVersionParts(a), javaVersionParts(b)
	n := max(len(pa), len(pb))
	for i := range n {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

// javaVersionParts parses Java version strings such as "21.0.13",
// "21.0.13.1+2", "1.8.0_442", and "8u442-b06" into comparable numeric parts
// (stripping build metadata after "+" or "-b").
func javaVersionParts(v string) []int {
	v, _, _ = strings.Cut(v, "+")
	if base, _, ok := strings.Cut(v, "-b"); ok {
		v = base
	}
	v, _ = strings.CutPrefix(v, "1.")
	if major, update, ok := strings.Cut(v, "u"); ok && !strings.Contains(major, ".") {
		v = major + ".0." + update
	}
	v = strings.ReplaceAll(v, "_", ".")
	var parts []int
	for seg := range strings.SplitSeq(v, ".") {
		digits := seg
		if i := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
			digits = digits[:i]
		}
		if digits == "" {
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		parts = append(parts, n)
	}
	return parts
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
