package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestPackagePlanCommands(t *testing.T) {
	jq := inventory.SystemPackage{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula"}
	gh := inventory.SystemPackage{Name: "gh", Version: "2.60.0", Available: "2.63.0", Kind: "formula"}
	orb := inventory.SystemPackage{Name: "orbstack", Version: "1.9.5", Kind: "cask"}
	pkg := inventory.SystemPackage{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "package"}
	wg := inventory.SystemPackage{Name: "jqlang.jq", Version: "1.7.1", Origin: "winget"}
	portable := inventory.SystemPackage{Name: "jqlang.jq", Version: "1.7.1", Kind: "portable"}
	flat := inventory.SystemPackage{Name: "org.gimp.GIMP", Version: "3.0", Reference: "app/org.gimp.GIMP/x86_64/stable", Scope: "user"}
	flatSystem := inventory.SystemPackage{Name: "org.gimp.GIMP", Version: "3.0", Reference: "app/org.gimp.GIMP/x86_64/stable", Scope: "system"}
	scoop := inventory.SystemPackage{Name: "jq", Version: "1.7.1", Kind: "app"}
	scoopGlobal := inventory.SystemPackage{Name: "7zip", Version: "24.08", Kind: "app", Scope: "global"}
	const scoopPath = `C:\Users\Ann Lee\scoop\shims\scoop.ps1`
	scoopArgs := []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scoopPath}
	type step struct {
		argv  []string
		admin bool
	}
	tests := []struct {
		name    string
		manager string
		path    string
		action  PackageAction
		pkgs    []inventory.SystemPackage
		want    []step // nil: Hearth cannot
	}{
		{"brew install", "brew", "/opt/homebrew/bin/brew", PackageInstall, []inventory.SystemPackage{{Name: "jq"}},
			[]step{{[]string{"/opt/homebrew/bin/brew", "install", "--formula", "jq"}, false}}},
		{"brew upgrades formulas and casks apart", "brew", "/opt/homebrew/bin/brew", PackageUpgrade, []inventory.SystemPackage{jq, orb, gh}, []step{
			{[]string{"/opt/homebrew/bin/brew", "upgrade", "--formula", "jq", "gh"}, false},
			{[]string{"/opt/homebrew/bin/brew", "upgrade", "--cask", "orbstack"}, false},
		}},
		{"brew uninstall cask", "brew", "/opt/homebrew/bin/brew", PackageUninstall, []inventory.SystemPackage{orb},
			[]step{{[]string{"/opt/homebrew/bin/brew", "uninstall", "--cask", "orbstack"}, false}}},
		{"winget upgrade from its source", "winget", "winget", PackageUpgrade, []inventory.SystemPackage{wg}, []step{{[]string{
			"winget", "upgrade", "--id", "jqlang.jq", "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}, false}}},
		{"winget uninstall portable", "winget", "winget", PackageUninstall, []inventory.SystemPackage{portable}, []step{{[]string{
			"winget", "uninstall", "--id", "jqlang.jq", "--exact", "--disable-interactivity"}, false}}},
		{"winget install", "winget", "winget", PackageInstall, []inventory.SystemPackage{{Name: "jqlang.jq"}}, []step{{[]string{
			"winget", "install", "--id", "jqlang.jq", "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}, false}}},
		{"choco as the administrator", "choco", `C:\ProgramData\chocolatey\bin\choco.exe`, PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", runAsScript}, true}}},
		{"scoop update", "scoop", scoopPath, PackageUpgrade, []inventory.SystemPackage{scoop},
			[]step{{append(slices.Clone(scoopArgs), "update", "jq"), false}}},
		{"scoop global as the administrator", "scoop", scoopPath, PackageUninstall, []inventory.SystemPackage{scoopGlobal},
			[]step{{[]string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", runAsScript}, true}}},
		{"scoop global and user apart", "scoop", scoopPath, PackageUpgrade, []inventory.SystemPackage{scoop, scoopGlobal}, nil},
		{"apt upgrade", "apt", "/usr/bin/apt", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/bin/sh", "-c", "/usr/bin/apt-get -oDPkg::Lock::Timeout=600 update && /usr/bin/apt-get -oDPkg::Lock::Timeout=600 install --only-upgrade -y jq"}, true}}},
		{"apt install", "apt", "/usr/bin/apt", PackageInstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/bin/sh", "-c", "/usr/bin/apt-get -oDPkg::Lock::Timeout=600 update && /usr/bin/apt-get -oDPkg::Lock::Timeout=600 install -y jq"}, true}}},
		{"apt remove keeps dependents", "apt", "/usr/bin/apt", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/dpkg", "--remove", "jq"}, true}}},
		{"dnf upgrade", "dnf", "/usr/bin/dnf", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/dnf", "upgrade", "-y", "jq"}, true}}},
		{"dnf remove keeps dependents", "dnf", "/usr/bin/dnf", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/rpm", "--erase", "jq"}, true}}},
		{"zypper update", "zypper", "/usr/bin/zypper", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/zypper", "--non-interactive", "update", "jq"}, true}}},
		{"zypper remove keeps dependents", "zypper", "/usr/bin/zypper", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/rpm", "--erase", "jq"}, true}}},
		{"pacman updates the whole system", "pacman", "/usr/bin/pacman", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/pacman", "-Syu", "--noconfirm"}, true}}},
		{"pacman remove", "pacman", "/usr/bin/pacman", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/pacman", "-R", "--noconfirm", "jq"}, true}}},
		{"apk upgrade", "apk", "/sbin/apk", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/sbin/apk", "upgrade", "jq"}, true}}},
		{"apk del", "apk", "/sbin/apk", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/sbin/apk", "del", "jq"}, true}}},
		{"snap refresh", "snap", "/usr/bin/snap", PackageUpgrade, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/snap", "refresh", "jq"}, true}}},
		{"snap remove", "snap", "/usr/bin/snap", PackageUninstall, []inventory.SystemPackage{pkg},
			[]step{{[]string{"/usr/bin/snap", "remove", "jq"}, true}}},
		{"flatpak update by ref", "flatpak", "/usr/bin/flatpak", PackageUpgrade, []inventory.SystemPackage{flat}, []step{{[]string{
			"/usr/bin/flatpak", "update", "-y", "--noninteractive", "--user", "app/org.gimp.GIMP/x86_64/stable"}, false}}},
		{"flatpak uninstall system", "flatpak", "/usr/bin/flatpak", PackageUninstall, []inventory.SystemPackage{flatSystem}, []step{{[]string{
			"/usr/bin/flatpak", "uninstall", "-y", "--noninteractive", "--system", "app/org.gimp.GIMP/x86_64/stable"}, false}}},
		{"flatpak install needs a remote", "flatpak", "/usr/bin/flatpak", PackageInstall, []inventory.SystemPackage{flat}, nil},
		{"flatpak install with remote", "flatpak", "/usr/bin/flatpak", PackageInstall, []inventory.SystemPackage{{Name: "org.gimp.GIMP", Reference: "org.gimp.GIMP", Scope: "user", Origin: "flathub"}}, []step{{[]string{
			"/usr/bin/flatpak", "install", "-y", "--noninteractive", "--user", "flathub", "org.gimp.GIMP"}, false}}},
		{"nix", "nix", "/nix/var/nix/profiles/default/bin/nix-env", PackageUpgrade, []inventory.SystemPackage{pkg}, nil},
		{"no packages", "brew", "/opt/homebrew/bin/brew", PackageUpgrade, nil, nil},
		{"a name that is not one", "brew", "/opt/homebrew/bin/brew", PackageUninstall, []inventory.SystemPackage{{Name: "jq; rm -rf ~"}}, nil},
		{"a name like a flag", "apt", "/usr/bin/apt", PackageUninstall, []inventory.SystemPackage{{Name: "-y"}}, nil},
		{"a manager does not remove itself", "choco", `C:\ProgramData\chocolatey\bin\choco.exe`, PackageUninstall, []inventory.SystemPackage{{Name: "chocolatey"}}, nil},
		{"nor WinGet", "winget", `C:\Users\a\AppData\Local\Microsoft\WindowsApps\winget.exe`, PackageUninstall, []inventory.SystemPackage{{Name: "Microsoft.AppInstaller"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := inventory.PackageManager{ID: tt.manager, Name: tt.manager, Path: tt.path}
			p, ok := PackagePlan(tt.action, m, tt.pkgs, nil)
			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, want %v", ok, tt.want != nil)
			}
			if !ok {
				return
			}
			var got []step
			for _, s := range p.Steps {
				got = append(got, step{s.Argv, s.Admin})
			}
			if !slices.EqualFunc(got, tt.want, func(a, b step) bool { return slices.Equal(a.argv, b.argv) && a.admin == b.admin }) {
				t.Errorf("steps\n%v\nwant\n%v", got, tt.want)
			}
			if p.Title == "" || p.Why == "" || len(p.Changes) == 0 {
				t.Errorf("plan says nothing: %+v", p)
			}
			// Only casks' installers run sudo themselves.
			for _, s := range p.Steps {
				if want := slices.Contains(s.Argv, "--cask"); s.Askpass != want {
					t.Errorf("%v: askpass %v", s.Argv, s.Askpass)
				}
			}
			askpass := slices.ContainsFunc(p.Steps, func(s Step) bool { return s.Askpass })
			if noted := slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Text == locales.NoteCaskAdmin() }); noted != askpass {
				t.Errorf("cask note %v, askpass steps %v", noted, askpass)
			}
			admin := slices.ContainsFunc(p.Steps, func(s Step) bool { return s.Admin })
			if noted := slices.ContainsFunc(p.Notes, func(n Note) bool { return n == AdminNote() }); noted != admin {
				t.Errorf("admin note %v, admin steps %v", noted, admin)
			}
		})
	}
}

// Chocolatey and global Scoop run as the administrator through a fixed
// script, their command line in the environment, quoted for Windows.
func TestPackagePlanRunAs(t *testing.T) {
	m := inventory.PackageManager{ID: "scoop", Name: "Scoop", Path: `C:\Users\Ann Lee\scoop\shims\scoop.ps1`}
	p, ok := PackagePlan(PackageUpgrade, m, []inventory.SystemPackage{{Name: "7zip", Version: "24.08", Scope: "global"}}, nil)
	if !ok {
		t.Fatal("no plan")
	}
	want := []string{"HEARTH_PROGRAM=powershell.exe",
		`HEARTH_ARGS=-NoProfile -ExecutionPolicy Bypass -File "C:\Users\Ann Lee\scoop\shims\scoop.ps1" update --global 7zip`}
	if !slices.Equal(p.Steps[0].Env, want) {
		t.Errorf("env %q\nwant %q", p.Steps[0].Env, want)
	}
	if !slices.Equal(p.Steps[0].Shown(), []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", m.Path, "update", "--global", "7zip"}) {
		t.Errorf("shows %q", p.Steps[0].Shown())
	}
	// Elevation is Windows' own here: run the step as it is anywhere.
	step := p.Steps[0]
	step.Admin = false
	f := &run.Fake{}
	f.Stdout("", step.Argv...)
	Execute(t.Context(), f, Plan{Steps: []Step{step}, Env: []string{"PATH=x"}}, &strings.Builder{}, func(State) {})
	if env := f.Calls()[0].Env; !slices.Equal(env, append([]string{"PATH=x"}, want...)) {
		t.Errorf("step ran with %q", env)
	}
}

func TestWindowsArg(t *testing.T) {
	for in, want := range map[string]string{
		"jq":                 "jq",
		"":                   `""`,
		`C:\Program Files\x`: `"C:\Program Files\x"`,
		`a"b`:                `"a\"b"`,
		`C:\dir with space\`: `"C:\dir with space\\"`,
		`C:\no\space`:        `C:\no\space`,
		`say \"hi\" there`:   `"say \\\"hi\\\" there"`,
	} {
		if got := windowsArg(in); got != want {
			t.Errorf("windowsArg(%q) = %s, want %s", in, got, want)
		}
	}
}

// The check after a plan ran reads the manager again.
func TestPackagePlanVerify(t *testing.T) {
	jq := inventory.SystemPackage{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula"}
	tests := []struct {
		name   string
		action PackageAction
		after  []inventory.SystemPackage
		ok     bool
	}{
		{"updated", PackageUpgrade, []inventory.SystemPackage{{Name: "jq", Version: "1.8.2"}}, true},
		{"updated with older keg kept", PackageUpgrade, []inventory.SystemPackage{{Name: "jq", Kind: "formula", Version: "1.7.1"}, {Name: "jq", Kind: "formula", Version: "1.8.2"}}, true},
		{"still old", PackageUpgrade, []inventory.SystemPackage{{Name: "jq", Version: "1.7.1"}}, false},
		{"still old with older keg kept", PackageUpgrade, []inventory.SystemPackage{{Name: "jq", Kind: "formula", Version: "1.6.0"}, {Name: "jq", Kind: "formula", Version: "1.7.1"}}, false},
		{"same-name cask does not verify formula", PackageUpgrade, []inventory.SystemPackage{{Name: "jq", Kind: "formula", Version: "1.7.1"}, {Name: "jq", Kind: "cask", Version: "1.8.2"}}, false},
		{"gone on update", PackageUpgrade, nil, false},
		{"removed", PackageUninstall, nil, true},
		{"still there", PackageUninstall, []inventory.SystemPackage{{Name: "jq", Version: "1.7.1"}}, false},
		{"installed", PackageInstall, []inventory.SystemPackage{{Name: "jq", Version: "1.8.2"}}, true},
		{"WinGet ids differ in case", PackageInstall, []inventory.SystemPackage{{Name: "JQ", Version: "1.8.2"}}, true},
		{"not installed", PackageInstall, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
			reread := func(context.Context) (inventory.PackageManager, error) {
				after := m
				after.Packages = tt.after
				return after, nil
			}
			p, _ := PackagePlan(tt.action, m, []inventory.SystemPackage{jq}, reread)
			f := &run.Fake{}
			f.Stdout("", p.Steps[0].Argv...)
			got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
			if (got.Phase == Succeeded) != tt.ok || got.Phase != Succeeded && got.Phase != NotVerified {
				t.Errorf("phase %d, err %q; want succeeded %v", got.Phase, got.Err, tt.ok)
			}
		})
	}
}

func TestPackageRefreshPlan(t *testing.T) {
	const scoopPath = `C:\Users\Ann\scoop\shims\scoop.ps1`
	tests := []struct {
		id    string
		path  string
		argv  []string
		admin bool
		ok    bool
	}{
		{"brew", "/opt/homebrew/bin/brew", []string{"/opt/homebrew/bin/brew", "update"}, false, true},
		{"apt", "/usr/bin/apt", []string{"/usr/bin/apt-get", "-oDPkg::Lock::Timeout=600", "update"}, true, true},
		{"dnf", "/usr/bin/dnf", []string{"/usr/bin/dnf", "makecache"}, true, true},
		{"zypper", "/usr/bin/zypper", []string{"/usr/bin/zypper", "--non-interactive", "refresh"}, true, true},
		{"apk", "/sbin/apk", []string{"/sbin/apk", "update"}, true, true},
		{"scoop", scoopPath, []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scoopPath, "update"}, false, true},
		{"winget", "", []string{"winget", "source", "update", "--disable-interactivity"}, false, true},
		{"flatpak", "/usr/bin/flatpak", []string{"/usr/bin/flatpak", "update", "--appstream", "-y", "--noninteractive"}, false, true},
		{"pacman", "/usr/bin/pacman", nil, false, false},
		{"choco", `C:\ProgramData\chocolatey\bin\choco.exe`, nil, false, false},
		{"snap", "/usr/bin/snap", nil, false, false},
		{"nix", "/nix/var/nix/profiles/default/bin/nix-env", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m := inventory.PackageManager{ID: tt.id, Name: tt.id, Path: tt.path}
			p, ok := PackageRefreshPlan(m, nil)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.argv) || p.Steps[0].Admin != tt.admin {
				t.Errorf("steps = %+v, want argv=%v admin=%v", p.Steps, tt.argv, tt.admin)
			}
			if tt.admin && len(p.Notes) == 0 {
				t.Error("expected AdminNote for admin refresh plan")
			}
		})
	}

	t.Run("verify counts available updates", func(t *testing.T) {
		m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
		p, ok := PackageRefreshPlan(m, func(context.Context) (inventory.PackageManager, error) {
			return inventory.PackageManager{
				ID:   "brew",
				Name: "Homebrew",
				Packages: []inventory.SystemPackage{
					{Name: "jq", Version: "1.7", Available: "1.8"},
					{Name: "git", Version: "2.48"},
				},
			}, nil
		})
		if !ok {
			t.Fatal("expected refresh plan for brew")
		}
		got, err := p.Verify(context.Background())
		if err != nil || got != locales.PkgRefreshChecked("Homebrew", 1) {
			t.Fatalf("Verify = (%q, %v), want (%q, nil)", got, err, locales.PkgRefreshChecked("Homebrew", 1))
		}
	})

	t.Run("verify reports up to date", func(t *testing.T) {
		m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
		p, ok := PackageRefreshPlan(m, func(context.Context) (inventory.PackageManager, error) {
			return inventory.PackageManager{ID: "brew", Name: "Homebrew", UpdatesKnown: true}, nil
		})
		if !ok {
			t.Fatal("expected refresh plan for brew")
		}
		got, err := p.Verify(context.Background())
		if err != nil || got != locales.PkgRefreshUpToDate("Homebrew") {
			t.Fatalf("Verify = (%q, %v), want (%q, nil)", got, err, locales.PkgRefreshUpToDate("Homebrew"))
		}
	})

	t.Run("verify does not claim up to date when updates unknown", func(t *testing.T) {
		m := inventory.PackageManager{ID: "flatpak", Name: "Flatpak", Path: "/usr/bin/flatpak"}
		p, ok := PackageRefreshPlan(m, func(context.Context) (inventory.PackageManager, error) {
			return inventory.PackageManager{ID: "flatpak", Name: "Flatpak", UpdatesKnown: false}, nil
		})
		if !ok {
			t.Fatal("expected refresh plan for flatpak")
		}
		got, err := p.Verify(context.Background())
		if err != nil || got != locales.PkgRefreshChange("Flatpak") {
			t.Fatalf("Verify = (%q, %v), want (%q, nil)", got, err, locales.PkgRefreshChange("Flatpak"))
		}
	})
}

func TestPackageAutoremovePlan(t *testing.T) {
	orphans := []inventory.SystemPackage{
		{Name: "brotli", Version: "1.1.0", Dependency: true},
		{Name: "oniguruma", Version: "6.9.9", Dependency: true},
	}
	tests := []struct {
		id    string
		path  string
		argv  []string
		admin bool
		ok    bool
	}{
		{"brew", "/opt/homebrew/bin/brew", []string{"/opt/homebrew/bin/brew", "autoremove"}, false, true},
		{"apt", "/usr/bin/apt", []string{"/usr/bin/apt-get", "-oDPkg::Lock::Timeout=600", "autoremove", "-y"}, true, true},
		{"dnf", "/usr/bin/dnf", []string{"/usr/bin/dnf", "autoremove", "-y"}, true, true},
		{"pacman", "/usr/bin/pacman", []string{"/usr/bin/pacman", "-Rns", "--noconfirm", "brotli", "oniguruma"}, true, true},
		{"winget", "winget", nil, false, false},
		{"nix", "/nix/bin/nix", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m := inventory.PackageManager{ID: tt.id, Name: tt.id, Path: tt.path}
			p, ok := PackageAutoremovePlan(m, orphans, nil)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.argv) || p.Steps[0].Admin != tt.admin {
				t.Errorf("steps = %+v, want argv=%v admin=%v", p.Steps, tt.argv, tt.admin)
			}
			if len(p.Changes) != len(orphans) {
				t.Errorf("changes = %d, want %d", len(p.Changes), len(orphans))
			}
		})
	}
	t.Run("empty orphans returns false", func(t *testing.T) {
		m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
		if _, ok := PackageAutoremovePlan(m, nil, nil); ok {
			t.Fatal("expected false for empty orphans")
		}
	})
	t.Run("pacman rejects manager itself", func(t *testing.T) {
		m := inventory.PackageManager{ID: "pacman", Name: "Pacman", Path: "/usr/bin/pacman"}
		if _, ok := PackageAutoremovePlan(m, []inventory.SystemPackage{{Name: "pacman"}}, nil); ok {
			t.Fatal("pacman autoremove must refuse removing pacman")
		}
	})
}

func TestPackageUninstallManyTitle(t *testing.T) {
	m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	pkgs := []inventory.SystemPackage{
		{Name: "jq", Version: "1.7.1", Kind: "formula"},
		{Name: "gh", Version: "2.60.0", Kind: "formula"},
	}
	p, ok := PackagePlan(PackageUninstall, m, pkgs, nil)
	if !ok {
		t.Fatal("expected uninstall plan")
	}
	if p.Title != locales.PkgUninstallMany(2) {
		t.Fatalf("title = %q, want %q", p.Title, locales.PkgUninstallMany(2))
	}
}

func TestFlatpakDefaultScopeInstallVerify(t *testing.T) {
	m := inventory.PackageManager{ID: "flatpak", Name: "Flatpak", Path: "/usr/bin/flatpak"}
	pkg := inventory.SystemPackage{
		Name:      "org.gimp.GIMP",
		Reference: "org.gimp.GIMP",
		Kind:      "app",
		Origin:    "flathub",
	}
	p, ok := PackagePlan(PackageInstall, m, []inventory.SystemPackage{pkg}, func(context.Context) (inventory.PackageManager, error) {
		return inventory.PackageManager{
			ID:   "flatpak",
			Name: "Flatpak",
			Packages: []inventory.SystemPackage{{
				Name:      "org.gimp.GIMP",
				Version:   "2.10",
				Kind:      "app",
				Scope:     "system",
				Reference: "app/org.gimp.GIMP/x86_64/stable",
				Origin:    "flathub",
			}},
		}, nil
	})
	if !ok {
		t.Fatal("expected flatpak install plan")
	}
	if _, err := p.Verify(context.Background()); err != nil {
		t.Fatalf("Verify failed for default->system scope: %v", err)
	}
}
