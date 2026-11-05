//go:build !windows

package sysenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathSources(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
		line       int
	}{
		{"export", "# comment\nexport PATH=\"$HOME/bin:$PATH\"", true, 2},
		{"zsh array", "path=($HOME/bin $path)", true, 1},
		{"zsh append", "path+=(~/bin)", true, 1},
		{"fish", "fish_add_path ~/bin", true, 1},
		{"fish set", "set -gx PATH ~/bin $PATH", true, 1},
		{"other variable", "export PYTHONPATH=$HOME/bin", false, 0},
		{"comment", "# export PATH=$HOME/bin:$PATH", false, 0},
		{"unrelated", "echo $HOME/bin", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "rc")
			if err := os.WriteFile(file, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			got := sourceFiles(Env{"HOME=" + root}, []string{file})[filepath.Join(root, "bin")]
			if (len(got) > 0) != tc.want {
				t.Fatalf("sources: %+v", got)
			}
			if tc.want && (got[0].File != file || got[0].Line != tc.line) {
				t.Fatal(got)
			}
		})
	}
}

func TestPathSourceUnknownExpansion(t *testing.T) {
	for _, body := range []string{
		"export PATH=$UNKNOWN/bin:$PATH",
		"export PATH=\"$(brew --prefix)/bin:$PATH\"",
		"export PATH=\"`brew --prefix`/bin:$PATH\"",
	} {
		file := filepath.Join(t.TempDir(), "rc")
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if got := sourceFiles(nil, []string{file}); len(got) != 0 {
			t.Errorf("%s: computed directory attributed: %+v", body, got)
		}
	}
}

// The line Homebrew's installer asks for puts its bin and sbin on PATH.
func TestPathSourceBrewShellenv(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".zprofile")
	if err := os.WriteFile(file, []byte("\neval \"$(/opt/homebrew/bin/brew shellenv zsh)\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := sourceFiles(nil, []string{file})
	for _, dir := range []string{"/opt/homebrew/bin", "/opt/homebrew/sbin"} {
		if s := got[dir]; len(s) != 1 || s[0].Line != 2 {
			t.Errorf("%s: %+v", dir, s)
		}
	}
}
