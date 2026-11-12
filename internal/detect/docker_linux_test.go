package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerLocationsLinux(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	exe := fakeExe(t, filepath.Join(bin, "dockerd"))
	podman := fakeExe(t, filepath.Join(bin, "podman"))
	engines, _ := dockerLocations(sysenv.Env{"PATH=" + bin}, home)
	if engines[0].Path != exe || engines[1].Path != podman {
		t.Fatalf("%+v", engines)
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
