package ops

import (
	"slices"
	"testing"
)

func testEngine() Engine { return Engine{Name: "OrbStack", Path: "/Applications/OrbStack.app"} }

func TestEngineStartMacOS(t *testing.T) {
	tests := []struct {
		e            Engine
		want         []string
		wantDisclaim bool
	}{
		{Engine{"OrbStack", "/Applications/OrbStack.app"}, []string{"/usr/bin/open", "-a", "/Applications/OrbStack.app"}, false},
		{Engine{"Docker Desktop", "/Applications/Docker.app"}, []string{"/usr/bin/open", "-a", "/Applications/Docker.app"}, false},
		{Engine{"Kiri", "/Applications/Kiri.app"}, []string{"/usr/bin/open", "-a", "/Applications/Kiri.app"}, false},
		{Engine{"Colima", "/opt/homebrew/bin/colima"}, []string{"/opt/homebrew/bin/colima", "start"}, true},
		{Engine{"Podman", "/opt/homebrew/bin/podman"}, []string{"/opt/homebrew/bin/podman", "machine", "start"}, true},
		{Engine{"Podman Desktop", "/Applications/Podman Desktop.app"}, nil, false},
	}
	for _, tt := range tests {
		steps := engineStart(tt.e)
		var got []string
		var gotDisclaim bool
		if len(steps) == 1 {
			got = steps[0].Argv
			gotDisclaim = steps[0].Disclaim
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.e.Name, got, tt.want)
		}
		if gotDisclaim != tt.wantDisclaim {
			t.Errorf("%s: Disclaim = %v, want %v", tt.e.Name, gotDisclaim, tt.wantDisclaim)
		}
	}
}
