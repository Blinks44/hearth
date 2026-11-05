//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// pyenv's shim first in PATH, picking its 3.12.8 by default; Homebrew's
// 3.13 behind it; a uv Python off PATH; tools from uv and pipx.
func TestPython(t *testing.T) {
	root := root(t)
	home := filepath.Join(root, "home")
	pyenv := filepath.Join(home, ".pyenv")
	shim := fakeExe(t, filepath.Join(pyenv, "shims", "python3"))
	pyenv312 := fakeExe(t, filepath.Join(pyenv, "versions", "3.12.8", "bin", "python3"))
	writeFile(t, filepath.Join(pyenv, "version"), "3.12\n")
	uvPython := fakeExe(t, filepath.Join(home, ".local", "share", "uv", "python", "cpython-3.11.11-macos-aarch64-none", "bin", "python3"))
	brew := filepath.Join(root, "brew")
	brewPython := fakeExe(t, filepath.Join(brew, "Cellar", "python@3.13", "3.13.1", "bin", "python3"))
	symlink(t, brewPython, filepath.Join(brew, "bin", "python3"))

	tools := filepath.Join(home, ".local", "share", "uv", "tools")
	writeFile(t, filepath.Join(tools, "ruff", "uv-receipt.toml"), "[tool]\nrequirements = [{ name = \"ruff\" }]\nentrypoints = [{ name = \"ruff\", install-path = \"/x/ruff\" }]\n")
	writeFile(t, filepath.Join(tools, "ruff", "lib", "python3.13", "site-packages", "ruff-0.9.2.dist-info", "METADATA"), "")
	writeFile(t, filepath.Join(home, ".local", "share", "pipx", "venvs", "httpie", "pipx_metadata.json"),
		`{"main_package":{"package":"httpie","package_version":"3.2.4","apps":["http","https"]}}`)
	writeFile(t, filepath.Join(home, ".config", "uv", "uv.toml"), "python-preference = \"only-managed\"\n")
	uv := fakeExe(t, filepath.Join(root, "bin", "uv"))

	sep := string(os.PathListSeparator)
	term := append(homeEnv(root), "HOMEBREW_PREFIX="+brew,
		"PATH="+filepath.Dir(shim)+sep+filepath.Join(brew, "bin")+sep+filepath.Dir(uv))
	r := &run.Fake{}
	r.Stdout("uv 0.5.21 (abc 2025-01-20)\n", uv, "--version")
	got, err := Python(t.Context(), r, term, sysenv.Env{"PATH=" + filepath.Join(brew, "bin")})
	if err != nil {
		t.Fatal(err)
	}
	type copy struct {
		path, version, manager string
		shim, def              bool
		onPath, inGUI          inventory.PathRank
	}
	var gotCopies []copy
	for _, in := range got.Installations {
		// This machine's own Pythons, in its fixed directories, aside.
		if strings.HasPrefix(in.RealPath, root) {
			gotCopies = append(gotCopies, copy{in.Path, in.Version, in.Source.Manager, in.Source.Shim, in.Default, in.OnPath, in.InGUI})
		}
	}
	want := []copy{
		{shim, "", "pyenv", true, false, 1, 0},
		{filepath.Join(brew, "bin", "python3"), "3.13.1", "brew", false, false, 2, 1},
		{pyenv312, "3.12.8", "pyenv", false, true, 0, 0},
		{uvPython, "3.11.11", "uv", false, false, 0, 0},
	}
	if len(gotCopies) != len(want) {
		t.Fatalf("got %+v", gotCopies)
	}
	for i := range want {
		if gotCopies[i] != want[i] {
			t.Errorf("copy %d = %+v, want %+v", i, gotCopies[i], want[i])
		}
	}
	// The machine's own Pythons, in their fixed places, are asked too.
	calls := slices.DeleteFunc(r.Calls(), func(c run.Cmd) bool { return !strings.HasPrefix(c.Args[0], root) })
	if len(calls) != 1 {
		t.Errorf("ran %v; versions are in the paths, and only uv is asked its own", calls)
	}
	wantTools := []inventory.Package{
		{Name: "ruff", Version: "0.9.2", Commands: []string{"ruff"}},
		{Name: "httpie", Version: "3.2.4", Commands: []string{"http", "https"}},
	}
	for _, in := range got.Installations {
		if strings.HasPrefix(in.RealPath, root) && !in.Source.Shim && !equalPackages(in.Packages, wantTools) {
			t.Errorf("%s tools %+v", in.Path, in.Packages)
		}
	}
	if len(got.Managers) != 2 {
		t.Fatalf("managers %+v", got.Managers)
	}
	if m := got.Managers[0]; m.Name != "pyenv" || !m.SwitchOnCD || m.Files[0] != ".python-version" {
		t.Errorf("pyenv %+v", m)
	}
	if m := got.Managers[1]; m.Name != "uv" || m.Version != "0.5.21" ||
		m.Settings["python-preference"] != "only-managed" || m.Settings["python-downloads"] != "automatic" {
		t.Errorf("uv %+v", m)
	}
}

func equalPackages(a, b []inventory.Package) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Version != b[i].Version || strings.Join(a[i].Commands, ",") != strings.Join(b[i].Commands, ",") {
			return false
		}
	}
	return true
}

// The environment wins over uv.toml.
func TestUvSettingsEnv(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config", "uv", "uv.toml"), "python-downloads = \"manual\"\n")
	got := uvSettings(sysenv.Env{"UV_PYTHON_DOWNLOADS=never"}, home)
	if got["python-downloads"] != "never" || got["python-preference"] != "managed" {
		t.Errorf("got %v", got)
	}
}
