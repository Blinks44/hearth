//go:build !windows

package detect

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// A version mise installed is found though nothing puts it on the PATH,
// as mise install leaves it: pnpm in the version's folder, Yarn in its
// bin, its version by the folder.
func TestJSMiseInstalls(t *testing.T) {
	for name, exe := range map[string]string{"pnpm": "pnpm", "yarn": filepath.Join("bin", "yarn")} {
		t.Run(name, func(t *testing.T) {
			root := root(t)
			dir := filepath.Join(root, "home", ".local", "share", "mise", "installs", name, "10.4.1")
			path := fakeExe(t, filepath.Join(dir, exe))
			symlink(t, dir, filepath.Join(filepath.Dir(dir), "latest"))
			r := &run.Fake{}

			got, err := jsManager(t.Context(), r, name, homeEnv(root, "bin"), sysenv.Env{"PATH="})
			if err != nil {
				t.Fatal(err)
			}
			checkInstallations(t, got, []inventory.Installation{{
				Tool: name, Version: "10.4.1", Path: path, RealPath: path,
				Source: inventory.Source{Kind: vm, Manager: "mise", Root: dir, Version: "10.4.1"},
			}})
			if len(got.Managers) != 1 || got.Managers[0].Name != "mise" || !slices.Contains(got.Managers[0].Files, "mise.toml") {
				t.Errorf("managers = %+v, want mise, which picks the version by mise.toml", got.Managers)
			}
			if calls := r.Calls(); len(calls) != 0 {
				t.Errorf("ran %v; the version is in the path", calls)
			}
		})
	}
}
