package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestServicePlanWindows(t *testing.T) {
	// A server found as an installer's copy keeps its source beside the
	// service.
	db := inventory.Database{Kind: "SQL Server", Source: "installer", Service: "MSSQL$SQLEXPRESS"}
	p, ok := ServicePlan(ServiceStop, db, nil)
	if !ok || len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Shown(), []string{"net.exe", "stop", "MSSQL$SQLEXPRESS", "/y"}) {
		t.Errorf("plan = %+v, %v", p.Steps, ok)
	}
}
