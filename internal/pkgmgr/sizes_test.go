package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// dpkg records sizes in KiB, Pacman in bytes; a package with none is
// left to count.
func TestRecordedSizes(t *testing.T) {
	dir := t.TempDir()
	status := filepath.Join(dir, "status")
	data := "Package: git\nStatus: install ok installed\nArchitecture: arm64\nInstalled-Size: 22000\n\n" +
		"Package: libc6\nArchitecture: arm64\nInstalled-Size: 100\n\nPackage: libc6\nArchitecture: armhf\nInstalled-Size: 90\n"
	if err := os.WriteFile(status, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	m := inventory.PackageManager{ID: "apt", Packages: []inventory.SystemPackage{{Name: "git"}, {Name: "libc6"}, {Name: "vim"}}}
	recordedSizes(&m, status, "")
	if m.Packages[0].Size != 22000*1024 || m.Packages[1].Size != 100*1024 || m.Packages[2].Size != 0 {
		t.Errorf("apt %+v", m.Packages)
	}

	db := filepath.Join(dir, "local")
	if err := os.MkdirAll(filepath.Join(db, "git-2.47.1-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	desc := "%NAME%\ngit\n\n%VERSION%\n2.47.1-1\n\n%SIZE%\n27365376\n\n%REASON%\n1\n"
	if err := os.WriteFile(filepath.Join(db, "git-2.47.1-1", "desc"), []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}
	m = inventory.PackageManager{ID: "pacman", Packages: []inventory.SystemPackage{{Name: "git"}}}
	recordedSizes(&m, "", db)
	if m.Packages[0].Size != 27365376 {
		t.Errorf("pacman %+v", m.Packages)
	}
}
