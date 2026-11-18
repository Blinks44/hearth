//go:build !darwin

package ops

import (
	"context"
	"errors"

	"github.com/Blinks44/hearth/internal/run"
)

// AskPassword asks for the password only on macOS, where Homebrew's casks
// need it.
func AskPassword(context.Context, run.Runner, string) (string, error) {
	return "", errors.New("ask for the password: only on macOS")
}
