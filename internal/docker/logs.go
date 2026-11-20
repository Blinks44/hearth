package docker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type LogTarget struct {
	ID, Name string
}

type Logs struct {
	Target     Target
	Containers []LogTarget
	Since      time.Duration
}

func (l Logs) command(env sysenv.Env, id string) run.Cmd {
	args := []string{"logs", "--follow", "--timestamps", "--tail", strconv.Itoa(containerlog.MaxLines)}
	if l.Since > 0 {
		args = append(args, "--since", l.Since.String())
	}
	return l.Target.Command(env, append(args, "--", id)...)
}

// Stream combines the selected containers' stdout and stderr. A stopped
// container returns its existing log and ends its stream.
func (l Logs) Stream(ctx context.Context, lines containerlog.LinesFunc, env sysenv.Env, got func([]containerlog.LogLine)) error {
	var workers sync.WaitGroup
	var publish sync.Mutex
	failures := make([]error, len(l.Containers))
	for i, source := range l.Containers {
		workers.Go(func() {
			err := streamLog(ctx, lines, env, l, source, func(batch []containerlog.LogLine) {
				publish.Lock()
				defer publish.Unlock()
				got(batch)
			})
			if err != nil && source.Name != "" {
				err = fmt.Errorf("%s: %w", source.Name, err)
			}
			failures[i] = err
		})
	}
	workers.Wait()
	return errors.Join(failures...)
}

func streamLog(ctx context.Context, lines containerlog.LinesFunc, env sysenv.Env, l Logs, target LogTarget, got func([]containerlog.LogLine)) error {
	if !ContainerID(target.ID) {
		return errors.New("docker logs: invalid container ID")
	}
	var previous containerlog.Level
	var diagnostic string
	err := containerlog.Batch(func(emit func(containerlog.LogLine)) error {
		return lines(ctx, l.command(env, target.ID), func(raw string) {
			line := containerlog.ParseLine(raw, previous)
			if line.At.IsZero() {
				diagnostic = raw
			}
			previous = line.Level
			line.Pod = target.Name
			emit(line)
		})
	}, got)
	if err != nil && ctx.Err() == nil && diagnostic != "" {
		return fmt.Errorf("docker logs: %s: %w", strings.TrimSpace(diagnostic), err)
	}
	return err
}

// ShellCommand probes bash inside the selected container before opening an
// interactive shell. The terminal keeps the same pinned Docker endpoint.
func ShellCommand(ctx context.Context, r run.Runner, env sysenv.Env, t Target, id string) (run.Cmd, error) {
	if !ContainerID(id) {
		return run.Cmd{}, errors.New("docker exec: invalid container ID")
	}
	probe := t.Command(env, "exec", "--", id, "bash", "--version")
	res, err := r.Run(ctx, probe)
	if err != nil {
		return run.Cmd{}, fmt.Errorf("docker exec: %w", err)
	}
	shell := "sh"
	if res.ExitCode == 0 {
		shell = "bash"
	}
	return t.Command(env, "exec", "-it", "--", id, shell), nil
}
