package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestMiseManager(t *testing.T) {
	root := root(t)
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	mise := filepath.Join(bin, "mise")
	writeFile(t, mise, "")
	if err := os.Chmod(mise, 0o755); err != nil {
		t.Fatal(err)
	}
	shims := filepath.Join(home, ".local", "share", "mise", "shims")
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "LOCALAPPDATA=" + filepath.Join(home, ".local", "share"),
		"PATH=" + shims + string(os.PathListSeparator) + bin}
	d := sources.ManagerDirs(env)
	d.Mise = filepath.Join(home, ".local", "share", "mise")

	for _, tt := range []struct {
		name, rc  string
		activated bool
	}{
		{"activated", `eval "$(mise activate zsh)"` + "\n", true},
		{"commented out", `# eval "$(mise activate zsh)"` + "\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeFile(t, startupFile(home), tt.rc)
			r := &run.Fake{}
			r.Stdout("2025.1.6 macos-arm64 (2025-01-20)\n", mise, "--version")
			m := miseManager(t.Context(), r, env, sysenv.Env{"PATH=" + bin}, d)
			if m.Name != "mise" || m.Version != "2025.1.6" || m.Activated != tt.activated ||
				!m.Shims || m.ShimsInGUI || !m.SwitchOnCD || len(m.Files) != 2 {
				t.Errorf("got %+v", m)
			}
		})
	}
}

func TestMiseVersionFromPath(t *testing.T) {
	r := &run.Fake{}
	if got := miseVersion(t.Context(), r, nil, "/opt/homebrew/Cellar/mise/2025.10.2/bin/mise"); got != "2025.10.2" {
		t.Errorf("version %q", got)
	}
	if len(r.Calls()) != 0 {
		t.Error("ran mise for a version its path names")
	}
}
