package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// SetEnvVarPlan returns a plan that persists name=value in the user's shell
// startup block (Unix) or user environment variables (Windows). Only allowlisted
// SDK home variables (ANDROID_HOME, JAVA_HOME) are accepted.
func SetEnvVarPlan(name, value string, env sysenv.Env, term Terminal) (Plan, error) {
	switch name {
	case "ANDROID_HOME", "JAVA_HOME":
	default:
		return Plan{}, ErrNoFix
	}
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) || hasControlChars(value) {
		return Plan{}, ErrNoFix
	}
	value = filepath.Clean(value)
	steps, err := setEnvVarSteps(name, value, env)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		Title:           locales.EnvVarSetTitle(name),
		Changes:         []string{locales.EnvVarSetChange(name, value)},
		Steps:           steps,
		RestartTerminal: true,
	}
	if term != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			gotEnv, err := term(ctx)
			if err != nil {
				return "", err
			}
			got := gotEnv.Get(name)
			if got == "" || !sysenv.SamePath(got, value) {
				return "", errors.New(name + " is not set to " + value)
			}
			return locales.EnvVarSetChange(name, value), nil
		}
	}
	return p, nil
}

func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
