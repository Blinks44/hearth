package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

// Python's versions install through uv, the manager found, though the
// terminal runs Homebrew's: from a row and from the header.
func TestPythonInstall(t *testing.T) {
	a := NewApp()
	a.Snapshot = pythonSnapshot()
	a.Shell = &fakeShell{}
	tt := openPython(t, a)
	a.PythonVersions = pythonLines(tt.Now())
	tt.Frame()
	screenshot(t, tt, "python-install")
	if !tt.HasText("through uv") {
		t.Errorf("no manager on the available card: %q", tt.Texts())
	}
	if tt.HasText("Install 3.15.0a1") {
		t.Errorf("install buttons: %q", tt.Texts())
	}
	if err := tt.Click("Install 3.13.2"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("Install Python 3.13.2") || !tt.HasText("uv python install 3.13.2") {
		t.Fatalf("no install plan: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Install 3.14.0"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("uv python install 3.14.0") {
		t.Fatalf("the header does not install the newest line: %q", tt.Texts())
	}
}

// A Ruby of rbenv is made the default and removed through rbenv, from
// its row's menu.
func TestRubyVersionMenu(t *testing.T) {
	a := NewApp()
	a.Snapshot = rubySnapshot()
	a.Shell = &fakeShell{}
	tt := openRuby(a)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "Make default") || !slices.Contains(menu, "Remove…") {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Remove Ruby 3.3.6") || !tt.HasText("rbenv uninstall -f 3.3.6") || !tt.HasText("Through rbenv. The gems of this version go with it.") {
		t.Fatalf("no remove plan: %q", tt.Texts())
	}
}

// With no copy, Go and Java install as the catalog says, through mise,
// set up first.
func TestInstallThroughMise(t *testing.T) {
	home := t.TempDir()
	mise := filepath.Join(home, "bin", "mise"+exeSuffix)
	if err := os.MkdirAll(filepath.Dir(mise), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mise, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh", "PATH=" + filepath.Dir(mise)}
	tests := []struct {
		tool, page string
		set        func(a *App)
		want       string
	}{
		{"go", "/tools/go", func(a *App) { a.GoVersions = &versions.Go{Releases: []string{"1.24.1", "1.23.7"}} }, "go@latest"},
		{"java", "/tools/java", func(a *App) {
			a.JavaVersions = &versions.Java{Lines: []versions.Cycle{{Cycle: "21", Latest: "21.0.6+7"}}}
		}, "java@temurin-25"},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			a := NewApp()
			a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: tc.tool}, wingetState}}
			a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
				done(build(env, nil))
			}
			tc.set(a)
			a.router.Push(tc.page)
			tt := newTester(a)
			if err := tt.Click("Install " + setupTitle(tc.tool)); err != nil {
				t.Fatalf("%v: %q", err, tt.Texts())
			}
			tt.Advance(time.Second)
			if a.op == nil {
				t.Fatal("no plan")
			}
			steps := a.op.plan.Steps
			if last := steps[len(steps)-1].Argv; !slices.Equal(last, []string{mise, "use", "--global", tc.want}) {
				t.Fatalf("steps %+v", steps)
			}
		})
	}
}

