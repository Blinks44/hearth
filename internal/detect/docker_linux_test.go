package detect

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerLocationsLinux(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	exe := fakeExe(t, filepath.Join(bin, "dockerd"))
	podman := fakeExe(t, filepath.Join(bin, "podman"))
	kiri := fakeExe(t, filepath.Join(bin, "kiri"))
	engines, paths := dockerLocations(sysenv.Env{"PATH=" + bin}, home)
	if engines[0].Path != exe || engines[1].Path != podman || engines[4].Name != "Kiri" || engines[4].Path != kiri {
		t.Fatalf("%+v", engines)
	}
	if paths[0] != "/usr/libexec/kiri/docker" {
		t.Fatalf("no Kiri CLI: %v", paths)
	}
}

func TestDockerLocationsLinuxKeepsInstalledColima(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	fakeExe(t, filepath.Join(bin, "brew"))
	env := sysenv.Env{"PATH=" + bin}
	engines, _ := dockerLocations(env, home)
	if engines[0].Name != "Docker Engine" || slices.ContainsFunc(engines, func(e inventory.DockerEngine) bool {
		return e.Name == "Colima"
	}) {
		t.Fatalf("installation choices with Homebrew: %+v", engines)
	}
	colima := fakeExe(t, filepath.Join(bin, "colima"))
	engines, _ = dockerLocations(env, home)
	if engines[0].Name != "Docker Engine" || !slices.Contains(engines, inventory.DockerEngine{Name: "Colima", Path: colima}) {
		t.Fatalf("installed engines: %+v", engines)
	}
}

// A user in the docker group as /etc/group has it, whom the socket still
// refuses, needs only to sign in again.
func TestDockerJoined(t *testing.T) {
	dir := t.TempDir()
	orig := groupFile
	groupFile = filepath.Join(dir, "group")
	t.Cleanup(func() { groupFile = orig })
	writeFile(t, groupFile, "root:x:0:\ndocker:x:124:bob,ann\nannx:x:1000:\n")
	for user, want := range map[string]bool{"ann": true, "bob": true, "an": false, "": false} {
		if got := dockerJoined(user); got != want {
			t.Errorf("dockerJoined(%q) = %v", user, got)
		}
	}
}
