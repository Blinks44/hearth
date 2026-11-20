package run

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OpenWith opens path in an editor: through its app on macOS when app is
// one, else through its command. It waits only for the launcher, which
// hands the file to the editor and returns.
func OpenWith(ctx context.Context, r Runner, app, command, path string, env []string) error {
	args := editorLauncher(app, command, path)
	if args == nil {
		return fmt.Errorf("open %s: no app or command", path)
	}
	res, err := r.Run(ctx, Cmd{Args: args, Env: env, Timeout: 20 * time.Second})
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: exit %d: %s", args[0], res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// App is an app Hearth opens things in: its name and its app bundle or
// command.
type App struct {
	Name, Path string
}
