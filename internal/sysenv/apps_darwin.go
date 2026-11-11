package sysenv

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// AppsAgentLabel names Hearth's launch agent, which gives launchd the
// terminal's PATH for apps as the user signs in.
const AppsAgentLabel = "dev.hearth.path"

// AppsAgent is the file of Hearth's launch agent for the user of home.
func AppsAgent(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", AppsAgentLabel+".plist")
}

// AppsPath is the PATH launchctl setenv gave launchd, which apps started
// from the Dock, Finder or Spotlight get in place of the system's; ""
// when there is none.
func AppsPath(ctx context.Context, r run.Runner) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{"/bin/launchctl", "getenv", "PATH"}, Timeout: 5 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(res.Stdout))
}

// Apps is the environment an app started from the Dock now gets: gui,
// Hearth's own, with the PATH launchd has since, which it gets instead.
func Apps(ctx context.Context, r run.Runner, gui Env) Env {
	if path := AppsPath(ctx, r); path != "" {
		return gui.with("PATH", path)
	}
	return gui
}
