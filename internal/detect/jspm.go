package detect

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Npm finds npm without running its launcher.
func Npm(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	return jsManager(ctx, r, "npm", term, gui)
}

// Pnpm finds pnpm and locally cached Corepack versions without running shims.
func Pnpm(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	return jsManager(ctx, r, "pnpm", term, gui)
}

// Yarn finds Yarn and locally cached Corepack versions without running shims.
func Yarn(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	return jsManager(ctx, r, "yarn", term, gui)
}

// Corepack finds Corepack itself; its package manifest gives its version.
func Corepack(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	return jsManager(ctx, r, "corepack", term, gui)
}

func jsManager(ctx context.Context, r run.Runner, name string, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder(name, term)
	f.lookPath(name, term, gui)
	if name == "pnpm" && pnpmHome(term) != "" {
		for _, exe := range jsLaunchers(name) {
			p := filepath.Join(pnpmHome(term), exe)
			if exists(p) {
				f.add(p)
			}
		}
	}
	// mise keeps pnpm in the version's folder, Yarn in its bin.
	if mise := sources.ManagerDirs(term).Mise; mise != "" && (name == "pnpm" || name == "yarn") {
		for _, exe := range jsLaunchers(name) {
			for _, pattern := range []string{filepath.Join(mise, "installs", name, "*", exe), filepath.Join(mise, "installs", name, "*", "bin", exe)} {
				matches, _ := filepath.Glob(pattern) // only a malformed pattern fails
				for _, m := range matches {
					if real, err := sysenv.RealPath(m); err == nil {
						f.add(real)
					}
				}
			}
		}
	}
	nodes := newFinder("node", term)
	nodes.lookPath(nodeExe, term, gui)
	for _, pattern := range nodeGlobs(sources.ManagerDirs(term)) {
		matches, _ := filepath.Glob(pattern)
		for _, p := range matches {
			// As Node does: a manager's alias directory is the version's.
			if real, err := sysenv.RealPath(p); err == nil {
				nodes.add(real)
			}
		}
	}
	for _, node := range nodes.ins {
		if node.Source.Shim {
			continue
		}
		for _, path := range []string{node.Path, node.RealPath} {
			for _, exe := range jsLaunchers(name) {
				p := filepath.Join(filepath.Dir(path), exe)
				if exists(p) {
					f.add(p)
				}
			}
		}
	}
	for i := range f.ins {
		in := &f.ins[i]
		pkg := npmPackage(in.Path, in.RealPath, name)
		if pkg == "" {
			pkg = filepath.Join(nodeModules(filepath.Join(filepath.Dir(in.Path), nodeExe)), name)
		}
		if p, ok := readPackage(pkg); ok && !in.Source.Shim {
			in.Version = p.Version
			if name == "npm" {
				modules, _ := npmGlobal(filepath.Join(filepath.Dir(in.Path), nodeExe), term)
				in.Packages = globalPackages(modules)
			} else if in.Source.Kind == inventory.KindUnknown && npmPackage(in.Path, in.RealPath, name) != "" {
				// npm install -g put it in npm's prefix, as Corepack since Node 25.
				in.Source = inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm", Root: pkg}
			}
		}
		// Not npmPackage: on Windows the Node folder holds both npm.cmd and
		// node_modules\corepack, and npm there is no shim. Corepack's own
		// .cmd shims name it in their text.
		core := linkedPackage(in.RealPath, "corepack")
		if name != "corepack" && (core != "" || strings.Contains(readHead(in.Path), "corepack")) {
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack", Root: core, Shim: true}
			in.Version = ""
		}
		if in.Version == "" && !in.Source.Shim && in.Source.Manager == "mise" {
			in.Version = pathVersion(in.Source.Version)
		}
		if in.Version == "" && !in.Source.Shim {
			in.Version = versionOf(ctx, r, term, in.RealPath)
		}
	}
	cache := corepackHome(term)
	var defaults map[string]string
	_ = json.Unmarshal([]byte(readFile(filepath.Join(cache, "lastKnownGood.json"))), &defaults)
	usesCorepack := false
	for _, in := range f.ins {
		if in.Source.Manager == "corepack" && in.OnPath == 1 {
			usesCorepack = true
		}
	}
	if name == "pnpm" || name == "yarn" || name == "npm" {
		matches, _ := filepath.Glob(filepath.Join(cache, "v1", name, "*", ".corepack"))
		for _, marker := range matches {
			if ctx.Err() != nil {
				break
			}
			dir := filepath.Dir(marker)
			version := filepath.Base(dir)
			in := f.add(dir)
			in.Version = version
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack", Root: dir}
			want, _, _ := strings.Cut(defaults[name], "+")
			in.Default = version == want
		}
	}
	st := inventory.ToolState{Tool: name, Installations: f.ins}
	if usesCorepack && term.Get("COREPACK_ENABLE_PROJECT_SPEC") != "0" {
		st.Managers = []inventory.Manager{{Name: "corepack", SwitchOnCD: true, Files: []string{"package.json"}}}
	}
	return st, ctx.Err()
}

// corepackHome is Corepack's cache folder, chosen as Corepack does.
func corepackHome(env sysenv.Env) string {
	if p := env.Get("COREPACK_HOME"); p != "" {
		return p
	}
	base := env.Get("XDG_CACHE_HOME")
	if base == "" {
		base = env.Get("LOCALAPPDATA")
	}
	if base == "" {
		base = filepath.Join(sources.ManagerDirs(env).Home, userCacheDir)
	}
	return filepath.Join(base, "node", "corepack")
}
