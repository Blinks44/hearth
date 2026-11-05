package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestPackagesPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", Version: "Homebrew 4.4.20", UpdatesKnown: true, Taps: []string{"custom/tools"}, Settings: []string{"HOMEBREW_NO_AUTO_UPDATE=—"}, Packages: []inventory.SystemPackage{
			{Name: "postgresql@17", Version: "17.2", Available: "17.3", Kind: "formula", Path: "/opt/homebrew/Cellar/postgresql@17/17.2"},
			{Name: "python@3.13", Version: "3.13.1", Available: "3.13.2", Kind: "formula", Path: "/opt/homebrew/Cellar/python@3.13/3.13.1"},
			{Name: "orbstack", Version: "1.9.5", Available: "1.9.6", Kind: "cask", Path: "/opt/homebrew/Caskroom/orbstack"},
			{Name: "node", Version: "23.6.0", Kind: "formula", Path: "/opt/homebrew/Cellar/node/23.6.0"},
		}},
		{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env", Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12", Kind: "profile"}}},
		{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true},
	}}}}
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages")
	for _, text := range []string{"postgresql@17", "orbstack", "custom/tools", "node"} {
		if !tt.HasText(text) {
			t.Fatalf("no %s: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Updates"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("node") || !tt.HasText("python@3.13") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// Updates not checked show as unknown, not as none.
	if !tt.HasText("Available updates have not been checked for this manager.") || !tt.HasText("—") {
		t.Fatal(tt.Texts())
	}
	a.packageTab = 0
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("hello") {
		t.Fatal(tt.Texts())
	}
	a.packageManager = "/winget"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Packages are not listed for this manager: its list command can refresh sources from the network.") {
		t.Fatal(tt.Texts())
	}
	a.Snapshot.Tools[0].PackageManagers = nil
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("hello") {
		t.Fatal(tt.Texts())
	}
}
