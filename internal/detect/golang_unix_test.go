//go:build !windows

package detect

import (
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fakeGo puts an executable go in root/bin and root's VERSION file, and
// returns its path.
func fakeGo(t *testing.T, root, version string) string {
	t.Helper()
	path := filepath.Join(root, "bin", goExe)
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "VERSION"), "go"+version+"\ntime 2025-01-01T00:00:00Z\n")
	return path
}

// mise's shim first in PATH, the old installer's copy behind it, and a
// toolchain go downloaded itself; versions all from VERSION files.
func TestGo(t *testing.T) {
	root := root(t)
	home := filepath.Join(root, "home")
	mise := filepath.Join(home, ".local", "share", "mise")
	miseGo := filepath.Join(mise, "installs", "go", "1.23.4")
	fakeGo(t, miseGo, "1.23.4")
	shim := filepath.Join(mise, "shims", goExe)
	writeFile(t, shim, "")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".config", "mise", "config.toml"), "[tools]\ngo = \"1.23\"\n")
	installer := filepath.Join(root, "local", "go")
	fakeGo(t, installer, "1.21.6")
	orig := goInstallDirs
	goInstallDirs = func(sysenv.Env) []string { return []string{installer} }
	t.Cleanup(func() { goInstallDirs = orig })
	toolchain := filepath.Join(home, "go", "pkg", "mod", "golang.org", "toolchain@v0.0.1-go1.22.10.darwin-arm64")
	fakeGo(t, toolchain, "1.22.10")

	// A command go install built, and a file that is not a Go program.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	info, err := buildinfo.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, "go", "bin", "gopls"), string(data))
	writeFile(t, filepath.Join(home, "go", "bin", "notes.txt"), "not a program")
	writeFile(t, goEnvFile(homeEnv(root), home), "GOTOOLCHAIN=local\n")

	term := append(homeEnv(root), "PATH="+filepath.Dir(shim)+string(os.PathListSeparator)+filepath.Join(installer, "bin"))
	r := &run.Fake{}
	got, err := Go(t.Context(), r, term, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	if calls := r.Calls(); len(calls) != 0 {
		t.Errorf("ran %v; versions are in VERSION files", calls)
	}
	type copy struct {
		path    string
		version string
		manager string
		shim    bool
		onPath  inventory.PathRank
		def     bool
	}
	want := []copy{
		{shim, "", "mise", true, 1, false},
		{filepath.Join(installer, "bin", goExe), "1.21.6", "", false, 2, false},
		{filepath.Join(miseGo, "bin", goExe), "1.23.4", "mise", false, 0, true},
		{filepath.Join(toolchain, "bin", goExe), "1.22.10", GoToolchain, false, 0, false},
	}
	if len(got.Installations) != len(want) {
		t.Fatalf("got %d copies: %+v", len(got.Installations), got.Installations)
	}
	for i, w := range want {
		in := got.Installations[i]
		g := copy{in.Path, in.Version, in.Source.Manager, in.Source.Shim, in.OnPath, in.Default}
		if g != w {
			t.Errorf("copy %d = %+v, want %+v", i, g, w)
		}
	}
	if tc := got.Installations[3]; tc.Source.Root != toolchain || tc.Source.Kind != inventory.KindVersionManager {
		t.Errorf("toolchain source %+v", tc.Source)
	}
	tools := got.Installations[1].Packages
	if len(tools) != 1 || tools[0].Name != "gopls" || tools[0].Module != info.Main.Path {
		t.Errorf("tools %+v, want gopls of %s", tools, info.Main.Path)
	}
	if len(got.Managers) != 1 || got.Managers[0].Name != "mise" || !got.Managers[0].Shims || got.Managers[0].ShimsInGUI {
		t.Errorf("managers %+v", got.Managers)
	}
	wantVars := map[string]string{
		"GOPATH":      filepath.Join(home, "go"),
		"GOMODCACHE":  filepath.Join(home, "go", "pkg", "mod"),
		"GOTOOLCHAIN": "local",
	}
	for k, v := range wantVars {
		if got.Vars[k] != v {
			t.Errorf("%s = %q, want %q", k, got.Vars[k], v)
		}
	}
}

// Without a VERSION file go is asked, kept from switching toolchains.
func TestGoRunVersion(t *testing.T) {
	r := &run.Fake{}
	r.Stdout("go version go1.22.2 linux/amd64\n", "/usr/bin/go", "version")
	if got := goRunVersion(t.Context(), r, sysenv.Env{"GOTOOLCHAIN=auto"}, "/usr/bin/go"); got != "1.22.2" {
		t.Errorf("version %q", got)
	}
	env := sysenv.Env(r.Calls()[0].Env)
	if env.Get("GOTOOLCHAIN") != "local" {
		t.Errorf("ran with GOTOOLCHAIN=%q", env.Get("GOTOOLCHAIN"))
	}
}

func TestReadGoEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env")
	writeFile(t, path, "# comment\nGOPROXY=https://proxy.example\nGOTOOLCHAIN = go1.24.0+auto\n")
	got := readGoEnvFile(path)
	if got["GOPROXY"] != "https://proxy.example" || got["GOTOOLCHAIN"] != "go1.24.0+auto" || len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

// GOBIN in GOROOT/bin, as mise sets it, holds go and gofmt: no utilities.
func TestGoToolsSkipsGo(t *testing.T) {
	gofmt, err := exec.LookPath("gofmt")
	if err != nil {
		t.Skip("no gofmt on PATH")
	}
	data, err := os.ReadFile(gofmt)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gofmt"), string(data))
	if tools := goTools(dir); len(tools) != 0 {
		t.Fatalf("tools %+v", tools)
	}
}
