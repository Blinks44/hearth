package ops

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestServicePlanMacOSOnlyBrew(t *testing.T) {
	if _, ok := ServicePlan(ServiceStart, inventory.Database{Kind: "PostgreSQL", Source: "installer", Service: "postgresql-17"}, nil); ok {
		t.Error("plan for a service Homebrew does not keep")
	}
}
