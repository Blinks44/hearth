package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Rustup is the Manager of rustup's proxies and toolchains.
const Rustup = "rustup"

// rustupFiles are the project files rustup picks a toolchain by.
var rustupFiles = []string{"rust-toolchain.toml", "rust-toolchain"}

// Rust finds every rustc: on the terminal's PATH (term) and the GUI's
// (gui), and the toolchains in RUSTUP_HOME. rustup's proxies in
// CARGO_HOME/bin are never run: in a project they may install the
// toolchain it asks for. A toolchain's own rustc is run for its version;
// the default toolchain is read from rustup's settings.toml, and the
// commands cargo install put in CARGO_HOME/bin from .crates2.json.
func Rust(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	home := sources.ManagerDirs(term).Home
	rustupHome, cargoHome := rustDir(term, "RUSTUP_HOME", home, ".rustup"), rustDir(term, "CARGO_HOME", home, ".cargo")
	cargoBin := filepath.Join(cargoHome, "bin")
	f := newFinder("rust", term)
	f.lookPath(rustcExe, term, gui)
	var toolchains []string
	if rustupHome != "" {
		matches, _ := filepath.Glob(filepath.Join(rustupHome, "toolchains", "*", "bin", rustcExe))
		for _, m := range matches {
			toolchains = append(toolchains, filepath.Dir(filepath.Dir(m)))
			f.add(m)
		}
	}
	def := rustDefault(term, rustupHome)
	tools := cargoTools(cargoHome)
	st := inventory.ToolState{Tool: "rust", Installations: f.ins}
	for i := range st.Installations {
		in := &st.Installations[i]
		if cargoHome != "" && filepath.Dir(in.Path) == cargoBin {
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: Rustup, Root: rustupHome, Shim: true}
			continue
		}
		root := filepath.Dir(filepath.Dir(in.RealPath))
		if slices.Contains(toolchains, root) {
			name := filepath.Base(root)
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: Rustup, Root: root, Version: name}
			in.Rust = &inventory.RustToolchain{Name: name, Targets: rustTargets(root), Components: rustComponents(root)}
			in.Default = def != "" && (name == def || strings.HasPrefix(name, def+"-"))
		}
		if v := rustcVersion.FindStringSubmatch(rustOutput(ctx, r, term, in.RealPath, "--version")); v != nil {
			in.Version = v[1]
			if in.Rust != nil {
				in.Rust.Date = v[2]
			}
		}
		if in.Version == "" && in.Rust != nil {
			in.Version = rustNameVersion.FindString(in.Rust.Name)
		}
		if in.Version == "" && in.Rust == nil {
			in.Version = pathVersion(in.Source.Version)
		}
		in.Packages = tools
	}
	if len(toolchains) > 0 || exists(filepath.Join(cargoBin, rustupExe)) {
		m := inventory.Manager{Name: Rustup, SwitchOnCD: true, Files: rustupFiles,
			Shims: onPath(term, cargoBin), ShimsInGUI: onPath(gui, cargoBin)}
		if v := rustupVersion.FindStringSubmatch(rustOutput(ctx, r, term, filepath.Join(cargoBin, rustupExe), "--version")); v != nil {
			m.Version = v[1]
		}
		st.Managers = []inventory.Manager{m}
	}
	if cargoHome != "" {
		st.Vars = map[string]string{"RUSTUP_HOME": rustupHome, "CARGO_HOME": cargoHome}
	}
	return st, ctx.Err()
}

var (
	// rustcVersion reads rustc --version, as "rustc 1.86.0-nightly
	// (a1b2c3d4e 2025-01-20)": the version and the build date.
	rustcVersion = regexp.MustCompile(`(?m)^rustc (\d+\.\d+\.\d+(?:-[a-z]+)?)(?: \(\w+ (\d{4}-\d\d-\d\d)\))?`)
	// rustupVersion reads rustup --version, "rustup 1.27.1 (54dd3d00f 2024-04-24)".
	rustupVersion = regexp.MustCompile(`(?m)^rustup (\d+\.\d+\.\d+)`)
	// rustNameVersion is the version a toolchain's name pins, as 1.79.0
	// of 1.79.0-aarch64-apple-darwin.
	rustNameVersion = regexp.MustCompile(`^\d+\.\d+(?:\.\d+)?`)
)

// rustDir is the directory the variable name sets, by default dir in
// the home; "" when neither is known.
func rustDir(env sysenv.Env, name, home, dir string) string {
	if v := env.Get(name); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, dir)
}

func rustOutput(ctx context.Context, r run.Runner, env sysenv.Env, path string, args ...string) string {
	if !exists(path) {
		return ""
	}
	res, err := r.Run(ctx, run.Cmd{Args: append([]string{path}, args...), Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return string(res.Stdout)
}

// rustDefault is the toolchain rustup runs outside projects that pin
// one: RUSTUP_TOOLCHAIN, else settings.toml's default_toolchain.
func rustDefault(env sysenv.Env, rustupHome string) string {
	if v := env.Get("RUSTUP_TOOLCHAIN"); v != "" {
		return v
	}
	var settings struct {
		DefaultToolchain string `toml:"default_toolchain"`
	}
	if rustupHome == "" {
		return ""
	}
	if _, err := toml.Decode(readFile(filepath.Join(rustupHome, "settings.toml")), &settings); err != nil {
		return ""
	}
	return settings.DefaultToolchain
}

// rustTargets reads the targets whose standard library rustup installed
// in the toolchain at root, by its list of components.
func rustTargets(root string) []string {
	var targets []string
	for line := range strings.SplitSeq(readFile(filepath.Join(root, "lib", "rustlib", "components")), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "rust-std-"); ok && t != "" {
			targets = append(targets, t)
		}
	}
	slices.Sort(targets)
	return targets
}

var knownRustComponents = []string{"clippy", "rustfmt", "rust-src", "rust-analyzer"}

// rustComponents reads the known optional rustup components installed in the
// toolchain at root from lib/rustlib/components (matching exact names or
// preview/host-suffixed entries like clippy-preview-aarch64-apple-darwin).
func rustComponents(root string) []string {
	var out []string
	for line := range strings.SplitSeq(readFile(filepath.Join(root, "lib", "rustlib", "components")), "\n") {
		entry := strings.TrimSpace(line)
		for _, comp := range knownRustComponents {
			if (entry == comp || strings.HasPrefix(entry, comp+"-")) && !slices.Contains(out, comp) {
				out = append(out, comp)
			}
		}
	}
	slices.Sort(out)
	return out
}

// cargoTools reads the packages cargo install put in CARGO_HOME, as its
// .crates2.json keeps them: "ripgrep 14.1.1 (registry+https://...)".
func cargoTools(cargoHome string) []inventory.Package {
	if cargoHome == "" {
		return nil
	}
	var crates struct {
		Installs map[string]struct{ Bins []string }
	}
	data, err := os.ReadFile(filepath.Join(cargoHome, ".crates2.json"))
	if err != nil || json.Unmarshal(data, &crates) != nil {
		return nil
	}
	var tools []inventory.Package
	for key, in := range crates.Installs {
		fields := strings.Fields(key)
		if len(fields) < 2 {
			continue
		}
		p := inventory.Package{Name: fields[0], Version: fields[1]}
		for _, bin := range in.Bins {
			p.Commands = append(p.Commands, strings.TrimSuffix(bin, ".exe"))
		}
		tools = append(tools, p)
	}
	slices.SortFunc(tools, func(a, b inventory.Package) int { return strings.Compare(a.Name, b.Name) })
	return tools
}
