package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

func rubySnapshot() *inventory.Snapshot {
	root := home + "/.rbenv"
	copy := func(v string) inventory.Installation {
		return inventory.Installation{Tool: "ruby", Version: v, Path: root + "/versions/" + v + "/bin/ruby", RealPath: root + "/versions/" + v + "/bin/ruby", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rbenv", Root: root + "/versions/" + v}}
	}
	first := copy("3.3.6")
	first.Default = true
	first.Packages = []inventory.Package{{Name: "bundler", Version: "2.6.2"}, {Name: "rubocop", Version: "1.70.0"}}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "ruby", Installations: []inventory.Installation{
		{Tool: "ruby", Path: root + "/shims/ruby", OnPath: 1, Source: inventory.Source{Manager: "rbenv", Shim: true}},
		first, copy("3.2.6"), {Tool: "ruby", Version: "2.6.10", Path: "/usr/bin/ruby", RealPath: "/usr/bin/ruby", OnPath: 2, InGUI: 1, Source: inventory.Source{Kind: inventory.KindSystem}},
	}, Managers: []inventory.Manager{{Name: "rbenv", Version: "1.3.0", Shims: true, SwitchOnCD: true, Files: []string{".ruby-version"}}}}}}
}

func openRuby(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/ruby")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func TestRubyPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = rubySnapshot()
	a.RubyVersions = &versions.Ruby{Lines: []versions.Cycle{{Cycle: "3.3", Latest: "3.3.7"}, {Cycle: "3.4", Latest: "3.4.1"}, {Cycle: "3.2", Latest: "3.2.7"}}}
	tt := openRuby(a)
	screenshot(t, tt, "ruby")
	for _, text := range []string{"3.3.6", "3.2.6", "2.6.10", "Version manager · rbenv", "How the terminal finds ruby", "3.3.7", "Gems · 3.3.6", "bundler", "rubocop", locales.RubySystemNote()} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("rubocop") || !tt.HasText("How the terminal finds ruby") {
		t.Fatalf("PATH %q", tt.Texts())
	}
}

func TestRubyEmpty(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "ruby"}}}
	tt := openRuby(a)
	if !tt.HasText("Ruby is not installed") {
		t.Fatalf("%q", tt.Texts())
	}
}
