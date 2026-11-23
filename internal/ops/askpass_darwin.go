package ops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// askTimeout is how long the password dialog waits for the user.
const askTimeout = 5 * time.Minute

// askScript asks for the password in macOS's own dialog, saying why, the
// first argument, and returns what the user typed.
const askScript = `on run argv
	return text returned of (display dialog (item 1 of argv as text) default answer "" with hidden answer with title "Hearth" with icon caution)
end run`

// AskPassword asks the user for the administrator's password for why, as
// sudo's askpass program; the error says when the user would not give it.
func AskPassword(ctx context.Context, r run.Runner, why string) (string, error) {
	res, err := r.Run(ctx, run.Cmd{Args: []string{"/usr/bin/osascript", "-e", askScript, why}, Timeout: askTimeout})
	if err != nil {
		return "", fmt.Errorf("ask for the password: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("ask for the password: %s", strings.TrimSpace(string(res.Stderr)))
	}
	return strings.TrimSuffix(string(res.Stdout), "\n"), nil
}
