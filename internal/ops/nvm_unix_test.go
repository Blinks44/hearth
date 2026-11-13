//go:build !windows

package ops

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestNvmMovePlan(t *testing.T) {
	home := t.TempDir()
	mise := filepath.Join(home, ".local", "bin", "mise")
	if err := os.MkdirAll(filepath.Dir(mise), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mise, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".zshrc")
	text := "alias ll='ls -l'\n" +
		"export NVM_DIR=\"$HOME/.nvm\"\n" +
		"[ -s \"$NVM_DIR/nvm.sh\" ] && \\. \"$NVM_DIR/nvm.sh\"  # This loads nvm\n" +
		"[ -s \"$NVM_DIR/bash_completion\" ] && \\. \"$NVM_DIR/bash_completion\"\n" +
		"# nvm use 18\n" +
		"export PATH=\"$HOME/bin:$PATH\"\n"
	if err := os.WriteFile(rc, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin"}
	node := inventory.ToolState{Tool: "node", Installations: []inventory.Installation{
		{Version: "20.18.1", Source: inventory.Source{Manager: "nvm"}},
		{Version: "22.12.0", Source: inventory.Source{Manager: "nvm"}, Default: true},
	}}
	line := inventory.StartupLine{File: rc, Line: 3}

	p, err := NvmMovePlan(line, node, "Node.js", env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != locales.NvmMoveTitle() {
		t.Errorf("title %q", p.Title)
	}
	use := slices.IndexFunc(p.Steps, func(s Step) bool { return slices.Contains(s.Argv, "node@22.12.0") })
	if use < 0 || p.Steps[use].Argv[0] != mise {
		t.Fatalf("no step has mise install Node 22.12.0: %+v", p.Steps)
	}
	last := p.Steps[len(p.Steps)-1]
	if last.Edit == nil || last.Edit.Path != rc || use > len(p.Steps)-2 {
		t.Fatalf("last step is not the edit of .zshrc after mise: %+v", p.Steps)
	}
	// mise's hook goes in .zshrc first, and the edit starts from it.
	if !strings.Contains(last.Edit.Before, "mise activate zsh") {
		t.Errorf("the edit does not start from mise's hook:\n%s", last.Edit.Before)
	}
	got := splitLines(last.Edit.After)
	for i, want := range []bool{false, true, true, true, false, false} {
		if commented := strings.HasPrefix(got[i], "# hearth: "); commented != want {
			t.Errorf("line %d %q: commented %v, want %v", i+1, got[i], commented, want)
		}
	}
	if !strings.Contains(last.Edit.After, "mise activate zsh)\"\n"+BlockEnd) {
		t.Errorf("mise's hook was touched:\n%s", last.Edit.After)
	}

	t.Run("the line is not nvm's any more", func(t *testing.T) {
		if _, err := NvmMovePlan(inventory.StartupLine{File: rc, Line: 1}, node, "Node.js", env, nil); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Errorf("err = %v, want the file changed", err)
		}
	})
	t.Run("nvm without a default version", func(t *testing.T) {
		p, err := NvmMovePlan(line, inventory.ToolState{Tool: "node"}, "Node.js", env, nil)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(p.Steps, func(s Step) bool { return len(s.Argv) > 1 && s.Argv[1] == "use" }) {
			t.Errorf("mise installs a version nvm does not have: %+v", p.Steps)
		}
	})
}
