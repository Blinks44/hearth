// Package detect finds the installed copies of developer tools. Detectors
// only read: files, the environment, and commands that change nothing.
package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Node finds every copy of Node.js: on the terminal's PATH (term), on the
// GUI's (gui), and in the version managers' directories. A copy that two
// paths lead to is one installation. Its version comes from the path when
// the path names one, else from node --version; a shim's stays unknown,
// since running one may pick or fetch a version.
func Node(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("node", term)
	f.lookPath(nodeExe, term, gui)
	dirs := sources.ManagerDirs(term)
	for _, pattern := range nodeGlobs(dirs) {
		matches, _ := filepath.Glob(pattern) // only a malformed pattern fails
		for _, m := range matches {
			// A manager's alias directory, like mise's installs/node/22,
			// leads to the version's own; name the copy by that.
			if real, err := sysenv.RealPath(m); err == nil {
				f.add(real)
			}
		}
	}

	state := inventory.ToolState{Tool: "node", Installations: f.ins}
	installed := map[string][]string{}
	for i := range state.Installations {
		in := &state.Installations[i]
		if in.Source.Shim {
			continue
		}
		in.Version = pathVersion(in.Source.Version)
		if in.Version == "" {
			in.Version = versionOf(ctx, r, term, in.RealPath)
		}
		modules, bin := npmGlobal(in.Path, term)
		in.Packages = globalPackages(modules)
		in.Corepack = corepackEnabled(filepath.Dir(in.Path), filepath.Dir(in.RealPath), bin)
		if m := in.Source.Manager; m != "" && in.Version != "" {
			installed[m] = append(installed[m], in.Version)
		}
	}
	defaults := nodeDefaults(dirs, term, installed)
	for i := range state.Installations {
		in := &state.Installations[i]
		if want, ok := defaults[in.Source.Manager]; ok && !in.Source.Shim && in.Version == want {
			in.Default = true
		}
	}
	state.Managers = withMise(ctx, r, nodeManagers(state.Installations, term, dirs.Home), term, gui, dirs)
	return state, ctx.Err()
}

// finder collects a tool's copies: a copy two paths lead to is one.
type finder struct {
	tool   string
	env    sysenv.Env // gives the home and the managers' directories
	ins    []inventory.Installation
	byReal map[string]int
}

func newFinder(tool string, env sysenv.Env) *finder {
	return &finder{tool: tool, env: env, byReal: map[string]int{}}
}

// add returns the copy at path, adding it when no copy so far has its
// real path. A Scoop shim runs one file, so it stands for that file's
// copy, as a symlink does.
func (f *finder) add(path string) *inventory.Installation {
	real, err := sysenv.RealPath(path)
	if err != nil {
		real = path
	}
	source := sources.Classify(path, real, f.env)
	if source.Shim && source.Manager == "scoop" {
		if target := scoopShimTarget(path); target != "" {
			if real, err = sysenv.RealPath(target); err != nil {
				real = target
			}
			source = sources.Classify(real, real, f.env)
		}
	}
	if i, ok := f.byReal[real]; ok {
		return &f.ins[i]
	}
	f.byReal[real] = len(f.ins)
	f.ins = append(f.ins, inventory.Installation{
		Tool:     f.tool,
		Path:     path,
		RealPath: real,
		Source:   source,
	})
	return &f.ins[len(f.ins)-1]
}

// lookPath adds the copies of exe on the terminal's PATH (term) and the
// GUI's (gui), with their ranks there.
func (f *finder) lookPath(exe string, term, gui sysenv.Env) {
	for i, p := range sysenv.LookPathAll(exe, term) {
		if in := f.add(p); in.OnPath == 0 {
			in.OnPath = inventory.PathRank(i + 1)
		}
	}
	for i, p := range sysenv.LookPathAll(exe, gui) {
		if in := f.add(p); in.InGUI == 0 {
			in.InGUI = inventory.PathRank(i + 1)
		}
	}
}

