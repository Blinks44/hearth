package sysenv

import (
	"context"

	"github.com/Blinks44/hearth/internal/run"
)

// AppsAgent is "": Hearth's launch agent is macOS's.
func AppsAgent(string) string { return "" }

// Apps is the environment apps started now get: Explorer reads the
// user's variables anew from the registry when they change, as a new
// terminal does. gui, Hearth's own, is what it was when Hearth started.
func Apps(ctx context.Context, r run.Runner, gui Env) Env {
	if env, err := Terminal(ctx, r); err == nil {
		return env
	}
	return gui
}
