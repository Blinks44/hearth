package issues

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

func TestDatabaseConflicts(t *testing.T) {
	for _, tc := range []struct {
		name               string
		listener, own, env bool
		want               int
	}{{"occupied", true, false, false, 1}, {"free", false, false, false, 0}, {"own container", true, true, false, 0}, {"connection", true, false, true, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			d := &inventory.DatabaseState{}
			if tc.listener {
				d.Listeners = []inventory.DatabaseListener{{Port: "5432"}}
			}
			s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: d}, {Tool: "docker", Docker: &inventory.DockerState{}}}}
			if tc.own {
				s.Tools[1].Docker.Databases = []inventory.Database{{Source: "docker", Status: "running", Project: "/dev/app", Service: "db", Ports: []string{"5432"}}}
			}
			file := "compose.yaml"
			if tc.env {
				file = ".env"
			}
			ps := []projects.Project{{Dir: "/dev/app", Databases: []projects.DatabaseReference{{Service: "db", File: file, Ports: []string{"5432"}}}}}
			if got := DatabaseConflicts(s, ps); len(got) != tc.want {
				t.Fatal(got)
			}
		})
	}
}
