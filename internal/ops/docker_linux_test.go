package ops

import (
	"context"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
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

func TestDockerServicePlanLinux(t *testing.T) {
	withSocket := inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "running", Autostart: "on"}
	noSocket := inventory.DockerServiceState{Unit: "docker.service", Status: "stopped", Autostart: "off"}
	detectState := func(s inventory.DockerServiceState) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{Docker: &inventory.DockerState{Service: &s}}, nil
		}
	}

	tests := []struct {
		name   string
		action DockerServiceAction
		svc    inventory.DockerServiceState
		want   []string
		after  inventory.DockerServiceState
	}{
		{
			name:   "start service",
			action: DockerServiceStart,
			svc:    noSocket,
			want:   []string{"systemctl", "start", "docker.service"},
			after:  inventory.DockerServiceState{Unit: "docker.service", Status: "running", Autostart: "off"},
		},
		{
			name:   "stop service and socket",
			action: DockerServiceStop,
			svc:    withSocket,
			want:   []string{"systemctl", "stop", "docker.socket", "docker.service"},
			after:  inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "stopped", Autostart: "on"},
		},
		{
			name:   "stop service without socket",
			action: DockerServiceStop,
			svc:    noSocket,
			want:   []string{"systemctl", "stop", "docker.service"},
			after:  inventory.DockerServiceState{Unit: "docker.service", Status: "stopped", Autostart: "off"},
		},
		{
			name:   "enable autostart without starting",
			action: DockerServiceEnable,
			svc:    withSocket,
			want:   []string{"systemctl", "enable", "docker.service", "docker.socket"},
			after:  inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "stopped", Autostart: "on"},
		},
		{
			name:   "disable autostart without stopping",
			action: DockerServiceDisable,
			svc:    withSocket,
			want:   []string{"systemctl", "disable", "docker.service", "docker.socket"},
			after:  inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "docker.socket", Status: "running", Autostart: "off"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := DockerServicePlan(tc.action, tc.svc, detectState(tc.after))
			if !ok {
				t.Fatal("expected plan")
			}
			if len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, tc.want) {
				t.Fatalf("steps = %+v, want admin %q", p.Steps, tc.want)
			}
			if slices.Contains(p.Steps[0].Argv, "--now") {
				t.Fatalf("autostart plan must not pass --now: %q", p.Steps[0].Argv)
			}
			if _, err := p.Verify(context.Background()); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
	if _, ok := DockerServicePlan(DockerServiceStart, inventory.DockerServiceState{Unit: "../bad"}, nil); ok {
		t.Error("accepted invalid unit name")
	}
	if _, ok := DockerServicePlan(DockerServiceStop, inventory.DockerServiceState{Unit: "docker.service", SocketUnit: "-f"}, nil); ok {
		t.Error("accepted invalid socket unit name")
	}
}
