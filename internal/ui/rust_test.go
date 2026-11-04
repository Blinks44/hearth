package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

func rustSnapshot() *inventory.Snapshot {
	rustup := func(name, version, date string, def bool, targets ...string) inventory.Installation {
		root := home + "/.rustup/toolchains/" + name
		return inventory.Installation{
			Tool: "rust", Version: version, Path: root + "/bin/rustc", RealPath: root + "/bin/rustc", Default: def,
			Source:   inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Root: root},
			Rust:     &inventory.RustToolchain{Name: name, Date: date, Targets: targets},
			Packages: []inventory.Package{{Name: "ripgrep", Version: "14.1.1", Commands: []string{"rg"}}, {Name: "cargo-watch", Version: "8.5.3", Commands: []string{"cargo-watch"}}},
		}
	}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "rust",
		Installations: []inventory.Installation{
			{Tool: "rust", Path: home + "/.cargo/bin/rustc", RealPath: home + "/.cargo/bin/rustup", OnPath: 1, InGUI: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Root: home + "/.rustup", Shim: true}},
			{Tool: "rust", Version: "1.83.0", Path: "/opt/homebrew/bin/rustc", RealPath: "/opt/homebrew/Cellar/rust/1.83.0/bin/rustc", OnPath: 2, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Version: "1.83.0"}},
			rustup("stable-aarch64-apple-darwin", "1.84.0", "2025-01-07", true, "aarch64-apple-darwin", "wasm32-unknown-unknown"),
			rustup("1.79.0-aarch64-apple-darwin", "1.79.0", "2024-06-10", false),
			rustup("nightly-aarch64-apple-darwin", "1.86.0-nightly", "2025-01-20", false),
		},
		Managers: []inventory.Manager{{Name: "rustup", Version: "1.27.1", SwitchOnCD: true, Files: []string{"rust-toolchain.toml", "rust-toolchain"}, Shims: true, ShimsInGUI: true}},
	}}}
}

func openRust(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/rust")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func TestRustPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = rustSnapshot()
	release := time.Now().AddDate(0, 0, -7)
	a.RustVersions = &versions.Rust{Lines: []versions.Cycle{{Cycle: "1.85", Latest: "1.85.0", Release: release}}}
	tt := openRust(a)
	screenshot(t, tt, "rust")
	for _, text := range []string{"3 toolchains", "stable 1.84.0", "1.79.0", "nightly", "2025-01-20", "1.83.0", "duplicate in PATH", "How the terminal finds rustc",
		"stable 1.85.0", "update for 1.84.0", "beta 1.86.0", "becomes stable in 5 weeks", "nightly 1.87.0", "installed build of 2025-01-20",
		"Version manager · rustup", "Targets · stable", "wasm32-unknown-unknown", "this computer", "Cargo utilities", "ripgrep", "8.5.3"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q; texts %q", text, tt.Texts())
		}
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Targets · stable") || !tt.HasText("which -a rustc") {
		t.Fatalf("PATH tab texts %q", tt.Texts())
	}
}

func TestRustNotInstalled(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "rust"}}}
	tt := openRust(a)
	if !tt.HasText("Rust is not installed") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
