//go:build !windows

package sysenv

import (
	"os"
	"path/filepath"
	"strings"
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

// RVM's .bash_profile sources .profile: its lines still have a source.
func TestPathSourcesBashProfile(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{".bash_profile": "source ~/.profile\n", ".profile": "export PATH=\"$PATH:/opt/go/bin\"\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := pathSources(Env{"HOME=" + root, "SHELL=/bin/bash"})["/opt/go/bin"]
	if len(got) != 1 || got[0].File != filepath.Join(root, ".profile") {
		t.Fatal(got)
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

func TestPathSourcesFollowsIncludes(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, body string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	zprofile := mustWrite(".zprofile", strings.Join([]string{
		"source ~/.orbstack/shell/init.zsh 2>/dev/null || :",
		`[ -s "$HOME/.cargo/env" ] && . "$HOME/.cargo/env"`,
		"source ~/.cycle-a.zsh",
		"# source ~/.commented.zsh",
		"source ~/.glob/*.zsh",
	}, "\n")+"\n")
	orb := mustWrite(".orbstack/shell/init.zsh", "export PATH=\"$PATH\":"+filepath.Join(root, ".orbstack/bin")+"\n")
	cargo := mustWrite(".cargo/env", "export PATH=\"$HOME/.cargo/bin:$PATH\"\n")
	mustWrite(".cycle-a.zsh", "source ~/.cycle-b.zsh\n")
	mustWrite(".cycle-b.zsh", "source ~/.cycle-a.zsh\nexport PATH=\"$HOME/cycle/bin:$PATH\"\n")
	mustWrite(".commented.zsh", "export PATH=\"$HOME/nope/bin:$PATH\"\n")

	got := sourceFiles(Env{"HOME=" + root}, []string{zprofile})
	if s := got[filepath.Join(root, ".orbstack/bin")]; len(s) != 1 || s[0].File != orb || s[0].Line != 1 {
		t.Errorf("orbstack source = %+v, want %s:1", s, orb)
	}
	if s := got[filepath.Join(root, ".cargo/bin")]; len(s) != 1 || s[0].File != cargo || s[0].Line != 1 {
		t.Errorf("cargo source = %+v, want %s:1", s, cargo)
	}
	if s := got[filepath.Join(root, "cycle/bin")]; len(s) != 1 || s[0].Line != 2 {
		t.Errorf("cycle source = %+v, want line 2", s)
	}
	if s := got[filepath.Join(root, "nope/bin")]; len(s) != 0 {
		t.Errorf("commented source should be ignored: %+v", s)
	}
}

func TestPathSourceManagerHooks(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, ".zprofile")
	body := strings.Join([]string{
		`eval "$(/opt/homebrew/bin/mise activate zsh --shims)"`,
		`eval "$(/opt/homebrew/bin/mise activate zsh)"`,
		`eval "$(pyenv init -)"`,
		`eval "$(rbenv init - zsh)"`,
	}, "\n") + "\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	got := sourceFiles(Env{"HOME=" + root}, []string{file})
	for dir, wantLine := range map[string]int{
		filepath.Join(root, ".local/share/mise/shims"): 1,
		filepath.Join(root, ".pyenv/shims"):            3,
		filepath.Join(root, ".rbenv/shims"):            4,
	} {
		if s := got[dir]; len(s) != 1 || s[0].File != file || s[0].Line != wantLine {
			t.Errorf("%s: got %+v, want %s:%d", dir, s, file, wantLine)
		}
	}
	// Custom MISE_DATA_DIR overrides the default mise shims directory.
	custom := filepath.Join(root, "mise-data")
	gotCustom := sourceFiles(Env{"HOME=" + root, "MISE_DATA_DIR=" + custom}, []string{file})
	if s := gotCustom[filepath.Join(custom, "shims")]; len(s) != 1 || s[0].Line != 1 {
		t.Errorf("custom MISE_DATA_DIR shims: %+v", s)
	}
}

func TestPathSourceHearthBlock(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, ".zshrc")
	body := strings.Join([]string{
		`export PATH="$HOME/user-bin:$PATH"`,
		`# >>> hearth >>>`,
		`export PATH="$HOME/hearth-bin:$PATH"`,
		`# <<< hearth <<<`,
		`export PATH="$HOME/after-bin:$PATH"`,
	}, "\n") + "\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	got := sourceFiles(Env{"HOME=" + root}, []string{file})
	if s := got[filepath.Join(root, "user-bin")]; len(s) != 1 || s[0].Hearth {
		t.Errorf("user-bin should not be marked Hearth: %+v", s)
	}
	if s := got[filepath.Join(root, "hearth-bin")]; len(s) != 1 || !s[0].Hearth {
		t.Errorf("hearth-bin should be marked Hearth: %+v", s)
	}
	if s := got[filepath.Join(root, "after-bin")]; len(s) != 1 || s[0].Hearth {
		t.Errorf("after-bin should not be marked Hearth: %+v", s)
	}
}
