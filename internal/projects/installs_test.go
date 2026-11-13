package projects

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Only a node_modules next to a package.json is a project's; tools',
// apps' and the ones inside node_modules are not.
func TestNodeModules(t *testing.T) {
	home := t.TempDir()
	// mk makes the files named package.json and the folders of the others.
	mk := func(rel ...string) {
		t.Helper()
		for _, r := range rel {
			path := filepath.Join(home, filepath.FromSlash(r))
			dir := path
			if filepath.Base(path) == "package.json" {
				dir = filepath.Dir(path)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if dir != path {
				if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	project := func(dir string) { mk(dir+"/package.json", dir+"/node_modules") }
	project("dev/web")
	// A monorepo: the root's and a package's, not the ones inside them.
	project("dev/mono")
	project("dev/mono/packages/ui")
	project("dev/mono/node_modules/left-pad")
	mk("dev/mono/node_modules/left-pad/node_modules/x/package.json")
	// No package.json next to it: not a project's.
	mk("dev/loose/node_modules")
	// Global packages of a prefix under home.
	mk("n/lib/node_modules/typescript/package.json")
	project(".npm-global/lib/node_modules/typescript")
	// A test's fixture, as Find leaves out.
	project("dev/web/testdata/app")
	// Another file system mounted under home: OrbStack's volumes.
	project("OrbStack/docker/volumes/app_node_modules/glob")
	// Tools', editors' and apps' own.
	project(".nvm/versions/node/v22.0.0/lib/node_modules/npm")
	project(".vscode/extensions/ms-python.python-2024.0.0")
	project("Library/Application Support/Code/User/globalStorage/x")
	project("AppData/Roaming/npm/node_modules/pnpm")
	project("go/pkg/mod/github.com/x/y@v1.0.0/web")
	project("miniconda3/share/jupyter/lab/staging")
	project("Applications/Tool.app/Contents/Resources/app")
	project("Downloads/VSCode-linux-x64/resources/app")

	old := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(home, "dev", "web", "package.json"), old, old); err != nil {
		t.Fatal(err)
	}
	device := deviceOf
	t.Cleanup(func() { deviceOf = device })
	deviceOf = func(info fs.FileInfo) (uint64, bool) {
		if info.Name() == "OrbStack" {
			return 99, true
		}
		return 1, true
	}
	got, err := NodeModules(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, in := range got {
		rel, _ := filepath.Rel(home, in.Project)
		dirs = append(dirs, filepath.ToSlash(rel))
		if in.Name != "node_modules" || in.Changed.IsZero() {
			t.Errorf("%s: %+v", rel, in)
		}
		if rel == filepath.Join("dev", "web") && !in.Changed.Equal(old) {
			t.Errorf("web changed %v, want %v", in.Changed, old)
		}
	}
	slices.Sort(dirs)
	if want := []string{"dev/mono", "dev/mono/packages/ui", "dev/web"}; !slices.Equal(dirs, want) {
		t.Errorf("found %q, want %q", dirs, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NodeModules(ctx, home); err == nil {
		t.Error("no error after ctx ended")
	}
}
