package projects

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

type installSpec struct {
	name      string
	role      string
	manifests []string
	files     []string
}

// installSpecs are the dependency or build folders tools put back from a
// project's manifest, with the manifests that confirm the folder belongs
// to that ecosystem and the files that tell when the project changed.
var installSpecs = []installSpec{
	{
		name:      "node_modules",
		role:      inventory.InstallRoleDeps,
		manifests: []string{"package.json"},
		files:     []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"},
	},
	{
		name:      ".venv",
		role:      inventory.InstallRoleDeps,
		manifests: []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "uv.lock", "poetry.lock"},
		files:     []string{"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock", "setup.py", "setup.cfg", "uv.lock", "poetry.lock", ".python-version"},
	},
	{
		name:      "venv",
		role:      inventory.InstallRoleDeps,
		manifests: []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "uv.lock", "poetry.lock"},
		files:     []string{"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock", "setup.py", "setup.cfg", "uv.lock", "poetry.lock", ".python-version"},
	},
	{
		name:      "vendor",
		role:      inventory.InstallRoleDeps,
		manifests: []string{"composer.json"},
		files:     []string{"composer.json", "composer.lock"},
	},
	{
		name:      "Pods",
		role:      inventory.InstallRoleDeps,
		manifests: []string{"Podfile"},
		files:     []string{"Podfile", "Podfile.lock"},
	},
	{
		name:      "target",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"Cargo.toml", "pom.xml"},
		files:     []string{"Cargo.toml", "Cargo.lock", "rust-toolchain.toml", "rust-toolchain", "pom.xml"},
	},
	{
		name:      ".next",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"next.config.*", "package.json"},
		files:     []string{"package.json", "next.config.*"},
	},
	{
		name:      ".nuxt",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"nuxt.config.*", "package.json"},
		files:     []string{"package.json", "nuxt.config.*"},
	},
	{
		name:      ".turbo",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"turbo.json", "package.json"},
		files:     []string{"turbo.json", "package.json"},
	},
	{
		name:      ".svelte-kit",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"svelte.config.*", "package.json"},
		files:     []string{"package.json", "svelte.config.*"},
	},
	{
		name:      ".parcel-cache",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"package.json", ".parcelrc"},
		files:     []string{"package.json", ".parcelrc"},
	},
	{
		name:      "dist",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"package.json", "tsconfig.json", "vite.config.*", "rsbuild.config.*", "pyproject.toml"},
		files:     []string{"package.json", "tsconfig.json", "vite.config.*", "rsbuild.config.*", "pyproject.toml"},
	},
	{
		name:      "build",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "CMakeLists.txt", "pubspec.yaml", "package.json"},
		files:     []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "CMakeLists.txt", "pubspec.yaml", "package.json"},
	},
	{
		name:      ".gradle",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
		files:     []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
	},
	{
		name:      "bin",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"*.csproj", "*.fsproj", "*.vbproj"},
		files:     []string{"*.csproj", "*.fsproj", "*.vbproj", "global.json"},
	},
	{
		name:      "obj",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"*.csproj", "*.fsproj", "*.vbproj"},
		files:     []string{"*.csproj", "*.fsproj", "*.vbproj", "global.json"},
	},
	{
		name:      "__pycache__",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "pytest.ini"},
		files:     []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "pytest.ini"},
	},
	{
		name:      ".pytest_cache",
		role:      inventory.InstallRoleBuild,
		manifests: []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "pytest.ini"},
		files:     []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "pytest.ini"},
	},
}

// InstallRole returns the Cleanup role ("deps" or "build") for a known project
// dependency or build directory name, or "" when unknown.
func InstallRole(name string) string {
	if spec, ok := installSpecByName(name); ok {
		return spec.role
	}
	return ""
}

func installSpecByName(name string) (installSpec, bool) {
	for _, s := range installSpecs {
		if s.name == name {
			return s, true
		}
	}
	return installSpec{}, false
}

