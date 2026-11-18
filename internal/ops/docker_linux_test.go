package ops

import (
	"slices"
	"testing"
)

func testEngine() Engine { return Engine{Name: "Docker Engine", Path: "/usr/bin/dockerd"} }

func TestEngineStartLinux(t *testing.T) {
	tests := []struct {
		e     Engine
		want  []string
		admin bool
	}{
		{Engine{"Docker Engine", "/usr/bin/dockerd"}, []string{"systemctl", "start", "docker"}, true},
		{Engine{"Docker Desktop", "/opt/docker-desktop"}, []string{"systemctl", "--user", "start", "docker-desktop"}, false},
		{Engine{"Podman", "/usr/bin/podman"}, []string{"systemctl", "--user", "start", "podman.socket"}, false},
		{Engine{"Kiri", "/usr/bin/kiri"}, []string{"systemctl", "start", "kiri"}, true},
		{Engine{"Rancher Desktop", "/usr/bin/rancher-desktop"}, nil, false},
	}
	for _, tt := range tests {
		steps := engineStart(tt.e)
		var got []string
		admin := false
		if len(steps) == 1 {
			got, admin = steps[0].Argv, steps[0].Admin
		}
		if !slices.Equal(got, tt.want) || admin != tt.admin {
			t.Errorf("%s: %q admin %v, want %q admin %v", tt.e.Name, got, admin, tt.want, tt.admin)
		}
	}
}
