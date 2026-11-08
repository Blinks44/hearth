package ops

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// mise comes through Homebrew; without Homebrew, its signed package comes
// first, installed as the administrator.
func TestInstallMiseMac(t *testing.T) {
	root := t.TempDir()
	defer func(p []string) { brewPrefixes = p }(brewPrefixes)
	brewPrefixes = []string{filepath.Join(root, "arm"), filepath.Join(root, "intel")}
	env := sysenv.Env{"HOME=" + root, "PATH=" + filepath.Join(root, "bin")}

	s, err := installMise(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Steps) != 4 || !s.Steps[2].Admin || s.Steps[2].Argv[0] != "/usr/sbin/installer" ||
		!slices.Contains(s.Steps[1].Argv, homebrewSigned) || s.Steps[3].Argv[1] != "install" || len(s.Notes) != 2 {
		t.Fatalf("without Homebrew: %+v", s)
	}
	if s.Mise != filepath.Join(filepath.Dir(s.Steps[3].Argv[0]), "mise") {
		t.Errorf("mise at %s, brew at %s", s.Mise, s.Steps[3].Argv[0])
	}

	brew := filepath.Join(root, "intel", "bin", "brew")
	write(t, brew)
	s, _ = installMise(env)
	if len(s.Steps) != 1 || !slices.Equal(s.Steps[0].Argv, []string{brew, "install", "mise"}) || s.Mise != filepath.Join(root, "intel", "bin", "mise") || len(s.Notes) != 0 {
		t.Errorf("with Homebrew: %+v", s)
	}
}
