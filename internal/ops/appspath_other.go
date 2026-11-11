//go:build !darwin

package ops

import "github.com/Blinks44/hearth/internal/sysenv"

// AppsPathSettable reports that Hearth can give apps started from the
// Dock the terminal's PATH: only macOS has one apps get apart.
const AppsPathSettable = false

// AppsPathPlan is macOS's.
func AppsPathPlan(sysenv.Env) (Plan, error) { return Plan{}, ErrNoFix }

// AppsPathStopPlan is macOS's.
func AppsPathStopPlan(string) Plan { return Plan{} }
