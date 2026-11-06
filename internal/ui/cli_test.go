package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// mise's tools without a page show in one table: the version that runs,
// else the default, and the others.
func TestCliPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	cp := func(name, version string, def bool, term inventory.PathRank) inventory.Installation {
		root := home + "/.local/share/mise/installs/" + name + "/" + version
		return inventory.Installation{Tool: name, Version: version, Path: root, Default: def, OnPath: term,
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: root}}
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "mise", MiseTools: []inventory.MiseTool{
		{Name: "deno", Installations: []inventory.Installation{cp("deno", "2.1.4", true, 1)}},
		{Name: "elixir", Installations: []inventory.Installation{cp("elixir", "1.17.3", false, 0)}},
		{Name: "zig", Installations: []inventory.Installation{cp("zig", "0.13.0", true, 0), cp("zig", "0.12.0", false, 0)}},
	}}}}
	a.router.Push(cliPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cli")
	hasTexts(t, tt, "CLI & clouds", "Tools · command line", "FROM MISE · NO PAGE OF THEIR OWN",
		"deno", "mise · in PATH", "2.1.4", "elixir", "mise · not set up", "zig", "mise · the default version · 1 more version: 0.12.0", "0.13.0",
		"mise's folders", "installs/ without running mise")
	a.cliFilter = "zi"
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("deno") || !tt.HasText("zig") {
		t.Errorf("filter: %q", tt.Texts())
	}
}
