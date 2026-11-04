//go:build !windows

package sysenv

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// resolvingFlag is set for the shell, so rc files can skip heavy work.
const resolvingFlag = "HEARTH_RESOLVING_ENVIRONMENT"

// shellTimeout leaves room for slow rc files, as VS Code does.
const shellTimeout = 10 * time.Second

// Terminal returns the environment a new terminal gets: the user's login
// shell runs interactively, so its rc files run, and prints its
// environment between random markers. Anything else rc files print is
// ignored.
func Terminal(ctx context.Context, r run.Runner) (Env, error) {
	return terminal(ctx, r, GUI(), rand.Text())
}

func terminal(ctx context.Context, r run.Runner, gui Env, marker string) (Env, error) {
	sh := loginShell(ctx, r, gui)
	script := "printf %s " + marker + "; /usr/bin/env -0; printf %s " + marker
	res, err := r.Run(ctx, run.Cmd{
		Args:    append(shellArgs(sh), script),
		Env:     gui.with(resolvingFlag, "1"),
		Timeout: shellTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("terminal environment from %s: %w", sh, err)
	}
	env, err := parseEnv(res.Stdout, marker)
	if err != nil {
		return nil, fmt.Errorf("terminal environment from %s (exit %d, stderr %q): %w",
			sh, res.ExitCode, res.Stderr, err)
	}
	return env.without(resolvingFlag), nil
}

// loginShell is $SHELL, else the user's shell from the system's user
// database, else /bin/sh.
func loginShell(ctx context.Context, r run.Runner, gui Env) string {
	if sh := gui.Get("SHELL"); sh != "" {
		return sh
	}
	if sh := userShell(ctx, r); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// shellArgs is the argv that runs a script in an interactive login shell.
func shellArgs(sh string) []string {
	switch filepath.Base(sh) {
	case "csh", "tcsh":
		return []string{sh, "-ic"}
	default: // sh, bash, zsh, fish, nu
		return []string{sh, "-i", "-l", "-c"}
	}
}

// parseEnv takes the NUL-separated entries of env -0 between the first two
// markers in out.
func parseEnv(out []byte, marker string) (Env, error) {
	_, rest, ok := bytes.Cut(out, []byte(marker))
	body, _, ok2 := bytes.Cut(rest, []byte(marker))
	if !ok || !ok2 {
		return nil, errors.New("no environment between markers")
	}
	var env Env
	for kv := range bytes.SplitSeq(body, []byte{0}) {
		if bytes.IndexByte(kv, '=') > 0 {
			env = append(env, string(kv))
		}
	}
	if len(env) == 0 {
		return nil, errors.New("empty environment between markers")
	}
	return env, nil
}
