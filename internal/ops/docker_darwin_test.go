package ops

import (
	"slices"
	"testing"
)

func testEngine() Engine { return Engine{Name: "OrbStack", Path: "/Applications/OrbStack.app"} }

func TestEngineStartMacOS(t *testing.T) {
	tests := []struct {
		e    Engine
		want []string
	}{
		{Engine{"OrbStack", "/Applications/OrbStack.app"}, []string{"/usr/bin/open", "-a", "/Applications/OrbStack.app"}},
		{Engine{"Docker Desktop", "/Applications/Docker.app"}, []string{"/usr/bin/open", "-a", "/Applications/Docker.app"}},
		{Engine{"Colima", "/opt/homebrew/bin/colima"}, []string{"/opt/homebrew/bin/colima", "start"}},
		{Engine{"Podman", "/opt/homebrew/bin/podman"}, []string{"/opt/homebrew/bin/podman", "machine", "start"}},
		{Engine{"Podman Desktop", "/Applications/Podman Desktop.app"}, nil},
	}
	for _, tt := range tests {
		steps := engineStart(tt.e)
		var got []string
		if len(steps) == 1 {
			got = steps[0].Argv
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.e.Name, got, tt.want)
		}
	}
}
