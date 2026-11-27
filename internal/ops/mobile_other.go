//go:build !darwin

package ops

import (
	"context"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// RemoveSimRuntimePlan is macOS-only.
func RemoveSimRuntimePlan(_ context.Context, _ run.Runner, _ inventory.SimRuntime, _ sysenv.Env, _ Detect) (Plan, error) {
	return Plan{}, ErrNoFix
}

// SwitchXcodePlan is macOS-only.
func SwitchXcodePlan(_ inventory.XcodeApp, _ Detect) (Plan, bool) {
	return Plan{}, false
}
