package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Bun finds every copy of Bun: on the terminal's PATH (term), on the GUI's
// (gui), in the install script's directory and in mise's. A copy npm put
// there is told by the bun package it lies in, one the script put there by
// its directory. The version comes from the path or npm's package when they
// name one, else from bun --version; a shim's stays unknown.
func Bun(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("bun", term)
	f.lookPath("bun", term, gui)
	dirs := sources.ManagerDirs(term)
	install := bunInstallDir(term, dirs.Home)
	if p := filepath.Join(install, "bin", bunExe); install != "" && exists(p) {
		f.add(p)
	}
	if dirs.Mise != "" {
		matches, _ := filepath.Glob(filepath.Join(dirs.Mise, "installs", "bun", "*", "bin", bunExe)) // only a malformed pattern fails
		for _, m := range matches {
			if real, err := filepath.EvalSymlinks(m); err == nil {
				f.add(real)
			}
		}
	}
	realInstall, err := filepath.EvalSymlinks(install)
	if err != nil {
		realInstall = install
	}

	// bun add -g puts packages in the install directory, whichever copy
	// runs it.
	globals := bunGlobals(install)
	state := inventory.ToolState{Tool: "bun", Installations: f.ins}
	for i := range state.Installations {
		in := &state.Installations[i]
		if in.Source.Shim {
			continue
		}
		in.Packages = globals
		switch pkg := npmPackage(in.Path, in.RealPath, "bun"); {
		case pkg != "":
			in.Source = inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm", Root: pkg}
			if p, ok := readPackage(pkg); ok {
				in.Version = p.Version
			}
		case realInstall != "" && within(in.RealPath, realInstall):
			in.Source = inventory.Source{Kind: inventory.KindInstaller, Manager: inventory.ScriptInstall, Root: install}
			in.PathFile = startupFileWith(term, dirs.Home, append(homeForms(install, dirs.Home), "BUN_INSTALL")...)
		default:
			in.Version = pathVersion(in.Source.Version)
		}
		if in.Version == "" {
			in.Version = versionOf(ctx, r, term, in.RealPath)
		}
	}
	return state, ctx.Err()
}

// bunInstallDir is where Bun's install script puts it: BUN_INSTALL, or
// .bun in the home; "" when neither is known.
func bunInstallDir(env sysenv.Env, home string) string {
	if dir := env.Get("BUN_INSTALL"); dir != "" {
		return dir
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".bun")
}

// bunGlobals reads the packages bun add -g installed in install: the
// ones its global package.json depends on, not theirs.
func bunGlobals(install string) []inventory.Package {
	if install == "" {
		return nil
	}
	global := filepath.Join(install, "install", "global")
	var manifest struct{ Dependencies map[string]string }
	if json.Unmarshal([]byte(readFile(filepath.Join(global, "package.json"))), &manifest) != nil {
		return nil
	}
	var pkgs []inventory.Package
	for name, want := range manifest.Dependencies {
		p, ok := readPackage(filepath.Join(global, "node_modules", filepath.FromSlash(name)))
		if !ok {
			p = inventory.Package{Name: name, Version: want}
		}
		pkgs = append(pkgs, p)
	}
	slices.SortFunc(pkgs, func(a, b inventory.Package) int { return strings.Compare(a.Name, b.Name) })
	return pkgs
}

// npmPackage returns the folder of the npm package name the copy at path
// comes from, or "" when npm did not put it there. On Unix npm links the
// command into the package, which the real path shows; on Windows it
// writes a script beside the global node_modules.
func npmPackage(path, real, name string) string {
	if dir := linkedPackage(real, name); dir != "" {
		return dir
	}
	if dir := filepath.Join(filepath.Dir(path), "node_modules", name); exists(filepath.Join(dir, "package.json")) {
		return dir
	}
	return ""
}

// linkedPackage returns the folder of the npm package name that the real
// path lies in, or "" when it lies in none.
func linkedPackage(real, name string) string {
	for dir := filepath.Dir(real); ; {
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if filepath.Base(dir) == name && filepath.Base(parent) == "node_modules" {
			return dir
		}
		dir = parent
	}
}

// startupFileWith returns the first shell startup file in home that
// mentions one of needles, as the line putting a directory on PATH does;
// "" for none.
func startupFileWith(env sysenv.Env, home string, needles ...string) string {
	for _, p := range StartupFiles(env, home) {
		text := readFile(p)
		for _, n := range needles {
			if n != "" && strings.Contains(text, n) {
				return p
			}
		}
	}
	return ""
}

// homeForms are the ways a startup file may write dir: as it is, and,
// when it is in home, from $HOME, ${HOME} or ~.
func homeForms(dir, home string) []string {
	forms := []string{dir}
	if home != "" && within(dir, home) {
		rest := filepath.ToSlash(dir[len(home):])
		forms = append(forms, "$HOME"+rest, "${HOME}"+rest, "~"+rest)
	}
	return forms
}

// within reports whether path is dir or in it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimRight(dir, `/\`)+string(filepath.Separator))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
