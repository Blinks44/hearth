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
