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
	if tt.HasText("Targets · stable") || !tt.HasText(whichAllCommand+" rustc") {
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

func TestRustManagement(t *testing.T) {
	a := NewApp()
	a.Shell = &fakeShell{}
	a.Snapshot = rustSnapshot()
	release := time.Now().AddDate(0, 0, -7)
	a.RustVersions = &versions.Rust{Lines: []versions.Cycle{{Cycle: "1.85", Latest: "1.85.0", Release: release}}}
	tt := openRust(a)
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "rust-narrow")

	// Header button proposes updating stable when outdated.
	if err := tt.Click("Install stable"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup toolchain install stable") {
		t.Fatalf("header install plan missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Available channel button proposes installing beta.
	if err := tt.Click("Install beta"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup toolchain install beta") {
		t.Fatalf("beta install plan missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Custom toolchain input proposes installing a pinned version.
	a.rustToolchainInput = "1.85.0"
	tt.Frame()
	if err := tt.Click("Install 1.85.0"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup toolchain install 1.85.0") {
		t.Fatalf("custom toolchain plan missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Toolchain row menu offers Make default and Remove….
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup toolchain uninstall stable-aarch64-apple-darwin") {
		t.Fatalf("uninstall plan missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Non-host target can be removed; host target has no remove button.
	if err := tt.Click("Remove target wasm32-unknown-unknown"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup target remove --toolchain stable-aarch64-apple-darwin wasm32-unknown-unknown") {
		t.Fatalf("target remove plan missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Custom target input proposes rustup target add.
	a.rustTargetInput = "x86_64-unknown-linux-gnu"
	tt.Frame()
	if err := tt.Click("Add target x86_64-unknown-linux-gnu"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup target add --toolchain stable-aarch64-apple-darwin x86_64-unknown-linux-gnu") {
		t.Fatalf("target add plan missing: %q", tt.Texts())
	}
}
