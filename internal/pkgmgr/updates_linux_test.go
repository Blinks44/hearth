package pkgmgr

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// detectIn is Detect without the machine's managers, as a runner's
// Linuxbrew, which are found in their fixed places.
func detectIn(ctx context.Context, f *run.Fake, env sysenv.Env, root string) (inventory.ToolState, error) {
	st, err := Detect(ctx, f, env, env)
	st.PackageManagers = slices.DeleteFunc(st.PackageManagers, func(m inventory.PackageManager) bool {
		return !strings.HasPrefix(m.Path, root)
	})
	return st, err
}

func TestDetectLocalUpdates(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"apk", "zypper"} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, id)
			if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
				t.Fatal(err)
			}
			installedArgs := []string{"info", "-v"}
			updateArgs := []string{"--no-network", "version", "--limit", "<"}
			if id == "zypper" {
				installedArgs = []string{"--no-refresh", "--xmlout", "search", "--installed-only", "--details", "--type", "package"}
				updateArgs = []string{"--non-interactive", "--no-refresh", "--xmlout", "list-updates", "--type", "package"}
			}
			f := &run.Fake{}
			f.Stdout("test version", path, "--version")
			installed := "nodejs-current-22.1.0-r0\n"
			if id == "zypper" {
				installed = `<stream><search-result><solvable-list><solvable name="libtest" edition="1.0-1" arch="x86_64"/></solvable-list></search-result></stream>`
			}
			f.Stdout(installed, append([]string{path}, installedArgs...)...)
			data, err := os.ReadFile(filepath.Join("testdata", id+"-updates"+map[string]string{"apk": ".txt", "zypper": ".xml"}[id]))
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{path}, updateArgs...)
			f.Stdout(string(data), args...)
			env := sysenv.Env{"PATH=" + root}
			st, err := detectIn(t.Context(), f, env, root)
			if err != nil || len(st.PackageManagers) != 1 {
				t.Fatalf("%+v, %v", st, err)
			}
			m := st.PackageManagers[0]
			if !m.UpdatesKnown || len(m.Warnings) != 0 || len(m.Packages) != 1 || m.Packages[0].Available == "" {
				t.Fatalf("%+v", m)
			}
			if !slices.ContainsFunc(f.Calls(), func(c run.Cmd) bool { return slices.Equal(c.Args, args) }) {
				t.Fatalf("unexpected query: %+v", f.Calls())
			}
			// A missing cache, failed command or truncated answer must preserve the
			// installed list and leave the update count unknown.
			for _, res := range []run.Result{
				{ExitCode: 1},
				{Stdout: data, Truncated: true},
				{Stdout: []byte("invalid")},
			} {
				f.Set(res, nil, args...)
				st, err = detectIn(t.Context(), f, env, root)
				if err != nil {
					t.Fatal(err)
				}
				m = st.PackageManagers[0]
				if m.UpdatesKnown || len(m.Warnings) != 1 || len(m.Packages) != 1 || m.Packages[0].Available != "" {
					t.Fatalf("%+v", m)
				}
			}
			f.Stdout(string(data), args...)
			f.Set(run.Result{ExitCode: 1}, nil, append([]string{path}, installedArgs...)...)
			st, err = detectIn(t.Context(), f, env, root)
			if err != nil || st.PackageManagers[0].UpdatesKnown || len(st.PackageManagers[0].Warnings) != 1 {
				t.Fatalf("failed installed query: %+v, %v", st, err)
			}
			f.Stdout(installed, append([]string{path}, installedArgs...)...)

			if id == "apk" {
				f.Set(run.Result{Stdout: []byte("Installed: Available:\n"), Stderr: []byte("WARNING: opening from cache: No such file or directory")}, nil, args...)
				st, err = detectIn(t.Context(), f, env, root)
				if err != nil || st.PackageManagers[0].UpdatesKnown || len(st.PackageManagers[0].Warnings) != 1 {
					t.Fatalf("%+v, %v", st, err)
				}
			}
		})
	}
}
