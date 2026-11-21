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
	// mise's shim runs the version its global config picks: the tool's
	// own, or, for what comes with Node, the one in Node's folder.
	dirs := sources.ManagerDirs(term)
	versions := map[string][]string{}
	for _, in := range slices.Concat(f.ins, nodes.ins) {
		if in.Source.Manager == "mise" && !in.Source.Shim && in.Source.Version != "" {
			tool := filepath.Base(filepath.Dir(in.Source.Root))
			versions[tool] = append(versions[tool], in.Source.Version)
		}
	}
	miseShim := slices.IndexFunc(f.ins, func(in inventory.Installation) bool {
		return in.Source.Manager == "mise" && in.Source.Shim && in.OnPath > 0
	})
	nodeDef := miseDefault(dirs, term, "node", versions["node"])
	if versions[name] != nil {
		// mise's own copy of the tool runs, as its pnpm.
		nodeDef = ""
	}
	for i := range f.ins {
		in := &f.ins[i]
		switch {
		case in.Source.Manager == "mise" && !in.Source.Shim:
			tool := filepath.Base(filepath.Dir(in.Source.Root))
			if def := miseDefault(dirs, term, tool, versions[tool]); def != "" && in.Source.Version == def {
				in.Default = true
			}
		case in.Source.Manager == "corepack" && in.Source.Shim && miseShim >= 0 && nodeDef != "" && dirs.Mise != "" &&
			within(in.RealPath, filepath.Join(dirs.Mise, "installs", "node", nodeDef)):
			// mise's shim runs Corepack's in the default Node's folder:
			// one command, where mise's shim stands on PATH.
			if r := f.ins[miseShim].OnPath; in.OnPath == 0 || r < in.OnPath {
				in.OnPath = r
			}
			f.ins[miseShim].OnPath = 0
		}
	}
	if miseShim >= 0 && f.ins[miseShim].OnPath == 0 && f.ins[miseShim].InGUI == 0 {
		f.ins = slices.Delete(f.ins, miseShim, miseShim+1)
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
	var userPkgs []inventory.Package
	switch name {
	case "pnpm":
		userPkgs = pnpmGlobalPackages(term)
	case "yarn":
		userPkgs = yarnGlobalPackages(term)
	}
	if len(userPkgs) > 0 {
		for i := range f.ins {
			f.ins[i].Packages = userPkgs
		}
	}
	st := inventory.ToolState{Tool: name, Installations: f.ins}
	if usesCorepack && term.Get("COREPACK_ENABLE_PROJECT_SPEC") != "0" {
		st.Managers = []inventory.Manager{{Name: "corepack", SwitchOnCD: true, Files: []string{"package.json"}}}
	}
	// mise's shim runs the version the folder's mise.toml asks for.
	if slices.ContainsFunc(f.ins, func(in inventory.Installation) bool { return in.Source.Manager == "mise" }) {
		st.Managers = append(st.Managers, miseManager(ctx, r, term, gui, sources.ManagerDirs(term)))
	}
	return st, ctx.Err()
}

// manifestPackages reads the top-level packages listed in dir/package.json's
// dependencies and resolves each against dir/node_modules so transitive
// packages hoisted by Yarn v1 are excluded.
func manifestPackages(dir string) []inventory.Package {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var m struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if json.Unmarshal(data, &m) != nil || len(m.Dependencies) == 0 {
		return nil
	}
	var pkgs []inventory.Package
	for name := range m.Dependencies {
		pkgDir := filepath.Join(dir, "node_modules", filepath.FromSlash(name))
		if p, ok := readPackage(pkgDir); ok {
			pkgs = append(pkgs, p)
		}
	}
	slices.SortFunc(pkgs, func(a, b inventory.Package) int { return strings.Compare(a.Name, b.Name) })
	return pkgs
}

// pnpmGlobalPackages reads packages installed with pnpm add -g from
// PNPM_HOME/global/*/package.json without running pnpm.
func pnpmGlobalPackages(env sysenv.Env) []inventory.Package {
	home := pnpmHome(env)
	if home == "" {
		return nil
	}
	manifests, _ := filepath.Glob(filepath.Join(home, "global", "*", "package.json"))
	slices.Sort(manifests)
	byName := map[string]inventory.Package{}
	for _, m := range manifests {
		for _, p := range manifestPackages(filepath.Dir(m)) {
			byName[p.Name] = p
		}
	}
	if len(byName) == 0 {
		return nil
	}
	pkgs := make([]inventory.Package, 0, len(byName))
	for _, p := range byName {
		pkgs = append(pkgs, p)
	}
	slices.SortFunc(pkgs, func(a, b inventory.Package) int { return strings.Compare(a.Name, b.Name) })
	return pkgs
}

// yarnGlobalPackages reads packages installed with yarn global add from
// Yarn's global folder without running yarn.
func yarnGlobalPackages(env sysenv.Env) []inventory.Package {
	dir := yarnGlobalDir(env)
	if dir == "" {
		return nil
	}
	return manifestPackages(dir)
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
