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
