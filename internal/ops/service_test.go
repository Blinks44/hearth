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
