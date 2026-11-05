package issues

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestDockerUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name, selected, status string
		want                   bool
	}{
		{"selected unavailable", "local", "unavailable", true},
		{"available", "local", "available", false},
		{"unchecked", "local", "", false},
		{"other context", "remote", "unavailable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "docker", Docker: &inventory.DockerState{Selected: tc.selected, Contexts: []inventory.DockerContext{{Name: "local", Status: tc.status}}}}}}
			found := Find(s, nil, nil, time.Now())
			if (len(found) == 1) != tc.want {
				t.Fatalf("%+v", found)
			}
			if tc.want && (found[0].Kind != DockerUnavailable || found[0].Tool != "docker" || found[0].Severity != Warning) {
				t.Fatalf("%+v", found)
			}
		})
	}
}
