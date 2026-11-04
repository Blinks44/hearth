package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

func phpSnapshot() *inventory.Snapshot {
	return &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "php", Installations: []inventory.Installation{
			{Tool: "php", Version: "8.4.3", Path: "/opt/homebrew/bin/php", RealPath: "/opt/homebrew/Cellar/php/8.4.3/bin/php", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}, OnPath: 1, InGUI: 1, PHP: &inventory.PHPConfig{Files: []string{"/opt/homebrew/etc/php/8.4/php.ini"}, Extensions: []string{"Core", "intl", "redis", "xdebug"}}},
			{Tool: "php", Version: "8.3.16", Path: home + "/Library/Application Support/Herd/bin/php", RealPath: home + "/Library/Application Support/Herd/bin/php83", Source: inventory.Source{Kind: inventory.KindInstaller, Manager: "Herd"}, OnPath: 2},
			{Tool: "php", Version: "8.2.27", Path: "/opt/homebrew/opt/php@8.2/bin/php", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
		}},
		{Tool: "composer", Installations: []inventory.Installation{{Tool: "composer", Version: "2.8.5", OnPath: 1, Packages: []inventory.Package{{Name: "laravel/installer", Version: "5.11.2", Commands: []string{"laravel"}}, {Name: "friendsofphp/php-cs-fixer", Version: "3.68.1", Commands: []string{"php-cs-fixer"}}}}}},
	}}
}

func openPHP(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/php")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func TestPHPPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = phpSnapshot()
	a.PHPVersions = &versions.PHP{Lines: []versions.PHPLine{{Cycle: "8.5", Latest: "8.5.0RC4", Release: time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)}, {Cycle: "8.4", Latest: "8.4.4"}, {Cycle: "8.3", Latest: "8.3.17"}}}
	tt := openPHP(a)
	for _, text := range []string{"3 copies", "8.4.3", "8.3.16", "8.2.27", "duplicate in PATH", "How the terminal finds php", "Configuration · 8.4.3", "/opt/homebrew/etc/php/8.4/php.ini", "Extensions · 4", "Composer · 2.8.5", "laravel/installer", "5.11.2", "8.4.4"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q; texts %q", text, tt.Texts())
		}
	}
	// A line not released yet is not offered; mise manages none of these.
	if tt.HasText("8.5.0RC4") || !tt.HasText("Version manager") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Configuration · 8.4.3") || !tt.HasText("which -a php") {
		t.Fatalf("PATH tab texts %q", tt.Texts())
	}
}

func TestPHPConfigFailure(t *testing.T) {
	a := NewApp()
	a.Snapshot = phpSnapshot()
	a.Snapshot.Tools[0].Installations[0].PHP = &inventory.PHPConfig{Err: "timeout"}
	tt := openPHP(a)
	if !tt.HasText("Could not read the PHP CLI configuration.") || tt.HasText("Extensions · 0") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestPHPNotInstalled(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "php"}}}
	tt := openPHP(a)
	if !tt.HasText("PHP is not installed") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