// nodeGlobs are where version managers keep Node versions, one match per
// version.
func nodeGlobs(d sources.Dirs) []string {
	var globs []string
	glob := func(dir string, parts ...string) {
		if dir != "" {
			globs = append(globs, filepath.Join(append([]string{dir}, parts...)...))
		}
	}
	for _, fnm := range d.Fnm {
		glob(fnm, "node-versions", "*", "installation", nodeBin)
	}
	glob(d.Nvm, "versions", "node", "*", nodeBin)
	glob(d.Volta, "tools", "image", "node", "*", nodeBin)
	glob(d.Mise, "installs", "node", "*", nodeBin)
	glob(d.Asdf, "installs", "nodejs", "*", nodeBin)
	glob(d.Nodenv, "versions", "*", nodeBin)
	glob(d.N, "n", "versions", "node", "*", nodeBin)
	glob(d.NvmWindows, "*", nodeExe)
	return globs
}

// pathVersion cleans a version a path names: Homebrew adds a revision, as
// in 23.3.0_1.
func pathVersion(v string) string {
	v, _, _ = strings.Cut(v, "_")
	return v
}

// versionOf asks the program at path its version, by --version, as
// node's v22.12.0 or bun's 1.2.5; "" when it cannot tell.
func versionOf(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "--version"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(res.Stdout)), "v")
}

// npmGlobal is where npm -g of the node at path puts packages and their
// commands. npm's prefix is found as npm finds it, without running it:
// npm_config_prefix, ~/.npmrc, then the npmrc npm ships with, where Node's
// Windows installer sets ${APPDATA}\npm; else it is the node's own prefix.
func npmGlobal(path string, env sysenv.Env) (modules, bin string) {
	prefix := env.Get("npm_config_prefix")
	if prefix == "" {
		prefix = env.Get("NPM_CONFIG_PREFIX")
	}
	home := sources.ManagerDirs(env).Home
	for _, rc := range []string{filepath.Join(home, ".npmrc"), filepath.Join(nodeModules(path), "npm", "npmrc")} {
		if prefix == "" {
			prefix = npmrcPrefix(rc, env)
		}
	}
	if prefix == "" {
		return nodeModules(path), filepath.Dir(path)
	}
	if rest, ok := strings.CutPrefix(prefix, "~"); ok && (rest == "" || os.IsPathSeparator(rest[0])) {
		prefix = home + rest
	}
	return prefixModules(prefix), prefixBin(prefix)
}

var npmrcVar = regexp.MustCompile(`\$\{([^}]+)\}`)

// npmrcPrefix is the prefix an npmrc file sets, with its ${VAR}s expanded
// as npm does; "" when it sets none.
func npmrcPrefix(path string, env sysenv.Env) string {
	data, _ := os.ReadFile(path) // no file, no prefix
	prefix := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "prefix" {
			prefix = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return npmrcVar.ReplaceAllStringFunc(prefix, func(m string) string { return env.Get(m[2 : len(m)-1]) })
}

// globalPackages reads the packages in a node_modules directory, scoped
// ones included, by their package.json files.
func globalPackages(dir string) []inventory.Package {
	var pkgs []inventory.Package
	entries, _ := os.ReadDir(dir) // no directory, no packages
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasPrefix(name, "@") {
			scoped, _ := os.ReadDir(filepath.Join(dir, name))
			for _, s := range scoped {
				if p, ok := readPackage(filepath.Join(dir, name, s.Name())); ok {
					pkgs = append(pkgs, p)
				}
			}
			continue
		}
		if p, ok := readPackage(filepath.Join(dir, name)); ok {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

func readPackage(dir string) (inventory.Package, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return inventory.Package{}, false
	}
	var p struct {
		Name, Version string
		Bin           any
	}
	if json.Unmarshal(data, &p) != nil {
		return inventory.Package{}, false
	}
	if p.Name == "" {
		p.Name = filepath.Base(dir)
	}
	return inventory.Package{Name: p.Name, Version: p.Version, Commands: commands(p.Name, p.Bin)}, true
}
