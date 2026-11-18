package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerLocationsWindows(t *testing.T) {
	home := root(t)
	pf := filepath.Join(home, "Program Files")
	local := filepath.Join(home, "AppData", "Local")
	desktop := filepath.Join(pf, "Docker", "Docker", "Docker Desktop.exe")
	rancher := filepath.Join(local, "Programs", "Rancher Desktop", "Rancher Desktop.exe")
	kiri := filepath.Join(local, "Kiri", "kiri-win.exe")
	writeFile(t, desktop, "")
	writeFile(t, rancher, "")
	writeFile(t, kiri, "")
	engines, paths := dockerLocations(sysenv.Env{"ProgramFiles=" + pf, "LOCALAPPDATA=" + local, "PATH="}, home)
	if engines[0].Path != desktop || engines[3].Path != rancher || engines[4].Name != "Kiri" || engines[4].Path != kiri {
		t.Fatalf("%+v", engines)
	}
	if paths[len(paths)-1] != filepath.Join(local, "Kiri", "bin", "docker.exe") {
		t.Fatalf("no Kiri CLI: %v", paths)
	}
	if paths[0] != filepath.Join(filepath.Dir(desktop), "resources", "bin", "docker.exe") {
		t.Fatalf("%v", paths)
	}
}
