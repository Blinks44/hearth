//go:build !windows

package ops

import (
	"context"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestDockerDefaultSocketPlan(t *testing.T) {
	p, ok := DockerDefaultSocketPlan("colima", "unix:///Users/dev/.colima/default/docker.sock", func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Docker: &inventory.DockerState{Selected: "colima"}}, nil
	})
	if !ok {
		t.Fatal("expected valid Docker default socket plan")
	}
	if len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, []string{"ln", "-sfn", "/Users/dev/.colima/default/docker.sock", "/var/run/docker.sock"}) {
		t.Fatalf("unexpected steps: %+v", p.Steps)
	}
	if p.Verify == nil {
		t.Fatal("expected Verify callback")
	}
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Rejects non-unix, default path itself, or invalid context names.
	for _, tc := range []struct{ ctx, endpoint string }{
		{"colima", "tcp://127.0.0.1:2375"},
		{"colima", "unix:///var/run/docker.sock"},
		{"colima", "unix://relative.sock"},
		{"bad name", "unix:///tmp/docker.sock"},
	} {
		if _, ok := DockerDefaultSocketPlan(tc.ctx, tc.endpoint, nil); ok {
			t.Errorf("expected rejected plan for (%q, %q)", tc.ctx, tc.endpoint)
		}
	}
}
