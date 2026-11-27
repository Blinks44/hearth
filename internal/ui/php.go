package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// php is the PHP page: the tool's header with where it stands, the
// copies installed, how the terminal finds php, the versions
// available, the version manager, CLI configuration and global Composer
// packages.
func (a *App) php(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		st, ok := a.toolState("php")
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if !ok {
				return
			}
			newest := a.PHPVersions.Newest(c.Now())
			if plan, can := a.phpInstall("", newest); can {
				installVersionButton(c, "php", newest, plan.Title, false, func() { a.Propose(plan) })
			}
		}})
		if !ok {
			a.toolEmpty(c, "php", "PHP")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		projs := a.toolProjects("php")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.phpHeader(c, ins, rows, active, projs)
				toolTabs(c, "php-tabs", &a.phpTab, len(rows), len(projs))
				switch a.phpTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, a.phpChannel(c))
						// Homebrew's and Herd's PHPs change outside Hearth.
						if slices.ContainsFunc(ins, func(in inventory.Installation) bool {
							return in.Source.Manager == "brew" || in.Source.Manager == "Herd"
						}) {
							toolNote(c, locales.PhpManagementNote())
						}
						pathCard(c, "php", ins, active, a.Home)
					}, func() {
						a.phpAvailable(c, ins, rows, active)
						a.managerCard(c, st, rows, active, nil)
						if active >= 0 {
							a.phpConfig(c, ins[active])
						}
						a.composerCard(c)
					})
				case 1:
					a.phpAvailable(c, ins, rows, active)
				case 2:
					a.toolProjectsTab(c, "php", st, projs)
				case 3:
					pathCard(c, "php", ins, active, a.Home)
				}
			})
		})
	})
}

// phpHeader is PHP's logo, name and how its versions are managed,
// and the cards of where it stands: installed, in the terminal, in a
// project.
func (a *App) phpHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int, projs []toolProject) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "php", "PHP", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(in), "php → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		a.toolProjectCard(c, "php", projs)
	})
}

// phpChannel marks versions whose security support has ended.
func (a *App) phpChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	return func(in inventory.Installation) (string, bool) {
		if a.PHPVersions != nil {
			if l, ok := a.PHPVersions.Line(in.Version); ok && l.Ended(c.Now()) {
				return "", true
			}
		}
		return "", false
	}
}

// phpAvailable lists the newest release of each line released and still
// supported, newest first, with how each stands to the copies installed.
func (a *App) phpAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	asOf := ""
	if a.PHPVersions != nil && !a.PHPVersions.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(a.PHPVersions.Fetched)
	}
	if m, ok := a.phpManager(); ok && phpPackage(m.ID, "8.4.0", "8.4.0") != "" {
		asOf = locales.Through(m.ID)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if a.PHPVersions == nil {
			text := locales.VersionsLoading()
			if a.PHPVersionsErr != "" {
				text = locales.VersionsFailed(a.PHPVersionsErr)
			}
			cardNote(c, text)
			return
		}
		now := c.Now()
		for _, l := range a.PHPVersions.Lines {
			if l.Latest == "" || l.Ended(now) || !l.Release.IsZero() && now.Before(l.Release) {
				continue
			}
			caption, update := goReleaseNote(l.Latest, ins, rows, active)
			listRow(c, l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
				if plan, ok := a.phpInstall(l.Latest, a.PHPVersions.Newest(now)); ok {
					iconButton(c.Key("install:"+l.Latest), "download", locales.InstallNamed(l.Latest)).OnClick(func() { a.Propose(plan) })
				}
			})
		}
	})
}

func (a *App) phpConfig(c *ui.Context, in inventory.Installation) {
	listCard(c, locales.PhpConfiguration(copyVersion(in)), "php --ini", func() {
		cfg := in.PHP
		if cfg == nil || cfg.Err != "" {
			cardNote(c, locales.PhpConfigurationFailed())
			return
		}
		if len(cfg.Files) == 0 {
			cardNote(c, locales.PhpNoIni())
		}
		for _, path := range cfg.Files {
			file := path
			listRow(c, filepath.Base(file), false, tilde(file, a.Home), func() {
				if a.canOpenFile() {
					iconButton(c.Key("php-open:"+file), "external-link", locales.PhpOpenIni()).OnClick(func() {
						a.openFile(c, file)
					})
				}
			})
		}
		listRow(c, locales.PhpExtensions(strconv.Itoa(len(cfg.Extensions))), false, strings.Join(cfg.Extensions, ", "), func() {
			iconButton(c, "copy", locales.PhpCopyExtensions()).OnClick(func() { c.WriteClipboard(strings.Join(cfg.Extensions, "\n")); c.Toast(locales.PhpExtensionsCopied()) })
		})
	})
}

