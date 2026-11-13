package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestServicePlanSystemd(t *testing.T) {
	db := inventory.Database{Kind: "PostgreSQL", Source: "system", Service: "postgresql@16-main.service"}
	p, ok := ServicePlan(ServiceStop, db, nil)
	if !ok || len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, []string{"systemctl", "stop", "postgresql@16-main.service"}) {
		t.Errorf("plan = %+v, %v", p.Steps, ok)
	}
}

// A Debian cluster starts with the system through postgresql.service.
func TestAutostartPlanSystemd(t *testing.T) {
	db := inventory.Database{Kind: "PostgreSQL", Source: "system", Service: "postgresql@16-main.service"}
	p, ok := AutostartPlan(false, db, nil)
	if !ok || len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, []string{"systemctl", "disable", "postgresql.service"}) {
		t.Errorf("plan = %+v, %v", p.Steps, ok)
	}
}
