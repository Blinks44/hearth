package ops

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ScriptSpec describes one project script launched by the user from Hearth.
type ScriptSpec struct {
	Dir    string
	Name   string
	Source string
	Argv   []string
}

// RunScript starts a project script in spec.Dir using the terminal environment,
// streams combined stdout/stderr lines in batches via containerlog.Batch, and
// terminates the entire process group cleanly when ctx is cancelled.
func RunScript(
	ctx context.Context,
	lines containerlog.LinesFunc,
	env sysenv.Env,
	spec ScriptSpec,
	onStart func(pid int),
	onLines func([]containerlog.LogLine),
) error {
	if spec.Dir == "" || len(spec.Argv) == 0 || spec.Argv[0] == "" {
		return errors.New("incomplete project script specification")
	}
	argv := slices.Clone(spec.Argv)
	if !filepath.IsAbs(argv[0]) && !strings.ContainsRune(argv[0], filepath.Separator) {
		if resolved, ok := resolveInEnvPath(env, argv[0]); ok {
			argv[0] = resolved
		} else if env == nil {
			if resolved, err := exec.LookPath(argv[0]); err == nil {
				argv[0] = resolved
			} else {
				return fmt.Errorf("command %q not found in PATH", argv[0])
			}
		} else {
			return fmt.Errorf("command %q not found in PATH", argv[0])
		}
	}
	if lines == nil {
		lines = run.CombinedLines
	}
	var prev containerlog.Level
	return containerlog.Batch(func(emit func(containerlog.LogLine)) error {
		return lines(ctx, run.Cmd{
			Args:      argv,
			Env:       env,
			Dir:       spec.Dir,
			KillGroup: true,
			OnStart:   onStart,
		}, func(raw string) {
			ln := containerlog.ParseLine(raw, prev)
			if ln.At.IsZero() {
				ln.At = time.Now()
			}
			prev = ln.Level
			emit(ln)
		})
	}, onLines)
}

func resolveInEnvPath(env sysenv.Env, command string) (string, bool) {
	if env == nil {
		return "", false
	}
	paths := sysenv.LookPathAll(command, env)
	if len(paths) == 0 {
		return "", false
	}
	return paths[0], true
}
