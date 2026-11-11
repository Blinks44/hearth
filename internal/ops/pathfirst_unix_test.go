//go:build !windows

package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPathFirstPlan(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, ".local", "bin")
	entries := []inventory.PathEntry{{Dir: "/opt/homebrew/bin"}, {Dir: "/usr/bin"}, {Dir: local}, {Dir: "/usr/bin"}, {Dir: "/gone", Missing: true}}
	block := func(lines ...string) string {
		return "\n" + BlockStart + "\n" + strings.Join(lines, "\n") + "\n" + BlockEnd + "\n"
	}
	tests := []struct {
		name, shell, file, before string
		dir                       string
		// want is what the file becomes, "" when there is no plan.
		want string
	}{
		{
			name: "zsh: a line in the block, $HOME kept", shell: "zsh", file: ".zshrc", before: "alias ll='ls -l'\n", dir: local,
			want: "alias ll='ls -l'\n" + block(`export PATH="$HOME/.local/bin:$PATH"`),
		},
		{
			name: "bash: a new .bashrc", shell: "bash", file: ".bashrc", dir: "/usr/bin",
			want: block(`export PATH="/usr/bin:$PATH"`)[1:],
		},
		{
			name: "fish", shell: "fish", file: ".config/fish/config.fish", dir: local,
			want: block(`set -gx PATH "$HOME/.local/bin" $PATH`)[1:],
		},
		{
			name: "a line there before goes after the others", shell: "zsh", file: ".zshrc", dir: local,
			before: block(`export PATH="$HOME/.local/bin:$PATH"`, `export PATH="/usr/bin:$PATH"`)[1:],
			want:   block(`export PATH="/usr/bin:$PATH"`, `export PATH="$HOME/.local/bin:$PATH"`)[1:],
		},
		{name: "already first", shell: "zsh", file: ".zshrc", dir: "/opt/homebrew/bin"},
		{name: "a missing folder", shell: "zsh", file: ".zshrc", dir: "/gone"},
		{name: "not in PATH", shell: "zsh", file: ".zshrc", dir: "/elsewhere"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(home, tt.file)
			_ = os.Remove(file)
			if tt.before != "" {
				if err := os.WriteFile(file, []byte(tt.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			env := sysenv.Env{"HOME=" + home, "SHELL=/bin/" + tt.shell}
			p, err := PathFirstPlan(entries, tt.dir, env, nil)
			if tt.want == "" {
				if !errors.Is(err, ErrNoFix) {
					t.Fatalf("err = %v, want ErrNoFix", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			e := p.Steps[0].Edit
			if len(p.Steps) != 1 || e.Path != file || e.Before != tt.before || e.New != (tt.before == "") {
				t.Fatalf("steps %+v", p.Steps)
			}
			if e.After != tt.want {
				t.Errorf("after\n%q\nwant\n%q", e.After, tt.want)
			}
		})
	}
}

// Putting an entry first checks a new terminal: the entry first, the
// rest in order; one that does not pass puts the file back.
func TestPathFirstVerify(t *testing.T) {
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	text := "alias ll='ls -l'\n"
	entries := []inventory.PathEntry{{Dir: "/opt/homebrew/bin"}, {Dir: "/usr/bin"}, {Dir: "/bin"}}
	for _, tt := range []struct {
		path string
		ok   bool
	}{
		{"/bin:/opt/homebrew/bin:/usr/bin", true},
		{"/bin:/opt/homebrew/bin:/usr/bin:/bin", true},
		{"/opt/homebrew/bin:/usr/bin:/bin", false},
		{"/bin:/usr/bin:/opt/homebrew/bin", false},
	} {
		if err := os.WriteFile(zshrc, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		terminal := func(context.Context) (sysenv.Env, error) { return sysenv.Env{"PATH=" + tt.path}, nil }
		p, err := PathFirstPlan(entries, "/bin", sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}, terminal)
		if err != nil {
			t.Fatal(err)
		}
		s := Execute(context.Background(), nil, p, &strings.Builder{}, func(State) {})
		data, _ := os.ReadFile(zshrc)
		if tt.ok && (s.Phase != Succeeded || string(data) == text) || !tt.ok && (s.Phase != NotVerified || string(data) != text) {
			t.Errorf("PATH %s: phase %v, err %q, file %q", tt.path, s.Phase, s.Err, data)
		}
	}
}

// On Windows the computer's Path stays ahead: the entry comes first after
// it.
func TestCheckFirstAhead(t *testing.T) {
	old := []string{"/sys", "/sys2", "/a", "/b", "/c"}
	for _, tt := range []struct {
		new []string
		ok  bool
	}{
		{[]string{"/sys", "/sys2", "/c", "/a", "/b"}, true},
		{[]string{"/c", "/sys", "/sys2", "/a", "/b"}, false},
		{[]string{"/sys", "/sys2", "/a", "/b", "/c"}, false},
	} {
		if _, err := checkFirst(old, tt.new, "/c", 2); (err == nil) != tt.ok {
			t.Errorf("%v: err = %v, want ok %v", tt.new, err, tt.ok)
		}
	}
}

// A folder a later line puts ahead again is named.
func TestCheckFirstAheadNamed(t *testing.T) {
	_, err := checkFirst([]string{"/nix", "/local", "/rvm"}, []string{"/rvm", "/nix", "/local"}, "/local", 0)
	if err == nil || !strings.Contains(err.Error(), "/rvm, /nix before /local") {
		t.Errorf("err = %v", err)
	}
}
