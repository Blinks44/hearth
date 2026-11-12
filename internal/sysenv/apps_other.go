//go:build !darwin && !windows

package sysenv

import (
	"context"

	"github.com/Blinks44/hearth/internal/run"
)

// AppsAgent is "": Hearth's launch agent is macOS's.
func AppsAgent(string) string { return "" }

// Apps is gui: apps get the environment Hearth does.
func Apps(_ context.Context, _ run.Runner, gui Env) Env { return gui }
