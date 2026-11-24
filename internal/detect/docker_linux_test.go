package detect

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
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

func TestDetectDockerServiceLinux(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	systemctl := fakeExe(t, filepath.Join(bin, "systemctl"))
	env := sysenv.Env{"PATH=" + bin}
	engines := []inventory.DockerEngine{{Name: "Docker Engine", Path: "/usr/bin/dockerd"}}

	tests := []struct {
		name   string
		stdout string
		want   *inventory.DockerServiceState
	}{
		{
			name: "active and enabled with socket",
			stdout: "Id=docker.service\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\n\n" +
				"Id=docker.socket\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\n",
			want: &inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "running", Autostart: "on"},
		},
		{
			name: "stopped and disabled with socket",
			stdout: "Id=docker.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n\n" +
				"Id=docker.socket\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n",
			want: &inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "stopped", Autostart: "off"},
		},
		{
			name: "socket still enabled keeps autostart on",
			stdout: "Id=docker.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n\n" +
				"Id=docker.socket\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\n",
			want: &inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "stopped", Autostart: "on"},
		},
		{
			name: "service without socket",
			stdout: "Id=docker.service\nLoadState=loaded\nActiveState=failed\nUnitFileState=disabled\n\n" +
				"Id=docker.socket\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n",
			want: &inventory.DockerServiceState{Unit: "docker.service", Status: "error", Autostart: "off"},
		},
		{
			name:   "unit not installed",
			stdout: "Id=docker.service\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n",
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &run.Fake{}
			r.Stdout(tc.stdout, systemctl, "show", "--property=Id,LoadState,ActiveState,UnitFileState", "docker.service", "docker.socket")
			got := detectDockerService(context.Background(), r, env, engines)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got = %+v, want %+v", got, tc.want)
			}
		})
	}
	if got := detectDockerService(context.Background(), &run.Fake{}, env, []inventory.DockerEngine{{Name: "Docker Engine"}}); got != nil {
		t.Fatalf("without dockerd: got %+v, want nil", got)
	}
}
