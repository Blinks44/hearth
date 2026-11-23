//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestRustRustupAndBrew(t *testing.T) {
	dir := root(t)
	home := filepath.Join(dir, "home")
	cargoBin := filepath.Join(home, ".cargo", "bin")
	toolchains := filepath.Join(home, ".rustup", "toolchains")
	stable := filepath.Join(toolchains, "stable-aarch64-apple-darwin")
	pinned := filepath.Join(toolchains, "1.79.0-aarch64-apple-darwin")
	nightly := filepath.Join(toolchains, "nightly-aarch64-apple-darwin")
	brew := filepath.Join(dir, "brew", "Cellar", "rust", "1.83.0", "bin", "rustc")
	brewBin := filepath.Join(dir, "brew", "bin")
	exes := []string{filepath.Join(cargoBin, "rustc"), filepath.Join(cargoBin, "rustup"), brew}
	for _, tc := range []string{stable, pinned, nightly} {
		exes = append(exes, filepath.Join(tc, "bin", "rustc"))
	}
	for _, path := range exes {
		writeFile(t, path, "")
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	symlink(t, brew, filepath.Join(brewBin, "rustc"))
	writeFile(t, filepath.Join(home, ".rustup", "settings.toml"), "version = \"12\"\ndefault_toolchain = \"stable-aarch64-apple-darwin\"\n")
	writeFile(t, filepath.Join(stable, "lib", "rustlib", "components"), "cargo-aarch64-apple-darwin\nclippy-preview-aarch64-apple-darwin\nrust-analyzer-preview-aarch64-apple-darwin\nrust-src\nrust-std-aarch64-apple-darwin\nrust-std-wasm32-unknown-unknown\nrustc-aarch64-apple-darwin\nrustfmt-preview-aarch64-apple-darwin\n")
	writeFile(t, filepath.Join(home, ".cargo", ".crates2.json"), `{"installs":{"ripgrep 14.1.1 (registry+https://github.com/rust-lang/crates.io-index)":{"bins":["rg"]}}}`)

	env := sysenv.Env{"HOME=" + home, "HOMEBREW_PREFIX=" + filepath.Join(dir, "brew"), "PATH=" + cargoBin + string(os.PathListSeparator) + brewBin}
	r := &run.Fake{}
	r.Stdout("rustc 1.84.0 (9fc6b4312 2025-01-07)\n", filepath.Join(stable, "bin", "rustc"), "--version")
	r.Stdout("rustc 1.86.0-nightly (a1b2c3d4e 2025-01-20)\n", filepath.Join(nightly, "bin", "rustc"), "--version")
	r.Stdout("rustup 1.27.1 (54dd3d00f 2024-04-24)\n", filepath.Join(cargoBin, "rustup"), "--version")
	got, err := Rust(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}

	byRoot := map[string]inventory.Installation{}
	for _, in := range got.Installations {
		byRoot[filepath.Dir(filepath.Dir(in.RealPath))] = in
	}
	if in := byRoot[stable]; in.Version != "1.84.0" || !in.Default || in.Source.Version != "stable-aarch64-apple-darwin" || in.Rust == nil || in.Rust.Name != "stable-aarch64-apple-darwin" || len(in.Rust.Targets) != 2 || len(in.Rust.Components) != 4 || len(in.Packages) != 1 || in.Packages[0].Commands[0] != "rg" {
		t.Errorf("stable %+v %+v", in, in.Rust)
	}
	// A pinned toolchain that cannot run still has the version its name says.
	if in := byRoot[pinned]; in.Version != "1.79.0" || in.Default {
		t.Errorf("pinned %+v", in)
	}
	if in := byRoot[nightly]; in.Version != "1.86.0-nightly" || in.Rust.Date != "2025-01-20" {
		t.Errorf("nightly %+v %+v", in, in.Rust)
	}
	if in := byRoot[filepath.Dir(filepath.Dir(brew))]; in.Version != "1.83.0" || in.Source.Manager != "brew" || in.OnPath != 2 {
		t.Errorf("brew %+v", in)
	}
	if a := inventory.TerminalCopy(got.Installations); a < 0 || got.Installations[a].Version != "1.84.0" {
		t.Fatalf("terminal copy %d of %+v", a, got.Installations)
	}
	if len(got.Managers) != 1 || got.Managers[0].Version != "1.27.1" || !got.Managers[0].Shims || got.Managers[0].ShimsInGUI {
		t.Errorf("managers %+v", got.Managers)
	}
	for _, call := range r.Calls() {
		if call.Args[0] == filepath.Join(cargoBin, "rustc") {
			t.Fatalf("ran rustup's proxy: %v", call.Args)
		}
	}
}

func TestRustToolchainOverride(t *testing.T) {
	dir := root(t)
	home := filepath.Join(dir, "home")
	rustupHome := filepath.Join(dir, "rustup")
	beta := filepath.Join(rustupHome, "toolchains", "beta-x86_64-unknown-linux-gnu", "bin", "rustc")
	writeFile(t, beta, "")
	writeFile(t, filepath.Join(rustupHome, "settings.toml"), "default_toolchain = \"stable-x86_64-unknown-linux-gnu\"\n")
	env := sysenv.Env{"HOME=" + home, "RUSTUP_HOME=" + rustupHome, "RUSTUP_TOOLCHAIN=beta", "PATH="}
	got, err := Rust(t.Context(), &run.Fake{}, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 || !got.Installations[0].Default || got.Vars["RUSTUP_HOME"] != rustupHome {
		t.Fatalf("state %+v", got)
	}
}
