//go:build !windows

package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestPathAddPlanShells(t *testing.T) {
	home := t.TempDir()
	customDir := filepath.Join(home, "tools", "bin")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("# login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		shell     string
		raw       string
		wantFiles []string
		wantLine  string
	}{
		{
			name:      "zsh expands tilde and uses $HOME",
			shell:     "/bin/zsh",
			raw:       "~/tools/bin",
			wantFiles: []string{filepath.Join(home, ".zshrc")},
			wantLine:  `export PATH="$HOME/tools/bin:$PATH"`,
		},
		{
			name:      "bash updates bashrc and login profile",
			shell:     "/bin/bash",
			raw:       customDir,
			wantFiles: []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".profile")},
			wantLine:  `export PATH="$HOME/tools/bin:$PATH"`,
		},
		{
			name:      "fish writes set -gx",
			shell:     "/usr/bin/fish",
			raw:       customDir,
			wantFiles: []string{filepath.Join(home, ".config", "fish", "config.fish")},
			wantLine:  `set -gx PATH "$HOME/tools/bin" $PATH`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := sysenv.Env{"HOME=" + home, "SHELL=" + tt.shell}
			entries := []inventory.PathEntry{{Dir: "/usr/bin"}}
			p, err := PathAddPlan(entries, tt.raw, env, nil)
			if err != nil {
				t.Fatalf("PathAddPlan() error = %v", err)
			}
			if !p.RestartTerminal {
				t.Error("RestartTerminal = false, want true")
			}
			if len(p.Notes) != 0 {
				t.Errorf("unexpected notes for existing dir: %+v", p.Notes)
			}
			if len(p.Steps) != len(tt.wantFiles) {
				t.Fatalf("len(Steps) = %d, want %d", len(p.Steps), len(tt.wantFiles))
			}
			for i, wantFile := range tt.wantFiles {
				edit := p.Steps[i].Edit
				if edit == nil {
					t.Fatalf("step %d has nil Edit", i)
				}
				if edit.Path != wantFile {
					t.Errorf("step %d path = %q, want %q", i, edit.Path, wantFile)
				}
				if !strings.Contains(edit.After, tt.wantLine) {
					t.Errorf("step %d After = %q, want %q", i, edit.After, tt.wantLine)
				}
			}
		})
	}
}

func TestPathAddPlanValidationAndNotes(t *testing.T) {
	home := t.TempDir()
	existingFile := filepath.Join(home, "tool")
	if err := os.WriteFile(existingFile, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}
	entries := []inventory.PathEntry{{Dir: "/usr/local/bin"}}

	invalid := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"empty", "   ", locales.PathAddInvalid()},
		{"relative", "bin/tools", locales.PathAddInvalid()},
		{"colon", "/tmp/a:/tmp/b", locales.PathAddInvalid()},
		{"quotes", `/tmp/"bad"`, locales.PathAddInvalid()},
		{"already in PATH", "/usr/local/bin/", locales.PathAddExists("/usr/local/bin")},
		{"regular file", existingFile, locales.PathAddNotDir(existingFile)},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PathAddPlan(entries, tt.raw, env, nil)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}

	missingDir := filepath.Join(home, "does-not-exist")
	p, err := PathAddPlan(entries, missingDir, env, nil)
	if err != nil {
		t.Fatalf("missing dir should produce a plan with a note, got err = %v", err)
	}
	if len(p.Notes) != 1 || p.Notes[0].Title != locales.PathAddMissingTitle() {
		t.Fatalf("Notes = %+v, want missing directory note", p.Notes)
	}
}

func TestPathAddPlanVerify(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}

	okTerm := func(context.Context) (sysenv.Env, error) {
		return sysenv.Env{"PATH=" + dir + ":/usr/bin"}, nil
	}
	p, err := PathAddPlan(nil, dir, env, okTerm)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := p.Verify(context.Background())
	if err != nil || msg != locales.PathAddCheckOk(dir) {
		t.Fatalf("Verify ok = (%q, %v), want (%q, nil)", msg, err, locales.PathAddCheckOk(dir))
	}

	failTerm := func(context.Context) (sysenv.Env, error) {
		return sysenv.Env{"PATH=/usr/bin"}, nil
	}
	p, err = PathAddPlan(nil, dir, env, failTerm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != locales.PathAddCheckFail(dir) {
		t.Fatalf("Verify fail err = %v, want %q", err, locales.PathAddCheckFail(dir))
	}
}

func TestPathAddPlanBashLoginSourcesRC(t *testing.T) {
	t.Run("direct .profile -> .bashrc", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "mybin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		profile := filepath.Join(home, ".profile")
		if err := os.WriteFile(profile, []byte("if [ -f \"$HOME/.bashrc\" ]; then\n  . \"$HOME/.bashrc\"\nfi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env := sysenv.Env{"HOME=" + home, "SHELL=/bin/bash"}
		p, err := PathAddPlan(nil, dir, env, nil)
		if err != nil {
			t.Fatalf("PathAddPlan() error = %v", err)
		}
		if len(p.Steps) != 1 || p.Steps[0].Edit.Path != filepath.Join(home, ".bashrc") {
			t.Fatalf("expected only .bashrc edit when .profile sources .bashrc, got %+v", p.Steps)
		}
	})

	t.Run("transitive .bash_profile -> .profile -> .bashrc", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "mybin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte("[[ -s \"$HOME/.profile\" ]] && source \"$HOME/.profile\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("if [ -n \"$BASH_VERSION\" ]; then\n  . \"$HOME/.bashrc\"\nfi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env := sysenv.Env{"HOME=" + home, "SHELL=/bin/bash"}
		p, err := PathAddPlan(nil, dir, env, nil)
		if err != nil {
			t.Fatalf("PathAddPlan() error = %v", err)
		}
		if len(p.Steps) != 1 || p.Steps[0].Edit.Path != filepath.Join(home, ".bashrc") {
			t.Fatalf("expected only .bashrc edit when .bash_profile transitively sources .bashrc via .profile, got %+v", p.Steps)
		}
	})
}
