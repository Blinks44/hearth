package docker

import (
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadColimaProfilesAndStatus(t *testing.T) {
	t.Parallel()
	var r run.Fake
	r.Stdout(`{"name":"default","status":"Running","cpus":4,"memory":8589934592,"disk":64424509440,"runtime":"docker"}
{"name":"build","status":"Stopped","cpus":2,"memory":2147483648,"disk":107374182400,"runtime":"containerd"}`, "colima", "list", "--json")
	r.Stdout(`{"display_name":"colima","driver":"VZ","mount_type":"virtiofs","docker_socket":"unix:///tmp/default/docker.sock","runtime":"docker","ip_address":"192.0.2.5"}`,
		"colima", "status", "--profile", "default", "--json")
	profiles, err := ReadColimaProfiles(t.Context(), &r, nil, "colima")
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles = %+v, %v", profiles, err)
	}
	p := profiles[0]
	if err := ReadColimaStatus(t.Context(), &r, nil, "colima", &p); err != nil {
		t.Fatal(err)
	}
	if p.CPU != 4 || p.Memory != 8*ColimaGiB || p.Disk != 60*ColimaGiB || p.Status != "running" ||
		p.Driver != "VZ" || p.Mount != "virtiofs" || p.Socket != "unix:///tmp/default/docker.sock" {
		t.Fatalf("running profile = %+v", p)
	}
	if p := profiles[1]; p.Name != "build" || p.Status != "stopped" || p.Runtime != "containerd" || p.Socket != "" {
		t.Fatalf("stopped profile = %+v", p)
	}
}

func TestColimaAutostart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, output, want string }{
		{"registered", `[{"name":"colima","status":"error","registered":true}]`, "on"},
		{"running without registration", `[{"name":"colima","status":"started","registered":false}]`, "off"},
		{"old brew", `[{"name":"colima","file":"/home/test/.config/systemd/user/homebrew.colima.service"}]`, "on"},
		{"missing", `[]`, ""},
		{"invalid", `null`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var r run.Fake
			r.Stdout(tc.output, "brew", "services", "list", "--json")
			if got := ColimaAutostart(t.Context(), &r, nil, "brew"); got != tc.want {
				t.Errorf("autostart = %q, want %q", got, tc.want)
			}
		})
	}
}
