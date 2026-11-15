package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstructions(t *testing.T) {
	for _, tt := range []struct{ name, a, c, want string }{
		{"missing", "", "", "missing"}, {"agents", "Rules", "", "agents"}, {"claude", "", "Rules", "claude"},
		{"same", "Rules\r\n", "Rules\n", "same"}, {"different", "Build", "Test", "different"},
		{"import", "Rules", "@AGENTS.md\n\nLocal rules", "linked"},
		{"fenced example", "Rules", "```md\n@AGENTS.md\n```", "different"},
		{"comment", "Rules", "<!--\n@AGENTS.md\n-->", "different"},
		{"indented example", "Rules", "    @AGENTS.md", "different"},
		{"inline example", "Rules", "`@AGENTS.md`", "different"},
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
			p := Read(dir)
			if got := p.Instructions.State(); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestInstructionReadFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{{"large", make([]byte, 256*1024+1)}, {"binary", []byte{0xff}}} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), tt.data, 0600); err != nil {
				t.Fatal(err)
			}
			if ReadInstructions(dir).State() != "unreadable" {
				t.Fatal("unreadable file treated as usable")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Symlink("gone", filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Skip(err)
	}
	in := ReadInstructions(dir)
	if !in.Claude.Link || in.State() != "unreadable" {
		t.Fatalf("broken link: %+v", in)
	}
}
