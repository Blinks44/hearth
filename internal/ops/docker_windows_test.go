package ops

import (
	"slices"
	"testing"
)

func testEngine() Engine {
	return Engine{Name: "Docker Desktop", Path: `C:\Program Files\Docker\Docker\Docker Desktop.exe`}
}

func TestEngineStartWindows(t *testing.T) {
	desktop := testEngine()
	steps := engineStart(desktop)
	if len(steps) != 1 || steps[0].Argv[0] != "powershell.exe" || !slices.Equal(steps[0].Shown(), []string{desktop.Path}) ||
		!slices.Contains(steps[0].Env, "HEARTH_PROGRAM="+desktop.Path) {
		t.Errorf("Docker Desktop: %+v", steps)
	}
	kiri := Engine{"Kiri", `C:\Users\ann\AppData\Local\Kiri\kiri-win.exe`}
	if steps := engineStart(kiri); len(steps) != 1 || !slices.Equal(steps[0].Shown(), []string{kiri.Path}) {
		t.Errorf("Kiri: %+v", steps)
	}
	podman := Engine{"Podman", `C:\Program Files\RedHat\Podman\podman.exe`}
	if steps := engineStart(podman); len(steps) != 1 || !slices.Equal(steps[0].Argv, []string{podman.Path, "machine", "start"}) {
		t.Errorf("Podman: %+v", steps)
	}
}
