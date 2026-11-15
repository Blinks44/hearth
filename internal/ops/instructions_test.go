package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/projects"
)

func TestInstructionPlan(t *testing.T) {
	for _, tt := range []struct {
		name, a, c string
		ok         bool
	}{
		{"agents", "Build\n", "", true}, {"claude", "", "Test\n", true}, {"different", "Build\n", "Test\n", true},
		{"same", "Build\n", "Build\n", true}, {"linked", "Build\n", "@AGENTS.md\n", false},
		{"missing", "", "", false}, {"imports", "Build\n", "@../rules.md\n", false},
		{"comments", "Build\n", "<!-- note -->\nTest", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range map[string]string{"AGENTS.md": tt.a, "CLAUDE.md": tt.c} {
				if text != "" {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			plan, err := InstructionPlan(dir)
			if (err == nil) != tt.ok {
				t.Fatalf("plan err=%v, want ok=%v", err, tt.ok)
			}
			if err != nil {
				return
			}
			result := Execute(context.Background(), nil, plan, io.Discard, func(State) {})
			if result.Phase != Succeeded {
				t.Fatalf("execution: %+v", result)
			}
			in := projects.ReadInstructions(dir)
			if in.State() != "linked" {
				t.Fatalf("not linked: %+v", in)
			}
			if tt.c != "" && (!strings.Contains(in.Claude.Text, tt.c) || !strings.Contains(in.Agents.Text, strings.TrimSpace(tt.c))) {
				t.Fatal("original rules lost")
			}
			for _, step := range plan.Steps {
				if !step.Edit.New {
					matches, _ := filepath.Glob(step.Edit.Path + ".hearth-*.bak")
					if len(matches) != 1 {
						t.Fatalf("missing backup: %v", matches)
					}
				}
			}
			if _, err := InstructionPlan(dir); err == nil {
				t.Fatal("second plan should not duplicate instructions")
			}
		})
	}
}

func TestInstructionPlanChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("Before"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := InstructionPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The shared file is unchanged by this plan, but is a dependency of the import.
	// Verification must not report an empty shared source as connected.
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := Execute(context.Background(), nil, p, io.Discard, func(State) {})
	if r.Phase == Succeeded {
		t.Fatal("empty shared source accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("new file not rolled back")
	}
}

func TestInstructionEditRollback(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{"AGENTS.md": "Build", "CLAUDE.md": "Test"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := InstructionPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Changed"), 0600); err != nil {
		t.Fatal(err)
	}
	result := Execute(context.Background(), nil, p, io.Discard, func(State) {})
	if result.Phase != Failed || len(result.Restored) != 1 {
		t.Fatalf("failed operation: %+v", result)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(b) != "Build" {
		t.Fatal("shared file not restored")
	}
}
