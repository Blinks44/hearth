package ops

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestProcessPlan(t *testing.T) {
	node := inventory.DatabaseListener{Address: "*", Port: "5173", Process: "node", PID: "4312", Path: filepath.FromSlash("/opt/node/bin/node"), Own: true}
	listening := true
	detect := func(context.Context) (inventory.ToolState, error) {
		d := &inventory.DatabaseState{}
		if listening {
			d.Listeners = []inventory.DatabaseListener{node}
		}
		return inventory.ToolState{Databases: d}, nil
	}
	p, ok := ProcessPlan(node, detect)
	if !ok || len(p.Steps) != 1 {
		t.Fatalf("plan %+v, %v", p, ok)
	}
	if !slices.Contains(p.Steps[0].Argv, "4312") {
		t.Errorf("step %q does not name the PID", p.Steps[0].Argv)
	}
	listening = false
	if _, err := p.Verify(t.Context()); err != nil {
		t.Errorf("Verify: %v", err)
	}

	system := systemDirs()[0] + "ControlCenter"
	for _, l := range []inventory.DatabaseListener{
		{Port: "5432", Process: "postgres", PID: "812", Path: "/opt/pg/bin/postgres"}, // another user's
		{Port: "5000", Process: "ControlCenter", PID: "90", Path: system, Own: true},  // the system's
		{Port: "6379"}, // not known whose
		{Port: "3000", Process: "node", PID: "1; rm -rf ~", Path: "/opt/node/bin/node", Own: true}, // not a PID
	} {
		if _, ok := ProcessPlan(l, nil); ok {
			t.Errorf("plan for %+v", l)
		}
	}
}
