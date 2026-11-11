package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestSetupParts(t *testing.T) {
	apt := []inventory.PackageManager{{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}}
	tests := []struct {
		name string
		s    Setup
		want []string // tool:manager, in order
	}{
		{
			name: "a new Mac: the base first, each after what it needs",
			s:    Setup{GOOS: "darwin", Tools: []string{"claude", "pnpm", "docker", "git", "node", "mise", "homebrew"}},
			want: []string{"homebrew:installer", "mise:brew", "node:mise", "pnpm:mise", "git:xcode-select", "docker:brew", "claude:installer"},
		},
		{
			name: "what is there already does not install again",
			s:    Setup{GOOS: "darwin", Tools: []string{"homebrew", "mise", "node", "pnpm"}, Have: map[string]bool{"homebrew": true, "mise": true, "node": true}},
			want: []string{"pnpm:mise"},
		},
		{
			name: "what a tool needs comes along",
			s:    Setup{GOOS: "linux", Tools: []string{"gemini"}, Managers: apt},
			want: []string{"mise:installer", "node:mise", "gemini:npm"},
		},
		{
			name: "the engine chosen",
			s:    Setup{GOOS: "darwin", Tools: []string{"docker"}, Have: map[string]bool{"homebrew": true}, Engine: "orbstack"},
			want: []string{"docker:brew:orbstack"},
		},
		{
			name: "the system's manager found",
			s:    Setup{GOOS: "linux", Tools: []string{"php", "git"}, Managers: apt},
			want: []string{"php:apt", "git:apt"},
		},
		{
			name: "no manager to install through",
			s:    Setup{GOOS: "linux", Tools: []string{"php"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, p := range tt.s.Parts() {
				s := p.Tool + ":" + p.Method.Manager
				if tt.s.Engine != "" {
					s += ":" + p.Method.Package
				}
				got = append(got, s)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("parts %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChainEdits(t *testing.T) {
	first := AddToBlock("alias ll='ls -l'\n", "one")
	steps := []Step{
		{Edit: &FileEdit{Path: "/h/.zshrc", Before: "alias ll='ls -l'\n", After: first}},
		{Argv: []string{"true"}},
		{Edit: &FileEdit{Path: "/h/.zshrc", Before: "alias ll='ls -l'\n", After: AddToBlock("alias ll='ls -l'\n", "two")}},
		{Edit: &FileEdit{Path: "/h/.zprofile", After: AddToBlock("", "three"), New: true}},
	}
	chainEdits(steps)
	second := steps[2].Edit
	if second.Before != first || second.After != AddToBlock(first, "two") || second.New {
		t.Errorf("the second edit of .zshrc: %+v", second)
	}
	if !steps[3].Edit.New || steps[3].Edit.Before != "" {
		t.Errorf("another file changed: %+v", steps[3].Edit)
	}
}
