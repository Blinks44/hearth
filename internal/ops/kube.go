package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// KubeContextPlan changes only kubectl's local current-context using its
// native config command. It never contacts a cluster or runs auth plugins.
func KubeContextPlan(cli, from, to string, env sysenv.Env, r run.Runner) (Plan, error) {
	if cli == "" || to == "" || strings.ContainsAny(to, "\x00\r\n") {
		return Plan{}, errors.New(locales.KubeContextInvalid())
	}
	p := Plan{
		Title: locales.KubeContextTitle(to), Why: locales.KubeContextWhy(),
		Env:     append([]string{}, env...),
		Steps:   []Step{{Argv: []string{cli, "config", "use-context", "--", to}, Timeout: time.Minute}},
		Changes: []string{"kubectl context: " + from + " → " + to},
	}
	p.Verify = func(ctx context.Context) (string, error) {
		res, err := r.Run(ctx, run.Cmd{Args: []string{cli, "config", "current-context"}, Env: env})
		if err != nil {
			return "", fmt.Errorf("check kubectl context: %w", err)
		}
		if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != to {
			return "", errors.New(locales.KubeContextMismatch(to))
		}
		return locales.KubeContextSelected(to), nil
	}
	return p, nil
}
