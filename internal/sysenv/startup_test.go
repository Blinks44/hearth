package sysenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestStartupCulprit(t *testing.T) {
	tests := []struct {
		line string
		want Culprit
	}{
		{`[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm`, CulpritNvm},
		{`# [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"`, ""},
		{`__conda_setup="$('/opt/miniconda3/bin/conda' 'shell.zsh' 'hook' 2> /dev/null)"`, CulpritConda},
		{`(& "C:\Users\me\miniconda3\Scripts\conda.exe" "shell.powershell" "hook") | Out-String | ?{$_} | Invoke-Expression`, CulpritConda},
		{`eval "$(pyenv init -)"`, CulpritPyenv},
		{`eval "$(pyenv virtualenv-init -)"`, CulpritPyenv},
		{`eval "$(rbenv init - zsh)"`, CulpritRbenv},
		{`[[ -s "$HOME/.rvm/scripts/rvm" ]] && source "$HOME/.rvm/scripts/rvm"`, CulpritRvm},
		{`[[ -s "$HOME/.sdkman/bin/sdkman-init.sh" ]] && source "$HOME/.sdkman/bin/sdkman-init.sh"`, CulpritSdkman},
		{`source $ZSH/oh-my-zsh.sh`, CulpritOhMyZsh},
		{`oh-my-posh init pwsh | Invoke-Expression`, CulpritOhMyPosh},
		{`Import-Module posh-git`, CulpritPoshGit},
		{`export PATH="$(brew --prefix)/opt/python/libexec/bin:$PATH"`, CulpritBrew},
		{`autoload -Uz compinit && compinit`, CulpritCompinit},
		{`compinit`, CulpritCompinit},
		{`autoload -Uz compinit && compinit -C`, ""},
		{`compinit -u -C`, ""},
		{`export PATH="$HOME/.local/bin:$PATH"`, ""},
	}
	for _, tt := range tests {
		if got := StartupCulprit(tt.line); got != tt.want {
			t.Errorf("StartupCulprit(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

// TestAttribute reads traces zsh 5.9 and bash 5.2 wrote starting with a
// .zshrc or .bashrc that sources a file sleeping 0.3 s, evaluates code
// sleeping 0.2 s and calls a function sleeping 0.1 s, on lines 2, 3 and 7.
func TestAttribute(t *testing.T) {
	tests := []struct {
		name, fixture string
		startup       []string
		rc            string
		files         []string
	}{
		{
			name: "zsh", fixture: "startup-zsh.txt", rc: "/home/u/.zshrc",
			startup: []string{"/etc/zshenv", "/etc/zprofile", "/etc/zshrc", "/home/u/.zprofile", "/home/u/.zshrc"},
			files:   []string{"/etc/zprofile", "/home/u/.zprofile", "/etc/zshrc", "/home/u/.zshrc"},
		},
		{
			name: "bash", fixture: "startup-bash.txt", rc: "/home/u/.bashrc",
			startup: []string{"/etc/profile", "/etc/bash.bashrc", "/home/u/.bash_profile", "/home/u/.bashrc"},
			files:   []string{"/etc/profile", "/etc/bash.bashrc", "/home/u/.bash_profile", "/home/u/.bashrc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			startup := func(f string) bool {
				return slices.Contains(tt.startup, f) || tt.name == "bash" && strings.HasPrefix(f, "/etc/profile.d/")
			}
			files, lines, took := attribute(parseTrace(out, "M"), startup)
			var names []string
			for _, f := range files {
				if !strings.HasPrefix(f.File, "/etc/profile.d/") {
					names = append(names, f.File)
				}
			}
			if !slices.Equal(names, tt.files) {
				t.Errorf("files = %q, want %q", names, tt.files)
			}
			slices.SortFunc(lines, func(a, b inventory.StartupLine) int { return int(b.Took - a.Took) })
			want := []struct {
				line     int
				from, to time.Duration
			}{{2, 300 * time.Millisecond, 320 * time.Millisecond}, {3, 200 * time.Millisecond, 220 * time.Millisecond}, {7, 100 * time.Millisecond, 120 * time.Millisecond}}
			for i, w := range want {
				l := lines[i]
				if l.File != tt.rc || l.Line != w.line || l.Took < w.from || l.Took > w.to {
					t.Errorf("line %d = %s:%d %v, want %s:%d in [%v, %v]", i, l.File, l.Line, l.Took, tt.rc, w.line, w.from, w.to)
				}
			}
			var sum time.Duration
			for _, f := range files {
				sum += f.Took
			}
			if sum != took || took < 600*time.Millisecond {
				t.Errorf("took %v, files sum to %v", took, sum)
			}
		})
	}
}

func TestSlowest(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(rc, []byte("export A=1\n  source ~/.nvm/nvm.sh  \ncompinit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ms := time.Millisecond
	files := []inventory.StartupFile{{File: rc, Took: 500 * ms}}
	lines := []inventory.StartupLine{{File: rc, Line: 1, Took: 4 * ms}, {File: rc, Line: 3, Took: 100 * ms}, {File: rc, Line: 2, Took: 396 * ms}}
	// The trace took twice the measured start.
	files, lines = slowest(files, lines, 500*ms, 250*ms)
	if files[0].Took != 250*ms {
		t.Errorf("file took %v, want 250ms", files[0].Took)
	}
	want := []inventory.StartupLine{{File: rc, Line: 2, Text: "source ~/.nvm/nvm.sh", Took: 198 * ms}, {File: rc, Line: 3, Text: "compinit", Took: 50 * ms}}
	if !slices.Equal(lines, want) {
		t.Errorf("lines = %+v, want %+v", lines, want)
	}
}
