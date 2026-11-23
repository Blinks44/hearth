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

func TestPathRemovable(t *testing.T) {
	home := "/Users/dev"
	rc := filepath.Join(home, ".zshrc")
	if PathRemovable(inventory.PathEntry{Dir: "/opt/bin", Sources: []inventory.PathSource{{File: rc, Line: 2}}}, home) {
		t.Error("non-Hearth source should not be removable")
	}
	if PathRemovable(inventory.PathEntry{Dir: "/opt/bin", Sources: []inventory.PathSource{{File: rc, Line: 2, Hearth: true, Hook: true}}}, home) {
		t.Error("hook source should not be removable")
	}
	if PathRemovable(inventory.PathEntry{Dir: "/opt/bin", Sources: []inventory.PathSource{{File: "/etc/profile", Line: 2, Hearth: true}}}, home) {
		t.Error("system file should not be removable")
	}
	if !PathRemovable(inventory.PathEntry{Dir: "/opt/bin", Sources: []inventory.PathSource{{File: rc, Line: 2, Hearth: true}}}, home) {
		t.Error("Hearth-managed user line should be removable")
	}
}

func TestPathRemovePlan(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(home, ".zshrc")
	firstDir := filepath.Join(home, "custom-a")
	secondDir := filepath.Join(home, "custom-b")
	userDir := filepath.Join(home, "user-bin")
	body := strings.Join([]string{
		`export PATH="$HOME/user-bin:$PATH"`,
		``,
		BlockStart,
		`export PATH="` + firstDir + `:$PATH"`,
		`export PATH="` + secondDir + `:$PATH"`,
		BlockEnd,
	}, "\n") + "\n"
	if err := os.WriteFile(rc, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}
	entries := []inventory.PathEntry{
		{Dir: userDir, Sources: []inventory.PathSource{{File: rc, Line: 1}}},
		{Dir: firstDir, Sources: []inventory.PathSource{{File: rc, Line: 4, Hearth: true}}},
		{Dir: secondDir, Sources: []inventory.PathSource{{File: rc, Line: 5, Hearth: true}}},
	}

	// Non-Hearth entry must return ErrNoFix.
	if _, err := PathRemovePlan(entries, userDir, env, nil); !errors.Is(err, ErrNoFix) {
		t.Fatalf("userDir err = %v, want ErrNoFix", err)
	}

	// Removing firstDir keeps secondDir and userDir intact.
	p, err := PathRemovePlan(entries, firstDir, env, func(context.Context) (sysenv.Env, error) {
		return sysenv.Env{"PATH=" + userDir + ":" + secondDir}, nil
	})
	if err != nil {
		t.Fatalf("PathRemovePlan firstDir: %v", err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Edit == nil {
		t.Fatalf("steps = %+v", p.Steps)
	}
	after := p.Steps[0].Edit.After
	if strings.Contains(after, firstDir) {
		t.Errorf("firstDir still present:\n%s", after)
	}
	if !strings.Contains(after, secondDir) || !strings.Contains(after, `$HOME/user-bin`) || !strings.Contains(after, BlockStart) {
		t.Errorf("unexpected after:\n%s", after)
	}
	if _, err := p.Verify(t.Context()); err != nil {
		t.Errorf("verify failed: %v", err)
	}

	// Removing the last Hearth entry removes the Hearth block markers too.
	if err := os.WriteFile(rc, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	p2, err := PathRemovePlan(entries, secondDir, env, nil)
	if err != nil {
		t.Fatalf("PathRemovePlan secondDir: %v", err)
	}
	after2 := p2.Steps[0].Edit.After
	if strings.Contains(after2, BlockStart) || strings.Contains(after2, BlockEnd) || strings.Contains(after2, secondDir) {
		t.Errorf("expected empty Hearth block removed:\n%s", after2)
	}
	if !strings.Contains(after2, `export PATH="$HOME/user-bin:$PATH"`) {
		t.Errorf("user line should remain:\n%s", after2)
	}
}
