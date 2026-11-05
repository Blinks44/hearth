package sysenv

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/internal/run"
)

// Terminal returns the environment a new terminal gets: the user's
// variables read anew from the registry by CreateEnvironmentBlock, as
// Explorer builds them. Hearth's own are what they were when Explorer
// started it, so a JDK installed since sets JAVA_HOME and Path only for
// new terminals.
func Terminal(context.Context, run.Runner) (Env, error) {
	env, err := windows.GetCurrentProcessToken().Environ(false)
	if err != nil {
		return nil, fmt.Errorf("terminal environment: %w", err)
	}
	return Env(env), nil
}
