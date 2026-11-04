package sysenv

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
)

// userShell reads the user's shell from the passwd database, or returns "".
func userShell(ctx context.Context, r run.Runner) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{"getent", "passwd", strconv.Itoa(os.Getuid())}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	// name:x:1000:1000:gecos:/home/name:/bin/bash
	line := strings.TrimSpace(string(res.Stdout))
	return line[strings.LastIndexByte(line, ':')+1:]
}
