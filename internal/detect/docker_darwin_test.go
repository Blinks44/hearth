package detect

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerLocationsDarwin(t *testing.T) {
	home := root(t)
	dir := filepath.Join(home, "Applications", "Docker.app")
	exe := filepath.Join(dir, "Contents", "Resources", "bin", "docker")
	fakeExe(t, exe)
	orb := filepath.Join(home, "Applications", "OrbStack.app", "Contents", "Info.plist")
	writeFile(t, orb, "")
	kiri := filepath.Join(home, "Applications", "Kiri.app")
	writeFile(t, filepath.Join(kiri, "Contents", "Info.plist"), "")
	engines, paths := dockerLocations(sysenv.Env{"PATH="}, home)
	if engines[0].Name != "Colima" || engines[1].Path != filepath.Dir(filepath.Dir(orb)) || engines[2].Path != dir || engines[6] != (inventory.DockerEngine{Name: "Kiri", Path: kiri}) {
		t.Fatalf("%+v", engines)
	}
	if !slices.Contains(paths, filepath.Join(home, ".kiri", "bin", "docker")) {
		t.Fatalf("no Kiri CLI: %v", paths)
	}
	if paths[len(paths)-1] != exe {
		t.Fatalf("%v", paths)
	}
}
