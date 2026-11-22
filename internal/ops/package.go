package ops

import (
	"cmp"
	"context"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// PackageAction is what to do with packages of a system package manager.
type PackageAction int

const (
	PackageInstall PackageAction = iota
	PackageUpgrade
	PackageUninstall
)

// Reread lists a package manager's packages again, as a scan would: a
// package plan's check uses it.
type Reread func(ctx context.Context) (inventory.PackageManager, error)

// packageName is what a package's name may hold. The names go to programs
// as arguments, never through a shell; the check keeps out what no
// manager names a package with, as spaces and quotes.
var packageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+@/:~-]*$`)

// runAsScript starts the program in HEARTH_PROGRAM as the administrator,
// with the command line in HEARTH_ARGS, through UAC, and exits as it did;
// when it cannot start, with the Windows error why, as 1223 for the user
// saying no. The script is fixed: the values come in the environment.
// The program runs in a window of its own, as UAC gives no way to its
// output.
const runAsScript = `[Console]::OutputEncoding = [Text.Encoding]::UTF8; $i = New-Object System.Diagnostics.ProcessStartInfo $env:HEARTH_PROGRAM, $env:HEARTH_ARGS; $i.Verb = "runas"; $i.UseShellExecute = $true; try { $p = [System.Diagnostics.Process]::Start($i) } catch { $e = $_.Exception; while ($e.InnerException) { $e = $e.InnerException }; Write-Error $e.Message; if ($e.NativeErrorCode) { exit $e.NativeErrorCode }; exit 1 }; $p.WaitForExit(); exit $p.ExitCode`

// managerPackages are the packages of each manager that are the manager
// itself, in lower case.
var managerPackages = map[string][]string{
	"choco":   {"chocolatey"},
	"winget":  {"microsoft.appinstaller"},
	"scoop":   {"scoop"},
	"apt":     {"apt", "dpkg"},
	"dnf":     {"dnf", "rpm"},
	"zypper":  {"zypper", "rpm"},
	"pacman":  {"pacman"},
	"apk":     {"apk-tools"},
	"snap":    {"snapd"},
	"flatpak": {"flatpak"},
}

// PackageManaged reports that Hearth changes packages through the
// manager id, as inventory.PackageManager names it.
func PackageManaged(id string) bool {
	return slices.Contains([]string{"brew", "winget", "choco", "scoop", "apt", "dnf", "zypper", "pacman", "apk", "snap", "flatpak"}, id)
}

// PackagePlan returns the plan that does action to pkgs, packages of m,
// through m itself; ok is false when Hearth cannot do it through m.
func PackagePlan(action PackageAction, m inventory.PackageManager, pkgs []inventory.SystemPackage, reread Reread) (p Plan, ok bool) {
	if len(pkgs) == 0 || slices.ContainsFunc(pkgs, func(pkg inventory.SystemPackage) bool { return !packageName.MatchString(packageRef(m.ID, pkg)) }) {
		return Plan{}, false
	}
	// A manager does not remove itself: what would update and remove the
	// rest is gone.
	if action == PackageUninstall && slices.ContainsFunc(pkgs, func(pkg inventory.SystemPackage) bool {
		return slices.Contains(managerPackages[m.ID], strings.ToLower(pkg.Name))
	}) {
		return Plan{}, false
	}
	steps, notes := packageSteps(action, m, pkgs)
	if len(steps) == 0 {
		return Plan{}, false
	}
	p.Steps, p.Notes = steps, notes
	if slices.ContainsFunc(steps, func(s Step) bool { return s.Admin }) {
		p.Notes = append(p.Notes, AdminNote())
	}
	name := pkgs[0].Name
	switch action {
	case PackageInstall:
		p.Title, p.Why = locales.PkgInstallTitle(name), locales.PkgInstallWhy(m.Name)
	case PackageUpgrade:
		p.Title, p.Why = locales.PkgUpgradeTitle(name), locales.PkgUpgradeWhy(m.Name)
		if len(pkgs) > 1 {
			p.Title = locales.PkgUpgradeMany(len(pkgs))
		}
	case PackageUninstall:
		p.Title, p.Why = locales.PkgUninstallTitle(name), locales.PkgUninstallWhy(m.Name)
	}
	for _, pkg := range pkgs {
		switch action {
		case PackageInstall:
			p.Changes = append(p.Changes, "+ "+pkg.Name+" · "+m.Name)
		case PackageUpgrade:
			p.Changes = append(p.Changes, pkg.Name+" "+pkg.Version+" → "+cmp.Or(pkg.Available, "?")+" · "+m.Name)
		case PackageUninstall:
			p.Changes = append(p.Changes, "− "+pkg.Name+" "+pkg.Version+" · "+m.Name)
		}
	}
	if reread != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			after, err := reread(ctx)
			if err != nil {
				return "", err
			}
			return checkPackages(action, m.ID, pkgs, after)
		}
	}
	return p, true
}

// PackageRefreshPlan returns the plan that refreshes m's local package index
// without installing or upgrading packages; ok is false for managers without a
// safe separate index refresh (e.g. pacman, choco, snap, nix).
func PackageRefreshPlan(m inventory.PackageManager, reread Reread) (Plan, bool) {
	if m.ID != "winget" && m.Path == "" {
		return Plan{}, false
	}
	steps := packageRefreshSteps(m)
	if len(steps) == 0 {
		return Plan{}, false
	}
	name := cmp.Or(m.Name, m.ID)
	p := Plan{
		Title:   locales.PkgRefreshTitle(name),
		Why:     locales.PkgRefreshWhy(name),
		Steps:   steps,
		Changes: []string{locales.PkgRefreshChange(name)},
	}
	if slices.ContainsFunc(steps, func(s Step) bool { return s.Admin }) {
		p.Notes = append(p.Notes, AdminNote())
	}
	if reread != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			after, err := reread(ctx)
			if err != nil {
				return "", err
			}
			updates := 0
			for _, pkg := range after.Packages {
				if pkg.Available != "" {
					updates++
				}
			}
			if updates == 0 {
				return locales.PkgRefreshUpToDate(name), nil
			}
			return locales.PkgRefreshChecked(name, updates), nil
		}
	}
	return p, true
}

func packageRefreshSteps(m inventory.PackageManager) []Step {
	sibling := func(name string) string {
		if path.IsAbs(m.Path) {
			return path.Join(path.Dir(m.Path), name)
		}
		return name
	}
	switch m.ID {
	case "brew":
		return []Step{{Argv: []string{m.Path, "update"}}}
	case "apt":
		aptGet := sibling("apt-get")
		return []Step{{
			Argv:  []string{aptGet, "-oDPkg::Lock::Timeout=600", "update"},
			Runs:  []string{aptGet, "update"},
			Admin: true,
		}}
	case "dnf":
		return []Step{{Argv: []string{m.Path, "makecache"}, Admin: true}}
	case "zypper":
		return []Step{{Argv: []string{m.Path, "--non-interactive", "refresh"}, Admin: true}}
	case "apk":
		return []Step{{Argv: []string{m.Path, "update"}, Admin: true}}
	case "scoop":
		return []Step{{Argv: []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", m.Path, "update"}}}
	case "winget":
		return []Step{{Argv: []string{"winget", "source", "update", "--disable-interactivity"}}}
	case "flatpak":
		return []Step{{Argv: []string{m.Path, "update", "--appstream", "-y", "--noninteractive"}}}
	}
	return nil
}

// packageRef is how m's commands name pkg: Flatpak by its full ref, the
// others by name.
func packageRef(manager string, pkg inventory.SystemPackage) string {
	if manager == "flatpak" && pkg.Reference != "" {
		return pkg.Reference
	}
	return pkg.Name
}

// packageSteps are the commands that do action to pkgs through m, and
// what to know of them; none when Hearth cannot.
func packageSteps(action PackageAction, m inventory.PackageManager, pkgs []inventory.SystemPackage) ([]Step, []Note) {
	names := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		names[i] = packageRef(m.ID, pkg)
	}
	// sibling is a program installed beside the manager's, as apt-get
	// beside apt: a full path, as the administrator's runs with a PATH of
	// its own. Only Linux's managers have them, so the path has slashes.
	sibling := func(name string) string {
		if path.IsAbs(m.Path) {
			return path.Join(path.Dir(m.Path), name)
		}
		return name
	}
	admin := func(argv ...string) ([]Step, []Note) { return []Step{{Argv: append(argv, names...), Admin: true}}, nil }
	switch m.ID {
	case "brew":
		// Formulas and casks take a flag each: a cask may share a
		// formula's name.
		verb := [...]string{"install", "upgrade", "uninstall"}[action]
		var steps []Step
		for _, kind := range []string{"formula", "cask"} {
			var these []string
			for _, pkg := range pkgs {
				if pkg.Kind == kind || kind == "formula" && pkg.Kind == "" {
					these = append(these, pkg.Name)
				}
			}
			if len(these) > 0 {
				// A cask's installer or uninstaller may run sudo, which
				// Homebrew points to SUDO_ASKPASS.
				steps = append(steps, Step{Argv: append([]string{m.Path, verb, "--" + kind}, these...), Askpass: kind == "cask"})
			}
		}
		var notes []Note
		if slices.ContainsFunc(steps, func(s Step) bool { return s.Askpass }) {
			notes = []Note{{Title: locales.NoteAdmin(), Text: locales.NoteCaskAdmin(), Tag: "sudo"}}
		}
		return steps, notes
	case "winget":
		var steps []Step
		for _, pkg := range pkgs {
			// WinGet is the app alias on PATH, as the terminal runs it.
			argv := []string{"winget", [...]string{"install", "upgrade", "uninstall"}[action], "--id", pkg.Name, "--exact"}
			source := pkg.Origin
			if action == PackageInstall && source == "" {
				source = "winget"
			}
			if source != "" {
				argv = append(argv, "--source", source)
			}
			if action != PackageUninstall {
				argv = append(argv, "--accept-package-agreements", "--accept-source-agreements")
			}
			steps = append(steps, Step{Argv: append(argv, "--disable-interactivity")})
		}
		// A package installed for every user, as Git, asks through UAC
		// itself; which do is the package's to say.
		var notes []Note
		if action != PackageUninstall {
			notes = []Note{{Title: locales.NoteAdmin(), Text: locales.NoteWingetUAC(), Tag: "UAC"}}
		}
		return steps, notes
	case "choco":
		// Chocolatey changes C:\ProgramData and does not ask for the
		// administrator itself.
		verb := [...]string{"install", "upgrade", "uninstall"}[action]
		return []Step{runAs(m.Path, append([]string{verb}, append(names, "-y", "--no-progress")...))}, nil
	case "scoop":
		verb := [...]string{"install", "update", "uninstall"}[action]
		script := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", m.Path, verb}
		if slices.ContainsFunc(pkgs, func(pkg inventory.SystemPackage) bool { return pkg.Scope == "global" }) {
			// Global apps are the administrator's: all of them or none.
			if slices.ContainsFunc(pkgs, func(pkg inventory.SystemPackage) bool { return pkg.Scope != "global" }) {
				return nil, nil
			}
			return []Step{runAs("powershell.exe", append(append(script, "--global"), names...))}, nil
		}
		return []Step{{Argv: append(append([]string{"powershell.exe"}, script...), names...)}}, nil
	case "apt":
		// A new Ubuntu updates itself in the background for a while: apt
		// waits for dpkg's lock instead of failing at once. The package
		// lists of a computer that has not updated them in a while name
		// versions the mirrors no longer keep: apt-get update reads them
		// first, in the same command, for one password. The plan shows
		// the commands without the lock's option.
		waiting := func(argv ...string) ([]Step, []Note) {
			lock := "-oDPkg::Lock::Timeout=600"
			script := argv[0] + " " + lock + " update && " + strings.Join(append(append([]string{argv[0], lock}, argv[1:]...), names...), " ")
			runs := append(append([]string{argv[0], "update", "&&"}, argv...), names...)
			return []Step{{Argv: []string{"/bin/sh", "-c", script}, Runs: runs, Admin: true}}, nil
		}
		switch action {
		case PackageInstall:
			return waiting(sibling("apt-get"), "install", "-y")
		case PackageUpgrade:
			return waiting(sibling("apt-get"), "install", "--only-upgrade", "-y")
		}
		// dpkg refuses to remove what other packages depend on, where
		// apt-get would remove them too.
		return admin(sibling("dpkg"), "--remove")
	case "dnf", "zypper":
		switch {
		case action == PackageUninstall:
			// rpm refuses, as dpkg does, where dnf and zypper would take
			// the dependent packages along.
			return admin(sibling("rpm"), "--erase")
		case m.ID == "dnf":
			return admin(m.Path, [...]string{"install", "upgrade"}[action], "-y")
		}
		return admin(m.Path, "--non-interactive", [...]string{"install", "update"}[action])
	case "pacman":
		switch action {
		case PackageInstall:
			return admin(m.Path, "-S", "--needed", "--noconfirm")
		case PackageUpgrade:
			// Arch upgrades the whole system or nothing: one package
			// alone may break those that share its libraries.
			return []Step{{Argv: []string{m.Path, "-Syu", "--noconfirm"}, Admin: true}},
				[]Note{{Title: locales.PkgPacmanNote(), Text: locales.PkgPacmanNoteText(), Tag: "pacman -Syu"}}
		}
		return admin(m.Path, "-R", "--noconfirm")
	case "apk":
		return admin(m.Path, [...]string{"add", "upgrade", "del"}[action])
	case "snap":
		return admin(m.Path, [...]string{"install", "refresh", "remove"}[action])
	case "flatpak":
		if action == PackageInstall {
			return nil, nil // which remote is the user's choice
		}
		var steps []Step
		for _, pkg := range pkgs {
			// Flatpak asks polkit itself for a system installation.
			argv := []string{m.Path, [...]string{"", "update", "uninstall"}[action], "-y", "--noninteractive", flatpakInstallation(pkg.Scope)}
			steps = append(steps, Step{Argv: append(argv, packageRef(m.ID, pkg))})
		}
		return steps, nil
	}
	return nil, nil
}

// flatpakInstallation is the flag that picks scope, a package's
// installation as flatpak list names it.
func flatpakInstallation(scope string) string {
	switch scope {
	case "", "system":
		return "--system"
	case "user":
		return "--user"
	}
	return "--installation=" + scope
}

// runAs is the step that runs program with args as the administrator on
// Windows, asking through UAC.
func runAs(program string, args []string) Step {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = windowsArg(a)
	}
	// Admin marks it for the user: Windows runs a step as it is, and the
	// script asks for the administrator itself.
	return Step{
		Argv:  []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", runAsScript},
		Env:   []string{"HEARTH_PROGRAM=" + program, "HEARTH_ARGS=" + strings.Join(quoted, " ")},
		Runs:  append([]string{program}, args...),
		Note:  locales.PkgRunAsStep(),
		Admin: true,
	}
}

// windowsArg quotes a for a Windows command line, as programs split it:
// backslashes are literal but before a quote.
func windowsArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range a {
		switch r {
		case '\\':
			slashes++
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, 2*slashes+1))
		default:
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, 2*slashes))
	b.WriteByte('"')
	return b.String()
}

// checkPackages says whether after, the manager read again once the plan
// ran, shows action done to pkgs.
func checkPackages(action PackageAction, manager string, pkgs []inventory.SystemPackage, after inventory.PackageManager) (string, error) {
	for _, pkg := range pkgs {
		i := slices.IndexFunc(after.Packages, func(p inventory.SystemPackage) bool {
			return strings.EqualFold(p.Name, pkg.Name) && (manager != "flatpak" || p.Scope == pkg.Scope) &&
				(pkg.Architecture == "" || p.Architecture == pkg.Architecture)
		})
		switch {
		case action == PackageInstall && i < 0:
			return "", errors.New(locales.PkgCheckNotInstalled(pkg.Name, after.Name))
		case action == PackageUpgrade && i < 0:
			return "", errors.New(locales.PkgCheckGone(pkg.Name, after.Name))
		case action == PackageUpgrade && after.Packages[i].Version == pkg.Version:
			return "", errors.New(locales.PkgCheckNotUpgraded(pkg.Name, pkg.Version))
		case action == PackageUninstall && i >= 0:
			return "", errors.New(locales.PkgCheckNotRemoved(pkg.Name, after.Name))
		}
	}
	names := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		names[i] = pkg.Name
	}
	list := strings.Join(names, ", ")
	switch action {
	case PackageInstall:
		return locales.PkgCheckInstalled(list), nil
	case PackageUpgrade:
		return locales.PkgCheckUpgraded(list), nil
	}
	return locales.PkgCheckRemoved(list), nil
}
