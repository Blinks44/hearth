package sysenv

import (
	"context"
	"os/user"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
)

// userShell reads the user's shell from Directory Services, or returns "".
func userShell(ctx context.Context, r run.Runner) string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{"/usr/bin/dscl", ".", "-read", "/Users/" + u.Username, "UserShell"}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	// UserShell: /bin/zsh
	_, sh, _ := strings.Cut(string(res.Stdout), "UserShell:")
	return strings.TrimSpace(sh)
}
