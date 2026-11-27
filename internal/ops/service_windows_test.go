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
	p, ok = ServicePlan(ServiceRestart, db, nil)
	if !ok || len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Shown(), []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Restart-Service -Name 'MSSQL$SQLEXPRESS' -Force"}) {
		t.Errorf("restart = %+v, %v", p.Steps, ok)
	}
}

func TestAutostartPlanWindows(t *testing.T) {
	db := inventory.Database{Kind: "PostgreSQL", Source: "installer", Service: "postgresql-x64-17"}
	p, ok := AutostartPlan(true, db, nil)
	if !ok || len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Shown(), []string{"sc.exe", "config", "postgresql-x64-17", "start=", "auto"}) {
		t.Errorf("plan = %+v, %v", p.Steps, ok)
	}
}