// hasInstallManifest reports whether dir contains at least one regular file
// matching any entry in names (exact filename or filepath.Glob pattern).
func hasInstallManifest(dir string, names []string) bool {
	for _, name := range names {
		if strings.ContainsAny(name, "*?[") {
			matches, err := filepath.Glob(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && !info.IsDir() {
					return true
				}
			}
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// homeSkipped are the folders under home that hold apps' and tools' own
// files, never the user's projects: macOS's Library, Windows' AppData,
// snap's data, Scoop's apps, Go's module cache and conda's installs. The
// media folders hold no projects either, and on macOS reading Music and
// Pictures asks the user for access to Apple Music and Photos.
var homeSkipped = []string{"Library", "AppData", "snap", "scoop", filepath.Join("go", "pkg"),
	"miniconda3", "anaconda3", "miniforge3", "mambaforge", "Music", "Pictures", "Movies", "Videos"}

// deviceOf is device, which tests replace to put a folder on a mount.
var deviceOf = device

// InstallsOptions configures discovery of project dependency and build folders.
type InstallsOptions struct {
	// Exclude skips additional directories (relative to home, ~/..., or absolute).
	Exclude []string
	// ExtraProjects scans tracked project directories even when they live outside home.
	ExtraProjects []string
}

// Installs finds dependency and build folders under home that sit next
// to a matching project manifest, with when the project last changed. It never
// goes into matched folders, node_modules, hidden folders other than recognized
// artifact specs, tests' testdata, app bundles, Electron app resources, the
// folders in homeSkipped, nor another file system mounted under home, as
// OrbStack's volumes in ~/OrbStack or a network share; it does not follow symlinks.
// When ctx ends it stops, with what it found so far and the error.
func Installs(ctx context.Context, home string) ([]inventory.ProjectInstall, error) {
	return InstallsWithOptions(ctx, home, InstallsOptions{})
}

// InstallsWithExclude is Installs with extra directories under home (or
// absolute paths) skipped in addition to homeSkipped.
func InstallsWithExclude(ctx context.Context, home string, extraExclude []string) ([]inventory.ProjectInstall, error) {
	return InstallsWithOptions(ctx, home, InstallsOptions{Exclude: extraExclude})
}

// InstallsWithOptions discovers project dependency and build folders under home
// and inside any tracked ExtraProjects (including those outside home), skipping
// excluded directories and deduplicating matches.
func InstallsWithOptions(ctx context.Context, home string, opts InstallsOptions) ([]inventory.ProjectInstall, error) {
	skip := map[string]bool{}
	if home != "" {
		for _, name := range homeSkipped {
			skip[filepath.Join(home, name)] = true
		}
	}
	for _, raw := range opts.Exclude {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if (strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`)) && home != "" {
			p = filepath.Join(home, p[2:])
		} else if !filepath.IsAbs(p) && home != "" {
			p = filepath.Join(home, p)
		}
		skip[filepath.Clean(p)] = true
	}

	var out []inventory.ProjectInstall
	seen := map[string]bool{}
	walkRoot := func(root string) error {
		if root == "" {
			return nil
		}
		cleanRoot := filepath.Clean(root)
		if skip[cleanRoot] {
			return nil
		}
		var rootDev uint64
		rootKnown := false
		if info, err := os.Stat(cleanRoot); err == nil {
			if !info.IsDir() {
				return nil
			}
			rootDev, rootKnown = deviceOf(info)
		} else {
			return nil
		}
		return filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil || !d.IsDir() {
				// A folder that cannot be listed is left out.
				return nil
			}
			name := d.Name()
			if path == cleanRoot {
				return nil
			}
			if name == "testdata" || skip[path] || strings.HasSuffix(name, ".app") || electronResources(path, name) {
				return filepath.SkipDir
			}
			if info, err := d.Info(); err == nil && rootKnown {
				if dev, ok := deviceOf(info); ok && dev != rootDev {
					return filepath.SkipDir
				}
			}
			if spec, ok := installSpecByName(name); ok {
				dir := filepath.Dir(path)
				if hasInstallManifest(dir, spec.manifests) {
					key := filepath.Clean(path)
					if !seen[key] {
						seen[key] = true
						out = append(out, inventory.ProjectInstall{
							Project: dir,
							Root:    enclosingProjectRoot(dir, home, opts.ExtraProjects),
							Name:    name,
							Role:    spec.role,
							Changed: changed(Project{Dir: dir, Files: spec.files}),
						})
					}
					return filepath.SkipDir
				}
				if name == "node_modules" {
					return filepath.SkipDir
				}
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		})
	}

	if home != "" {
		if err := walkRoot(home); err != nil {
			return out, err
		}
	}
	for _, proj := range opts.ExtraProjects {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		proj = strings.TrimSpace(proj)
		if proj == "" {
			continue
		}
		cleanProj := filepath.Clean(proj)
		if home != "" && (cleanProj == filepath.Clean(home) || within(cleanProj, filepath.Clean(home))) {
			continue
		}
		if err := walkRoot(cleanProj); err != nil {
			return out, err
		}
	}
	return out, nil
}

// NodeModules is an alias for Installs kept for callers and tests.
func NodeModules(ctx context.Context, home string) ([]inventory.ProjectInstall, error) {
	return Installs(ctx, home)
}

// electronResources reports that path is the resources folder of an
// Electron app, as an unpacked VS Code on Linux: its app holds a
// package.json and node_modules of the app, not a project.
func electronResources(path, name string) bool {
	if name != "resources" {
		return false
	}
	for _, app := range []string{"app", "app.asar"} {
		if _, err := os.Stat(filepath.Join(path, app)); err == nil {
			return true
		}
	}
	return false
}

var workspaceRootMarkers = []string{
	"pnpm-workspace.yaml",
	"turbo.json",
	"nx.json",
	"lerna.json",
	"go.work",
}

// enclosingProjectRoot returns the parent git repository or workspace root
// enclosing dir when dir is a nested subpackage (for example, moduix/packages/solid
// or hearth/website), or "" when dir itself is a repository root or standalone.
// It stops strictly below home so a dotfiles ~/.git never swallows user projects.
func enclosingProjectRoot(dir, home string, extraProjects []string) string {
	cleanDir := filepath.Clean(dir)
	cleanHome := ""
	if home != "" {
		cleanHome = filepath.Clean(home)
	}
	if _, err := os.Stat(filepath.Join(cleanDir, ".git")); err == nil {
		return ""
	}
	inHome := cleanHome != "" && within(cleanDir, cleanHome)
	const maxAscend = 8

	// 1. Nearest enclosing .git directory or file strictly below home.
	cur := cleanDir
	for range maxAscend {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		if cleanHome != "" && (parent == cleanHome || (inHome && !within(parent, cleanHome))) {
			break
		}
		if _, err := os.Stat(filepath.Join(parent, ".git")); err == nil {
			return parent
		}
		cur = parent
	}

	// 2. Nearest tracked project ancestor (for tracked non-git monorepos).
	bestExtra := ""
	for _, ep := range extraProjects {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		cleanEP := filepath.Clean(ep)
		if cleanEP == cleanDir || (cleanHome != "" && cleanEP == cleanHome) {
			continue
		}
		if within(cleanDir, cleanEP) && len(cleanEP) > len(bestExtra) {
			bestExtra = cleanEP
		}
	}
	if bestExtra != "" {
		return bestExtra
	}

	// 3. Nearest enclosing workspace root marker strictly below home.
	cur = cleanDir
	for range maxAscend {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		if cleanHome != "" && (parent == cleanHome || (inHome && !within(parent, cleanHome))) {
			break
		}
		if hasInstallManifest(parent, workspaceRootMarkers) {
			return parent
		}
		cur = parent
	}
	return ""
}
