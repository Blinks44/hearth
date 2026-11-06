package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Pacman says by package why it is there: %REASON% 1 is a dependency, and
// no reason at all is a package asked for.
func TestPacmanReasons(t *testing.T) {
	db := t.TempDir()
	for name, desc := range map[string]string{
		"git-2.47.1-1":  "%NAME%\ngit\n\n%VERSION%\n2.47.1-1\n",
		"pcre2-10.44-1": "%NAME%\npcre2\n\n%VERSION%\n10.44-1\n\n%REASON%\n1\n",
	} {
		if err := os.MkdirAll(filepath.Join(db, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(db, name, "desc"), []byte(desc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := inventory.PackageManager{ID: "pacman", Packages: []inventory.SystemPackage{{Name: "git"}, {Name: "pcre2"}}}
	if !reasons(&m, "", db) {
		t.Fatal("reasons not known")
	}
	if m.Packages[0].Dependency || !m.Packages[1].Dependency {
		t.Errorf("packages %+v", m.Packages)
	}
	m = inventory.PackageManager{ID: "pacman"}
	if reasons(&m, "", filepath.Join(db, "missing")) {
		t.Error("reasons known without the database")
	}
}

// APT's states file marks what it pulled in, upgradable or not.
func TestAPTAutomatic(t *testing.T) {
	states := filepath.Join(t.TempDir(), "extended_states")
	data := "Package: liberror-perl\nArchitecture: all\nAuto-Installed: 1\n\nPackage: git\nArchitecture: arm64\nAuto-Installed: 0\n"
	if err := os.WriteFile(states, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	m := inventory.PackageManager{ID: "apt", Packages: []inventory.SystemPackage{{Name: "git"}, {Name: "liberror-perl", Available: "0.18"}}}
	if !reasons(&m, states, "") || m.Packages[0].Dependency || !m.Packages[1].Dependency {
		t.Errorf("packages %+v", m.Packages)
	}
}
