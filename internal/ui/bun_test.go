package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// bunSnapshot is Bun from the install script first in PATH, and an older
// copy from npm inside fnm's Node after it; apps do not find bun.
func bunSnapshot() *inventory.Snapshot {
	globals := []inventory.Package{
		{Name: "@biomejs/biome", Version: "1.9.4", Commands: []string{"biome"}},
		{Name: "vercel", Version: "41.4.1", Commands: []string{"vercel"}},
	}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "bun", Installations: []inventory.Installation{
		{Tool: "bun", Version: "1.2.5", Path: home + "/.bun/bin/bun", RealPath: home + "/.bun/bin/bun",
			Source:   inventory.Source{Kind: inventory.KindInstaller, Manager: inventory.ScriptInstall, Root: home + "/.bun"},
			OnPath:   1,
			Packages: globals, PathFile: home + "/.zshrc"},
		{Tool: "bun", Version: "1.1.30", Path: home + "/.local/state/fnm_multishells/48121/bin/bun",
			RealPath: home + "/.local/share/fnm/node-versions/v22.12.0/installation/lib/node_modules/bun/bin/bun.exe",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm",
				Root: home + "/.local/share/fnm/node-versions/v22.12.0/installation/lib/node_modules/bun"},
			OnPath: 2, Packages: globals},
	}}}}
}

func openBun(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push("/tools/bun")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The page shows both copies, the script's in use and npm's a duplicate
// unused behind it; the newest version is an update; the script's card
// says where it put Bun and which file adds it to PATH.
func TestBunPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = bunSnapshot()
	a.BunLatest = &versions.Latest{Version: "1.2.8"}
	tt := openBun(t, a)
	for _, s := range []string{
		"Installed by the official script in ~/.bun · also from npm", "2 copies", "script · npm", whichCommand + " bun → ~/.bun",
		"v1.2.5", "v1.1.30", "stable", "in PATH", "duplicate in PATH",
		"Bun updates itself with bun upgrade. The copy from npm lives inside a Node.js version: it is removed with npm -g and goes away with that Node version.",
		"How the terminal finds bun", whichAllCommand + " bun", "~/.bun/bin/bun", "used", "shadowed",
		"Apps started outside the terminal do not find bun, MCP servers in desktop clients for one. VS Code and JetBrains IDEs usually take the terminal's PATH themselves.",
		"The copy from npm (v1.1.30) is not used: " + dirText(home+"/.bun/bin/bun") + " comes first in PATH.",
		"Available", "v1.2.8", "stable · update for v1.2.5", "recommended", "canary",
		"Install · script", "bun.sh/install", "Install folder", "BUN_INSTALL", "~/.bun",
		"Added to PATH", "a line in ~/.zshrc", "~/.bun/bin",
		"Global packages", "bun add -g", "vercel", "commands: vercel", "41.4.1",
	} {
		if !tt.HasText(s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
	if tt.HasText(".npmrc") {
		t.Error("Bun's rows offer npm's settings")
	}
}

// With the newest version installed it is no update, and a copy not from
// the script has no script's card.
func TestBunBrew(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "bun", Installations: []inventory.Installation{
		{Tool: "bun", Version: "1.2.8", Path: "/opt/homebrew/bin/bun",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/bun/1.2.8"},
			OnPath: 1, InGUI: 1},
	}}}}
	a.BunLatest = &versions.Latest{Version: "1.2.8"}
	tt := openBun(t, a)
	for _, s := range []string{"Installed from brew", "1 copy", whichCommand + " bun → brew", "stable · installed", "No global packages"} {
		if !tt.HasText(s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
	for _, s := range []string{"recommended", "Install · script", "Bun updates itself", "is not used", "do not find bun"} {
		if tt.HasText(s) {
			t.Errorf("shows %q", s)
		}
	}
}

// Without the list of versions the card says it is coming, or why it
// could not.
func TestBunAvailableMissing(t *testing.T) {
	a := NewApp()
	a.Snapshot = bunSnapshot()
	tt := openBun(t, a)
	if !tt.HasText("Getting the version list…") {
		t.Errorf("texts %q", tt.Texts())
	}
	a = NewApp()
	a.Snapshot = bunSnapshot()
	a.BunLatestErr = "offline"
	tt = openBun(t, a)
	if !tt.HasText("Could not get the version list: offline") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestBunChannel(t *testing.T) {
	for _, tt := range []struct{ version, want string }{
		{"1.2.5", "stable"}, {"1.2.9-canary.20250401.1", "canary"}, {"", ""},
	} {
		if got, ended := bunChannel(tt.version); got != tt.want || ended {
			t.Errorf("bunChannel(%q) = %q, %v", tt.version, got, ended)
		}
	}
}
