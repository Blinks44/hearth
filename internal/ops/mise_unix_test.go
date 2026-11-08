//go:build !windows

package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestHookMise(t *testing.T) {
	tests := []struct {
		name, shell string
		files       map[string]string // existing files, by name under home
		want        map[string]string // what each edited file becomes
	}{
		{
			name:  "zsh: activate in .zshrc, shims in .zprofile",
			shell: "/bin/zsh",
			files: map[string]string{".zshrc": "alias ll='ls -l'\n"},
			want: map[string]string{
				".zshrc":    "alias ll='ls -l'\n\n" + BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate zsh)\"\n" + BlockEnd + "\n",
				".zprofile": BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate zsh --shims)\"\n" + BlockEnd + "\n",
			},
		},
		{
			name:  "bash: activate in .bashrc, shims in .profile",
			shell: "/bin/bash",
			want: map[string]string{
				".bashrc":  BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate bash)\"\n" + BlockEnd + "\n",
				".profile": BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate bash --shims)\"\n" + BlockEnd + "\n",
			},
		},
		{
			name:  "bash: .profile, which the desktop reads, though there is .bash_profile",
			shell: "/usr/bin/bash",
			files: map[string]string{".bash_profile": ". ~/.profile\n"},
			want: map[string]string{
				".bashrc":  BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate bash)\"\n" + BlockEnd + "\n",
				".profile": BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate bash --shims)\"\n" + BlockEnd + "\n",
			},
		},
		{
			name:  "fish",
			shell: "/usr/bin/fish",
			want:  map[string]string{".config/fish/config.fish": BlockStart + "\n/opt/homebrew/bin/mise activate fish | source\n" + BlockEnd + "\n"},
		},
		{
			name:  "activated already by the user",
			shell: "/bin/zsh",
			files: map[string]string{".zshrc": "eval \"$(mise activate zsh)\"\n"},
		},
		{
			name:  "a comment does not activate",
			shell: "/bin/zsh",
			files: map[string]string{".zshrc": "# eval \"$(mise activate zsh)\"\n", ".zprofile": ""},
			want: map[string]string{
				".zshrc":    "# eval \"$(mise activate zsh)\"\n\n" + BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate zsh)\"\n" + BlockEnd + "\n",
				".zprofile": BlockStart + "\neval \"$(/opt/homebrew/bin/mise activate zsh --shims)\"\n" + BlockEnd + "\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for name, text := range tt.files {
				if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			steps, err := hookMise(sysenv.Env{"HOME=" + home, "SHELL=" + tt.shell}, "/opt/homebrew/bin/mise")
			if err != nil {
				t.Fatal(err)
			}
			if len(steps) != len(tt.want) {
				t.Fatalf("%d steps, want %d: %+v", len(steps), len(tt.want), steps)
			}
			for _, step := range steps {
				name, _ := filepath.Rel(home, step.Edit.Path)
				_, existed := tt.files[name]
				if step.Edit.After != tt.want[name] || step.Edit.New == existed {
					t.Errorf("%s (new %v) becomes\n%q\nwant\n%q", name, step.Edit.New, step.Edit.After, tt.want[name])
				}
			}
		})
	}
	if _, err := hookMise(sysenv.Env{"HOME=/h", "SHELL=/bin/tcsh"}, "mise"); err == nil {
		t.Error("tcsh hooked")
	}
	// mise's installer puts it under the home folder, written as $HOME.
	home := t.TempDir()
	steps, _ := hookMise(sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}, filepath.Join(home, ".local/bin/mise"))
	if want := BlockStart + "\neval \"$($HOME/.local/bin/mise activate zsh)\"\n" + BlockEnd + "\n"; steps[0].Edit.After != want {
		t.Errorf("got %q, want %q", steps[0].Edit.After, want)
	}
}

func TestFindMise(t *testing.T) {
	root := t.TempDir()
	defer func(p []string) { brewPrefixes = p }(brewPrefixes)
	brewPrefixes = []string{filepath.Join(root, "brew")}
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	env := sysenv.Env{"HOME=" + home, "PATH=" + bin}
	if got := findMise(env); got != "" {
		t.Fatalf("found %q in nothing", got)
	}
	installed := filepath.Join(root, "brew", "bin", "mise")
	write(t, installed)
	if got := findMise(env); got != installed {
		t.Errorf("Homebrew's: %q", got)
	}
	onPath := filepath.Join(bin, "mise")
	write(t, onPath)
	if got := findMise(env); got != onPath {
		t.Errorf("on PATH first: %q", got)
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
