package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// goSnapshot is mise's shim first in PATH, picking 1.23.4, the old
// installer's copy behind it, and a toolchain Go downloaded itself.
func goSnapshot() *inventory.Snapshot {
	mise := home + "/.local/share/mise"
	tools := []inventory.Package{{Name: "gopls", Version: "0.17.1", Module: "golang.org/x/tools/gopls"}}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "go", Installations: []inventory.Installation{
		{Tool: "go", Path: mise + "/shims/go", RealPath: mise + "/shims/go",
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: mise, Shim: true}, OnPath: 1, InGUI: 1},
		{Tool: "go", Version: "1.21.6", Path: "/usr/local/go/bin/go", RealPath: "/usr/local/go/bin/go",
			Source: inventory.Source{Kind: inventory.KindInstaller, Root: "/usr/local"}, OnPath: 2, Packages: tools},
		{Tool: "go", Version: "1.23.4", Path: mise + "/installs/go/1.23.4/bin/go", RealPath: mise + "/installs/go/1.23.4/bin/go",
			Source:  inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: mise + "/installs/go/1.23.4", Version: "1.23.4"},
			Default: true, Packages: tools},
		{Tool: "go", Version: "1.22.10", Path: home + "/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.10.darwin-arm64/bin/go",
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: detect.GoToolchain,
				Root: home + "/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.10.darwin-arm64"}, Packages: tools},
	}, Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true, Files: []string{"mise.toml", ".tool-versions"},
		Version: "2025.1.6", Shims: true, ShimsInGUI: true}},
		Vars: map[string]string{"GOTOOLCHAIN": "auto"}}}}
}

func openGo(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push("/tools/go")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The page shows mise's versions picked through its shims, the toolchain
// Go downloaded, and the old installer's copy unused behind the shims;
// the available versions as they stand to those; and mise's card.
func TestGoPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = goSnapshot()
	a.GoVersions = &versions.Go{Releases: []string{"1.24.0", "1.23.5", "1.22.10"}}
	tt := openGo(t, a)
	screenshot(t, tt, "go")
	for _, s := range []string{
		"Versions managed by mise · also from installer, go", "3 versions", "go → mise",
		"go1.23.4", "go1.22.10", "go1.21.6", "downloaded by Go", "default", "in PATH", "duplicate in PATH",
		"~/.local/share/mise/installs/go/1.23.4", "~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.10.darwin-arm64",
		"Go downloaded the toolchains marked go itself, for a toolchain line in go.mod (GOTOOLCHAIN=auto): they live in the module cache and go away with it.",
		"How the terminal finds go", whichAllCommand + " go", "~/.local/share/mise/shims/go",
		"The copy from installer (go1.21.6) is not used: " + dirText(home+"/.local/share/mise/shims/go") + " comes first in PATH.",
		"1.24.0", "new version", "1.23.5", "update for go1.23.4", "recommended", "installed",
		"Version manager · mise", "2025.1.6", "Connected through shims", "the terminal and apps both see it",
		"reads mise.toml, .tool-versions",
		"Utilities", "GOPATH/bin", "gopls", "golang.org/x/tools/gopls", "0.17.1",
	} {
		if !tt.HasText(s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
}

func TestGoReleaseNote(t *testing.T) {
	ins := goSnapshot().Tools[0].Installations
	rows := listed(ins)
	for _, tt := range []struct {
		version, want string
		update        bool
	}{
		{"1.23.5", "update for go1.23.4", true},
		{"1.21.13", "update for go1.21.6", false},
		{"1.22.10", "installed", false},
		{"1.24.0", "new version", false},
		{"1.20.14", "", false},
	} {
		got, update := goReleaseNote(tt.version, ins, rows, 2)
		if got != tt.want || update != tt.update {
			t.Errorf("goReleaseNote(%s) = %q, %v", tt.version, got, update)
		}
	}
}

func TestMiseConnectionCard(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		activated, shims, gui bool
		caption               string
	}{
		{name: "not connected", caption: "no terminal activation or shims detected"},
		{name: "GUI only", gui: true, caption: "only apps started outside the terminal see it"},
		{name: "GUI only switch", gui: true, caption: "no terminal activation or shims detected"},
		{name: "activate with GUI shims", activated: true, gui: true, caption: "terminal through activate; apps through shims"},
		// mise activate puts its shims on the terminal's PATH too.
		{name: "activate with terminal shims", activated: true, shims: true, caption: "Connected through mise activate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApp()
			a.Snapshot = goSnapshot()
			m := &a.Snapshot.Tools[0].Managers[0]
			m.Activated, m.Shims, m.ShimsInGUI = tc.activated, tc.shims, tc.gui
			m.SwitchOnCD = tc.activated || tc.shims
			m.ConfigPath = home + "/.config/mise/config.toml"
			tt := openGo(t, a)
			for _, text := range []string{tc.caption, "Global configuration", "~/.config/mise/config.toml"} {
				if !tt.HasText(text) {
					t.Errorf("missing %q; texts %q", text, tt.Texts())
				}
			}
			screenshot(t, tt, "mise-"+tc.name)
		})
	}
}
