package projects

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// nodeFiles are the files of a Node project that tell when it changed.
var nodeFiles = []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"}

// homeSkipped are the folders under home that hold apps' and tools' own
// files, never the user's projects: macOS's Library, Windows' AppData,
// snap's data, Scoop's apps, Go's module cache and conda's installs. The
// media folders hold no projects either, and on macOS reading Music and
// Pictures asks the user for access to Apple Music and Photos.
var homeSkipped = []string{"Library", "AppData", "snap", "scoop", filepath.Join("go", "pkg"),
	"miniconda3", "anaconda3", "miniforge3", "mambaforge", "Music", "Pictures", "Movies", "Videos"}

// deviceOf is device, which tests replace to put a folder on a mount.
var deviceOf = device

// NodeModules finds the node_modules folders under home that sit next to
// a package.json, as a project installs them, with when the project last
// changed. It never goes into a node_modules, a hidden folder (tools' own,
// as ~/.npm, ~/.nvm or ~/.vscode), tests' testdata, an app bundle, an
// Electron app's resources, the folders in homeSkipped, nor another file
// system mounted under home, as OrbStack's volumes in ~/OrbStack or a
// network share; it does not follow symlinks.
// When ctx ends it stops, with what it found so far and the error.
func NodeModules(ctx context.Context, home string) ([]inventory.ProjectInstall, error) {
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
		if strings.HasPrefix(name, ".") || name == "testdata" || skip[path] || strings.HasSuffix(name, ".app") || electronResources(path, name) {
			return filepath.SkipDir
		}
		if info, err := d.Info(); err == nil && homeKnown {
			if dev, ok := deviceOf(info); ok && dev != homeDev {
				return filepath.SkipDir
			}
		}
		if name != "node_modules" {
			return nil
		}
		dir := filepath.Dir(path)
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			out = append(out, inventory.ProjectInstall{Project: dir, Name: name, Changed: changed(Project{Dir: dir, Files: nodeFiles})})
		}
		return filepath.SkipDir
	})
	return out, err
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
