//go:build !windows

package ai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func executable(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// Claude Code's native install names its version in the path, so it is
// not run; Codex from npm is asked, and the apps' PATH finds only it.
func TestDetectCommands(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	claude := executable(t, filepath.Join(home, ".local", "share", "claude", "versions", "1.0.83"))
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(claude, filepath.Join(bin, "claude")); err != nil {
		t.Fatal(err)
	}
	npm := filepath.Join(root, "npm", "bin")
	codex := executable(t, filepath.Join(npm, "codex"))
	term := sysenv.Env{"HOME=" + home, "PATH=" + bin + string(os.PathListSeparator) + npm}
	gui := sysenv.Env{"PATH=" + npm}
	r := &run.Fake{}
	r.Stdout("codex-cli 0.29.0\n", codex, "--version")

	found := byID(Detect(t.Context(), r, term, gui))
	got := found["claude"].Installations
	if len(got) != 1 || got[0].Version != "1.0.83" || got[0].OnPath != 1 || got[0].InGUI != 0 || got[0].Tool != "claude" {
		t.Errorf("claude: %+v", got)
	}
	got = found["codex"].Installations
	if len(got) != 1 || got[0].Version != "0.29.0" || got[0].OnPath != 1 || got[0].InGUI != 1 {
		t.Errorf("codex: %+v", got)
	}
	var ran [][]string
	for _, c := range r.Calls() {
		ran = append(ran, c.Args)
	}
	if want := [][]string{{codex, "--version"}}; !reflect.DeepEqual(ran, want) {
		t.Errorf("ran %q, want %q", ran, want)
	}
	if !found["claude"].Found() || found["gemini"].Found() {
		t.Error("found: claude must be, gemini must not")
	}
}

// A server's command is found on the terminal's PATH or at its path; the
// path of one by name is the first on PATH.
func TestMCPCommands(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	npx := executable(t, filepath.Join(bin, "npx"))
	server := executable(t, filepath.Join(root, "opt", "server"))
	tree(t, home, map[string]string{".cursor/mcp.json": `{"mcpServers": {
		"byName": {"command": "npx"}, "byPath": {"command": "` + server + `"},
		"gone": {"command": "` + filepath.Join(root, "opt", "gone") + `"}, "home": {"command": "~/bin/x"}}}`}, nil)
	got, errs := MCP(sysenv.Env{"HOME=" + home, "PATH=" + bin})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	type check struct {
		name, path string
		missing    bool
	}
	var checks []check
	for _, s := range got {
		checks = append(checks, check{s.Name, s.CommandPath, s.Missing})
	}
	want := []check{{"byName", npx, false}, {"byPath", server, false}, {"gone", "", true}, {"home", "", false}}
	if !reflect.DeepEqual(checks, want) {
		t.Errorf("got  %+v\nwant %+v", checks, want)
	}
}

// A mise shim leads to mise itself: the shim is asked, not mise, which
// would tell its own version.
func TestDetectShimVersion(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	mise := executable(t, filepath.Join(root, "bin", "mise"))
	shims := filepath.Join(home, ".local", "share", "mise", "shims")
	if err := os.MkdirAll(shims, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(shims, "gemini")
	if err := os.Symlink(mise, shim); err != nil {
		t.Fatal(err)
	}
	term := sysenv.Env{"HOME=" + home, "PATH=" + shims}
	r := &run.Fake{}
	r.Stdout("0.68.0\n", shim, "--version")
	r.Stdout("2026.11.1 linux-arm64\n", mise, "--version")
	got := byID(Detect(t.Context(), r, term, sysenv.Env{}))["gemini"].Installations
	if len(got) != 1 || !got[0].Source.Shim || got[0].Version != "0.68.0" {
		t.Errorf("gemini: %+v", got)
	}
}
