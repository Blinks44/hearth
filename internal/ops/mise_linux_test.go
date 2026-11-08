package ops

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// mise comes from the system's packages where they have it, else from its
// own installer into the home folder.
func TestInstallMiseLinux(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	env := sysenv.Env{"HOME=" + root, "PATH=" + bin}
	s, _ := installMise(env)
	if len(s.Steps) != 1 || s.Steps[0].Admin || s.Steps[0].Argv[2] != miseRunScript || s.Mise != filepath.Join(root, ".local/bin/mise") {
		t.Fatalf("installer: %+v", s)
	}
	write(t, filepath.Join(bin, "pacman"))
	s, _ = installMise(env)
	if len(s.Steps) != 1 || !s.Steps[0].Admin || !slices.Contains(s.Steps[0].Argv, "mise") || s.Steps[0].Argv[0] != "pacman" || s.Mise != "/usr/bin/mise" {
		t.Errorf("pacman: %+v", s)
	}
}
