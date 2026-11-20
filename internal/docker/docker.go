// Package docker reads a local Docker engine for the engine view, apart
// from the scan: what the engine is and holds, read again every few
// seconds while the view shows. It runs the terminal's docker CLI against
// the engine's socket, so viewing an engine does not change the CLI's
// context.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Every is how often Watch reads the engine again.
const Every = 5 * time.Second

// Target is the engine to read: the terminal's docker CLI and the
// address of a local context's engine.
type Target struct {
	Docker, Context, Endpoint string
}

// Engine is what the engine said of itself when it was read.
type Engine struct {
	Taken time.Time
	// Version is the engine's, OS what it runs on, as Docker names them.
	Version, OS, Arch string
	CPUs              int
	Memory            int64
	// Containers counts all of them; Running, Paused and Stopped by state.
	Containers, Running, Paused, Stopped int
	Images                               int
}

// Error is an engine that did not answer, with what docker said.
type Error struct {
	Text string
}

func (e *Error) Error() string { return e.Text }

// info is the part of docker info's JSON the view reads.
type info struct {
	ServerVersion     string
	OperatingSystem   string
	Architecture      string
	NCPU              int
	MemTotal          int64
	Containers        int
	ContainersRunning int
	ContainersPaused  int
	ContainersStopped int
	Images            int
	ServerErrors      []string
}

// Read asks the engine about itself.
func Read(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Engine, error) {
	// The address, not the context: DOCKER_CONTEXT in the terminal's
	// environment would otherwise win over it.
	res, err := r.Run(ctx, t.Command(env, "info", "--format", "{{json .}}"))
	if err != nil {
		return Engine{}, fmt.Errorf("docker info: %w", err)
	}
	var in info
	jsonErr := json.Unmarshal(res.Stdout, &in)
	if len(in.ServerErrors) > 0 {
		return Engine{}, &Error{Text: strings.TrimSpace(in.ServerErrors[0])}
	}
	if res.ExitCode != 0 || jsonErr != nil {
		text := strings.TrimSpace(string(res.Stderr))
		if text == "" {
			text = fmt.Sprintf("docker info exited with %d", res.ExitCode)
		}
		return Engine{}, &Error{Text: text}
	}
	if in.ServerVersion == "" {
		return Engine{}, errors.New("docker info: no engine in the answer")
	}
	return Engine{
		Taken: time.Now(), Version: in.ServerVersion, OS: in.OperatingSystem, Arch: in.Architecture,
		CPUs: in.NCPU, Memory: in.MemTotal, Containers: in.Containers, Running: in.ContainersRunning,
		Paused: in.ContainersPaused, Stopped: in.ContainersStopped, Images: in.Images,
	}, nil
}

// Watch reads the engine now and every Every after, giving each read to
// got, until ctx is done.
func Watch(ctx context.Context, r run.Runner, env sysenv.Env, t Target, got func(Engine, error)) {
	for {
		e, err := Read(ctx, r, env, t)
		if ctx.Err() != nil {
			return
		}
		got(e, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(Every):
		}
	}
}
