package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

const home = "/Users/dev"

// nodeSnapshot is fnm with two versions, the default first in PATH, and
// a Homebrew copy after it.
func nodeSnapshot() *inventory.Snapshot {
	fnm := func(v string) inventory.Source {
		return inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm",
			Root: home + "/.local/share/fnm/node-versions/v" + v, Version: v}
	}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		{Tool: "node", Version: "22.12.0", Path: home + "/.local/state/fnm_multishells/48121/bin/node",
			Source: fnm("22.12.0"), OnPath: 1, Default: true},
		{Tool: "node", Version: "23.6.0", Path: "/opt/homebrew/bin/node",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/node/23.6.0"}, OnPath: 2, InGUI: 1},
		{Tool: "node", Version: "20.18.1", Path: home + "/.local/share/fnm/node-versions/v20.18.1/installation/bin/node",
			Source: fnm("20.18.1")},
	}}}}
}

func openNode(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push("/tools/node")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The page lists both copies, fnm's and Homebrew's, and marks the one the
// terminal runs; Homebrew's, later in PATH, is a duplicate.
func TestNodeVersions(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := openNode(t, a)
	for _, s := range []string{
		"Versions managed by fnm · also from brew", "3 versions", "fnm · brew", "which node → fnm",
		"v22.12.0", "v23.6.0", "v20.18.1", "LTS", "Current",
		"~/.local/share/fnm/node-versions/v22.12.0", "/opt/homebrew/bin/node",
		"default", "in PATH", "duplicate in PATH",
	} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	// Only the active row is tinted, behind its path.
	bg := func(text string) color.RGBA {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("no %q", text)
		}
		return tt.Image().RGBAAt(int(r.X)-4, int(r.Y+r.H/2))
	}
	want := Colors(Theme(ui.LightTheme())).ActiveRow
	if got := bg("~/.local/share/fnm/node-versions/v22.12.0"); !same(got, want) {
		t.Errorf("active row %v, want %v", got, want)
	}
	if got := bg("/opt/homebrew/bin/node"); same(got, want) {
		t.Error("a row not in use is tinted")
	}
}

func same(c color.RGBA, want ui.Color) bool {
	return c.R == want.R && c.G == want.G && c.B == want.B
}

// Without a copy to show, the page says why: not checked, failed, none.
func TestNodeEmpty(t *testing.T) {
	a := NewApp()
	tt := openNode(t, a)
	if !tt.HasText("Not checked yet") {
		t.Errorf("before a scan, texts %q", tt.Texts())
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Err: "timeout"}}}
	tt.Frame()
	if !tt.HasText("Could not check Node.js: timeout") {
		t.Errorf("failed, texts %q", tt.Texts())
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node"}}}
	tt.Frame()
	if !tt.HasText("Node.js is not installed on this computer") {
		t.Errorf("none, texts %q", tt.Texts())
	}
}

func TestTerminalIndex(t *testing.T) {
	mise := func(v string, rank inventory.PathRank, def bool) inventory.Installation {
		return inventory.Installation{Version: v, OnPath: rank, Default: def,
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise"}}
	}
	shim := inventory.Installation{OnPath: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Shim: true}}
	tests := []struct {
		name string
		ins  []inventory.Installation
		want int
	}{
		{"first in PATH", []inventory.Installation{mise("20", 2, false), mise("22", 1, false)}, 1},
		{"shim picks the default", []inventory.Installation{shim, mise("20", 0, false), mise("22", 0, true)}, 2},
		{"shim without a default", []inventory.Installation{shim, mise("22", 0, false)}, -1},
		{"not in PATH", []inventory.Installation{mise("22", 0, true)}, -1},
	}
	for _, tt := range tests {
		if got := terminalIndex(tt.ins); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestManagedBy(t *testing.T) {
	src := func(k inventory.Kind, m string) inventory.Installation {
		return inventory.Installation{Source: inventory.Source{Kind: k, Manager: m}}
	}
	brew := src(inventory.KindPackageManager, "brew")
	nvm := src(inventory.KindVersionManager, "nvm")
	tests := []struct {
		ins    []inventory.Installation
		active int
		want   string
	}{
		{[]inventory.Installation{nvm, nvm}, 0, "Versions managed by nvm"},
		{[]inventory.Installation{brew, nvm}, 0, "Versions managed by nvm · also from brew"},
		{[]inventory.Installation{brew, src(inventory.KindInstaller, "")}, -1, "Installed from brew, installer"},
	}
	for _, tt := range tests {
		rows := versions(tt.ins)
		if got := managedBy(tt.ins, rows, tt.active); got != tt.want {
			t.Errorf("managedBy = %q, want %q", got, tt.want)
		}
	}
}

func TestNodeChannel(t *testing.T) {
	for v, want := range map[string]string{"22.12.0": "LTS", "23.6.0": "Current", "": "", "x": ""} {
		if got := nodeChannel(v); got != want {
			t.Errorf("nodeChannel(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestDisplayPath(t *testing.T) {
	vm := inventory.Installation{Path: home + "/.nvm/versions/node/v22.0.0/bin/node",
		Source: inventory.Source{Kind: inventory.KindVersionManager, Root: home + "/.nvm/versions/node/v22.0.0"}}
	for in, want := range map[*inventory.Installation]string{
		&vm:                              "~/.nvm/versions/node/v22.0.0",
		{Path: "/opt/homebrew/bin/node"}: "/opt/homebrew/bin/node",
		{Path: home + "x/bin/node"}:      home + "x/bin/node",
	} {
		if got := displayPath(*in, home); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", in.Path, got, want)
		}
	}
}
