package ops

import (
	"context"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestServicePlanBrew(t *testing.T) {
	pg := inventory.Database{Kind: "PostgreSQL", Name: "PostgreSQL", Source: "brew", Service: "postgresql@17", Status: "running"}
	found := func(status string) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			after := pg
			after.Status = status
			return inventory.ToolState{Databases: &inventory.DatabaseState{Servers: []inventory.Database{after}}}, nil
		}
	}
	p, ok := ServicePlan(ServiceStop, pg, found("stopped"))
	if !ok {
		t.Fatal("no plan")
	}
	if want := []string{"brew", "services", "stop", "postgresql@17"}; len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, want) {
		t.Errorf("steps = %+v, want %q", p.Steps, want)
	}
	if len(p.Notes) != 1 {
		t.Errorf("notes = %+v, want the one on login", p.Notes)
	}
	if _, err := p.Verify(context.Background()); err != nil {
		t.Errorf("Verify: %v", err)
	}
	p, _ = ServicePlan(ServiceStart, pg, found("stopped"))
	if !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "start", "postgresql@17"}) {
		t.Errorf("start = %q", p.Steps[0].Argv)
	}
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("Verify passed for a service still stopped")
	}
	p, ok = ServicePlan(ServiceRestart, pg, found("running"))
	if !ok || len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "restart", "postgresql@17"}) {
		t.Errorf("restart = %+v, %v", p.Steps, ok)
	}
	if _, err := p.Verify(context.Background()); err != nil {
		t.Errorf("restart Verify: %v", err)
	}
}

func TestServicePlanRefuses(t *testing.T) {
	for _, db := range []inventory.Database{
		{Kind: "PostgreSQL", Source: "docker", Service: "db", Name: "shop-db-1"},
		{Kind: "PostgreSQL", Source: "brew"},
		{Kind: "PostgreSQL", Source: "brew", Service: "postgresql; rm"},
	} {
		if _, ok := ServicePlan(ServiceStart, db, nil); ok {
			t.Errorf("plan for %+v", db)
		}
	}
}

// Homebrew's autostart is its login item: turned off, a running service
// runs on without one.
func TestAutostartPlanBrew(t *testing.T) {
	pg := inventory.Database{Kind: "PostgreSQL", Source: "brew", Service: "postgresql@17", Status: "running", Autostart: "on"}
	found := func(autostart string) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			after := pg
			after.Autostart = autostart
			return inventory.ToolState{Databases: &inventory.DatabaseState{Servers: []inventory.Database{after}}}, nil
		}
	}
	p, ok := AutostartPlan(false, pg, found("off"))
	if !ok || len(p.Steps) != 2 || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "stop", "postgresql@17"}) ||
		!slices.Equal(p.Steps[1].Argv, []string{"brew", "services", "run", "postgresql@17"}) {
		t.Fatalf("off: %+v, %v", p.Steps, ok)
	}
	if _, err := p.Verify(context.Background()); err != nil {
		t.Errorf("Verify: %v", err)
	}
	pg.Status, pg.Autostart = "stopped", "off"
	p, _ = AutostartPlan(true, pg, found("off"))
	if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "start", "postgresql@17"}) || len(p.Changes) != 2 {
		t.Errorf("on: %+v, changes %q", p.Steps, p.Changes)
	}
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("Verify passed with autostart still off")
	}
	// One running without a login item starts again with one.
	pg.Status = "running"
	if p, _ := AutostartPlan(true, pg, nil); len(p.Steps) != 2 || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "stop", "postgresql@17"}) ||
		!slices.Equal(p.Steps[1].Argv, []string{"brew", "services", "start", "postgresql@17"}) {
		t.Errorf("on while running: %+v", p.Steps)
	}
	if _, ok := AutostartPlan(true, inventory.Database{Kind: "PostgreSQL", Source: "docker", Service: "db"}, nil); ok {
		t.Error("plan for a container")
	}
}