func (a *App) composerCard(c *ui.Context) {
	st, ok := a.toolState("composer")
	active := inventory.TerminalCopy(st.Installations)
	if !ok || active < 0 {
		listCard(c, "Composer", "", func() {
			listRow(c, locales.ToolNotInstalled("Composer"), false, "", func() { a.installToolButton(c, "composer") })
		})
		return
	}
	in := st.Installations[active]
	a.managedGlobalsCard(c, "composer", "Composer · "+in.Version, locales.PhpNoComposerPackages(), in.Packages, "composer global")
}

// phpManagers are the package managers PHP installs with, the first
// found: mise and asdf build it from source, which takes libraries a
// computer seldom has.
var phpManagers = []string{"brew", "winget", "apt", "dnf", "pacman", "zypper"}

// phpManager is the package manager PHP installs with, as the scan found
// it, with the updates checked since: the one of the PHP in use, when it
// is one of phpManagers, else the first found.
func (a *App) phpManager() (inventory.PackageManager, bool) {
	st, ok := a.Snapshot.Tool("packages")
	if !ok {
		return inventory.PackageManager{}, false
	}
	ids := phpManagers
	if php, ok := a.toolState("php"); ok {
		if i := inventory.TerminalCopy(php.Installations); i >= 0 && slices.Contains(phpManagers, php.Installations[i].Source.Manager) {
			ids = append([]string{php.Installations[i].Source.Manager}, phpManagers...)
		}
	}
	for _, id := range ids {
		for _, m := range st.PackageManagers {
			if m.ID == id {
				if checked, ok := a.PackageUpdates[m.Path]; ok {
					m = checked
				}
				return m, true
			}
		}
	}
	return inventory.PackageManager{}, false
}

// phpPackage is the package of manager that installs the PHP line of
// release, newest being the newest release: Homebrew's php for the newest
// line, php@8.3 for the others, WinGet's PHP.PHP.8.3. Linux's managers
// have the system's one PHP, its command line alone, installed when
// release is "", as the header does. "" when manager has none.
func phpPackage(manager, release, newest string) string {
	l := phpLine(cmp.Or(release, newest))
	switch manager {
	case "brew":
		if l == "" || l == phpLine(newest) {
			return "php"
		}
		return "php@" + l
	case "winget":
		if l == "" {
			return ""
		}
		return "PHP.PHP." + l
	}
	if release != "" {
		return ""
	}
	return map[string]string{"apt": "php-cli", "dnf": "php-cli", "pacman": "php", "zypper": "php8"}[manager]
}

// phpLine is the line of a PHP version, as 8.4 of 8.4.3; "" when it has
// none.
func phpLine(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// phpInstall is the plan that installs the PHP line of release, or the
// newest line or the system's PHP when it is "", with the system's package
// manager; ok is false without one, or when a copy of the line or the
// package is installed already, from any source.
func (a *App) phpInstall(release, newest string) (ops.Plan, bool) {
	m, ok := a.phpManager()
	if !ok {
		return ops.Plan{}, false
	}
	if php, ok := a.toolState("php"); ok && slices.ContainsFunc(php.Installations, func(in inventory.Installation) bool {
		return !in.Stub && !in.Source.Shim && in.Version != "" && phpLine(in.Version) == phpLine(cmp.Or(release, newest))
	}) {
		return ops.Plan{}, false
	}
	name := phpPackage(m.ID, release, newest)
	if name == "" || slices.ContainsFunc(m.Packages, func(p inventory.SystemPackage) bool { return strings.EqualFold(p.Name, name) }) {
		return ops.Plan{}, false
	}
	p, ok := a.packagePlan(ops.PackageInstall, m, []inventory.SystemPackage{{Name: name}})
	// A package of a line is named by the release, as PHP 8.4.3.
	if v := cmp.Or(release, newest); ok && phpPackage(m.ID, "8.4.0", "8.4.0") != "" && v != "" {
		p.Title = locales.VersionInstallTitle("PHP", v)
	}
	return p, ok
}