// PHP installs through Homebrew: its newest line is the formula php.
func TestPHPInstall(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "php", Installations: []inventory.Installation{{Tool: "php", Version: "8.3.15", Path: "/opt/homebrew/bin/php", OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/php@8.3/8.3.15"}}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
			Packages: []inventory.SystemPackage{{Name: "php@8.3", Version: "8.3.15", Path: "/opt/homebrew/Cellar/php@8.3/8.3.15"}}}}},
	}}
	a.PHPVersions = &versions.PHP{Lines: []versions.Cycle{{Cycle: "8.4", Latest: "8.4.4"}, {Cycle: "8.3", Latest: "8.3.17"}, {Cycle: "8.2", Latest: "8.2.27"}}}
	a.router.Push("/tools/php")
	tt := newTester(a)
	if tt.HasText("Install 8.3.17") || !tt.HasText("through brew") {
		t.Errorf("texts %q", tt.Texts())
	}
	for button, want := range map[string]string{"Install 8.4.4": "install --formula php", "Install 8.2.27": "install --formula php@8.2"} {
		if err := tt.Click(button); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText("/opt/homebrew/bin/brew " + want) {
			t.Fatalf("%s: %q", button, tt.Texts())
		}
		if err := tt.Click("Cancel"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
}

// newTester opens a's page in a window of the design's size.
func newTester(a *App) *ui.Tester {
	a.Home = home
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// Homebrew's Python moves to mise, set up first, from its row's menu:
// mise installs the same version and makes it the default.
func TestMoveToMise(t *testing.T) {
	dir := t.TempDir()
	mise := filepath.Join(dir, "bin", "mise"+exeSuffix)
	if err := os.MkdirAll(filepath.Dir(mise), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mise, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + dir, "SHELL=/bin/zsh", "PATH=" + filepath.Dir(mise)}
	a := NewApp()
	a.Snapshot = pythonSnapshot()
	a.Shell = &fakeShell{}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	tt := openPython(t, a)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Move to mise…"); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	tt.Frame()
	tt.Advance(time.Second)
	if a.op == nil || !tt.HasText("Move Python 3.13.1 to mise") || !tt.HasText(".venv folders made with the old copy keep running it. Create them again to use mise's Python.") {
		t.Fatalf("no plan: %q", tt.Texts())
	}
	steps := a.op.plan.Steps
	if last := steps[len(steps)-1].Argv; !slices.Equal(last, []string{mise, "use", "-g", "python@3.13.1"}) {
		t.Fatalf("steps %+v", steps)
	}
}

func TestPHPPackage(t *testing.T) {
	tests := []struct{ manager, release, newest, want string }{
		{"brew", "", "8.4.4", "php"},
		{"brew", "8.4.4", "8.4.4", "php"},
		{"brew", "8.3.17", "8.4.4", "php@8.3"},
		{"winget", "", "8.4.4", "PHP.PHP.8.4"},
		{"winget", "8.3.17", "8.4.4", "PHP.PHP.8.3"},
		{"winget", "", "", ""},
		{"apt", "", "8.4.4", "php-cli"},
		{"apt", "8.3.17", "8.4.4", ""}, // the system's one PHP, not a line
		{"dnf", "", "", "php-cli"},
		{"pacman", "", "8.4.4", "php"},
		{"zypper", "", "8.4.4", "php8"},
		{"choco", "", "8.4.4", ""},
	}
	for _, tc := range tests {
		if got := phpPackage(tc.manager, tc.release, tc.newest); got != tc.want {
			t.Errorf("%s %q %q: %q, want %q", tc.manager, tc.release, tc.newest, got, tc.want)
		}
	}
}

// On Windows PHP installs with WinGet, a package for each line; the line
// of the PHP Scoop put there is not offered again.
func TestPHPInstallWinget(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "php", Installations: []inventory.Installation{{Tool: "php", Version: "8.5.12", Path: `C:\Users\u\scoop\shims\php.exe`, OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "scoop", Root: `C:\Users\u\scoop\apps\php\8.5.12`}}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "winget", Name: "WinGet", Path: `C:\winget.exe`}}},
	}}
	a.PHPVersions = &versions.PHP{Lines: []versions.Cycle{{Cycle: "8.5", Latest: "8.5.12"}, {Cycle: "8.4", Latest: "8.4.27"}}}
	a.router.Push("/tools/php")
	tt := newTester(a)
	if tt.HasText("Install 8.5.12") || !tt.HasText("through winget") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Install 8.4.27"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("PHP.PHP.8.4") || !tt.HasText("Install PHP 8.4.27") {
		t.Fatalf("no WinGet plan: %q", tt.Texts())
	}
}

// wingetState is WinGet found, which installs mise on Windows; other
// systems install it otherwise.
var wingetState = inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "winget", Name: "WinGet", Path: "winget"}}}
