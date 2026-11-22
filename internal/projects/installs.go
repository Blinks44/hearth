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
	manifests []string
	files     []string
}

// installSpecs are the dependency or build folders tools put back from a
// project's manifest, with the manifests that confirm the folder belongs
// to that ecosystem and the files that tell when the project changed.
var installSpecs = []installSpec{
	{
		name:      "node_modules",
		manifests: []string{"package.json"},
		files:     []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"},
	},
	{
		name:      ".venv",
		manifests: []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg", "uv.lock", "poetry.lock"},
		files:     []string{"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock", "setup.py", "setup.cfg", "uv.lock", "poetry.lock", ".python-version"},
	},
	{
		name:      "target",
		manifests: []string{"Cargo.toml", "pom.xml"},
		files:     []string{"Cargo.toml", "Cargo.lock", "rust-toolchain.toml", "rust-toolchain", "pom.xml"},
	},
	{
		name:      "vendor",
		manifests: []string{"composer.json"},
		files:     []string{"composer.json", "composer.lock"},
	},
}

func installSpecByName(name string) (installSpec, bool) {
	for _, s := range installSpecs {
		if s.name == name {
			return s, true
		}
	}
	return installSpec{}, false
}

// hasInstallManifest reports whether dir contains at least one regular file from names.
func hasInstallManifest(dir string, names []string) bool {
	for _, name := range names {
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

// Installs finds dependency and build folders under home that sit next
// to a matching project manifest (node_modules next to package.json,
// .venv next to Python manifests, target next to Cargo.toml or pom.xml,
// vendor next to composer.json or go.mod), with when the project last
// changed. It never goes into matched folders, node_modules, hidden
// folders other than .venv, tests' testdata, app bundles, Electron app
// resources, the folders in homeSkipped, nor another file system mounted
// under home, as OrbStack's volumes in ~/OrbStack or a network share; it
// does not follow symlinks.
// When ctx ends it stops, with what it found so far and the error.
func Installs(ctx context.Context, home string) ([]inventory.ProjectInstall, error) {
	skip := map[string]bool{}
	for _, name := range homeSkipped {
		skip[filepath.Join(home, name)] = true
	}
	var homeDev uint64
	homeKnown := false
	if info, err := os.Stat(home); err == nil {
		homeDev, homeKnown = deviceOf(info)
	}
	var out []inventory.ProjectInstall
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || !d.IsDir() {
			// A folder that cannot be listed is left out.
			return nil
		}
		name := d.Name()
		if path == home {
			return nil
		}
		if name == "testdata" || skip[path] || strings.HasSuffix(name, ".app") || electronResources(path, name) {
			return filepath.SkipDir
		}
		if info, err := d.Info(); err == nil && homeKnown {
			if dev, ok := deviceOf(info); ok && dev != homeDev {
				return filepath.SkipDir
			}
		}
		if spec, ok := installSpecByName(name); ok {
			dir := filepath.Dir(path)
			if hasInstallManifest(dir, spec.manifests) {
				out = append(out, inventory.ProjectInstall{Project: dir, Name: name, Changed: changed(Project{Dir: dir, Files: spec.files})})
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
	return out, err
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
