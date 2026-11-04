// Package detect finds the installed copies of developer tools. Detectors
// only read: files, the environment, and commands that change nothing.
package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	state := inventory.ToolState{Tool: "node"}
	byReal := map[string]int{}
	add := func(path string) *inventory.Installation {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}
		if i, ok := byReal[real]; ok {
			return &state.Installations[i]
		}
		byReal[real] = len(state.Installations)
		state.Installations = append(state.Installations, inventory.Installation{
			Tool:     "node",
			Path:     path,
			RealPath: real,
			Source:   sources.Classify(path, real, term),
		})
		return &state.Installations[len(state.Installations)-1]
	}

	for i, p := range sysenv.LookPathAll(nodeExe, term) {
		if in := add(p); in.OnPath == 0 {
			in.OnPath = inventory.PathRank(i + 1)
		}
	}
	for i, p := range sysenv.LookPathAll(nodeExe, gui) {
		if in := add(p); in.InGUI == 0 {
			in.InGUI = inventory.PathRank(i + 1)
		}
	}
	dirs := sources.ManagerDirs(term)
	for _, pattern := range nodeGlobs(dirs) {
		matches, _ := filepath.Glob(pattern) // only a malformed pattern fails
		for _, m := range matches {
			// A manager's alias directory, like mise's installs/node/22,
			// leads to the version's own; name the copy by that.
			if real, err := filepath.EvalSymlinks(m); err == nil {
				add(real)
			}
		}
	}

	installed := map[string][]string{}
	for i := range state.Installations {
		in := &state.Installations[i]
		if in.Source.Shim {
			continue
		}
		in.Version = pathVersion(in.Source.Version)
		if in.Version == "" {
			in.Version = nodeVersion(ctx, r, term, in.RealPath)
		}
		in.Packages = globalPackages(nodeModules(in.Path))
		in.Corepack = corepackEnabled(in.Path, in.RealPath)
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
	state.Managers = nodeManagers(state.Installations, term, dirs.Home)
	return state, ctx.Err()
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

// nodeVersion asks the node at path its version; "" when it cannot tell.
func nodeVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "--version"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(res.Stdout)), "v")
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
