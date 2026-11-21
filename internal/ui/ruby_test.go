package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
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

func TestRubyBundler(t *testing.T) {
	// Installed Bundler shows its path and gem version.
	a := NewApp()
	a.Shell = &fakeShell{}
	a.Snapshot = rubySnapshot()
	a.Snapshot.Tools[0].Vars = map[string]string{"bundle": home + "/.rbenv/shims/bundle"}
	tt := openRuby(a)
	screenshot(t, tt, "ruby-bundler")
	hasTexts(t, tt, "Bundler", "2.6.2", "bundle", "~/.rbenv/shims/bundle")

	// Missing Bundler offers Install… through mise's gem:bundler package.
	a2 := NewApp()
	a2.Snapshot = rubySnapshot()
	a2.Snapshot.Tools[0].Managers = append(a2.Snapshot.Tools[0].Managers, inventory.Manager{Name: "mise", Version: "2025.1.6", Activated: true})
	a2.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}, nil))
	}
	tt2 := openRuby(a2)
	screenshot(t, tt2, "ruby-bundler-missing")
	hasTexts(t, tt2, "Bundler", locales.NotInPath(), "Install…")
	if err := tt2.Click("Install…"); err != nil {
		t.Fatal(err)
	}
	tt2.Advance(time.Second)
	if !tt2.HasText("mise use --global gem:bundler") {
		t.Fatalf("expected bundler install plan: %q", tt2.Texts())
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
