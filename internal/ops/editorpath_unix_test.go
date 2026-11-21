//go:build !windows

package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestEditorPathPlanUnix(t *testing.T) {
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("export EDITOR=vim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmdDir := filepath.Join(home, "Library", "Application Support", "JetBrains", "Toolbox", "scripts")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdSrc := filepath.Join(cmdDir, "idea")
	if err := os.WriteFile(cmdSrc, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ed := inventory.Editor{ID: "IU", Name: "IntelliJ IDEA", Command: "idea", CommandSource: cmdSrc}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}
	terminal := func(context.Context) (sysenv.Env, error) {
		return sysenv.Env{"PATH=" + cmdDir}, nil
	}
	p, err := EditorPathPlan(ed, env, terminal)
	if err != nil {
		t.Fatal(err)
	}
	if !p.RestartTerminal || len(p.Steps) != 1 || p.Steps[0].Edit == nil {
		t.Fatalf("unexpected plan: %+v", p)
	}
	wantLine := `export PATH="$HOME/Library/Application Support/JetBrains/Toolbox/scripts:$PATH"`
	if !strings.Contains(p.Steps[0].Edit.After, wantLine) {
		t.Errorf("after = %q, want %q", p.Steps[0].Edit.After, wantLine)
	}
	got := Execute(t.Context(), &run.Fake{}, p, &strings.Builder{}, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("phase = %d, err = %q", got.Phase, got.Err)
	}
	data, err := os.ReadFile(zshrc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), wantLine) {
		t.Errorf(".zshrc missing line: %s", string(data))
	}
}

func TestEditorPathPlanRejectsInvalid(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}
	for _, ed := range []inventory.Editor{
		{Command: "", CommandSource: "/a/b"},
		{Command: "code", CommandSource: ""},
		{Command: "code", CommandSource: "relative/code"},
		{Command: "code", CommandSource: "/bad/$HOME/code"},
	} {
		if _, err := EditorPathPlan(ed, env, nil); err == nil {
			t.Errorf("expected error for %+v", ed)
		}
	}
}
