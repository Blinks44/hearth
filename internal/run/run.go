// Package run starts external commands for detectors and operations: an
// argv without a shell, a timeout, a cap on the output kept, and no console
// window on Windows. Code that runs commands takes a Runner, so tests can
// pass a Fake.
package run

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Defaults for a Cmd that leaves them zero.
const (
	DefaultTimeout   = 5 * time.Second
	DefaultMaxOutput = 1 << 20
)

// Cmd describes one command to run.
type Cmd struct {
	// Args is the argv: Args[0] is the program, found as exec.Command
	// finds it, in Hearth's own PATH when it has no separator: pass a full
	// path for a program of the terminal's.
	Args []string
	// Env is the whole environment of the command, as KEY=value. Nil
	// inherits Hearth's own, which is not the terminal's: pass sysenv's.
	Env []string
	// Dir is the working directory; empty is Hearth's own.
	Dir string
	// Timeout ends the command after it; zero is DefaultTimeout.
	Timeout time.Duration
	// MaxOutput caps the bytes kept of stdout and of stderr each; zero is
	// DefaultMaxOutput. The rest is read and dropped.
	MaxOutput int
	// Stdin supplies input without putting it in argv or the journal.
	Stdin io.Reader
	// Output, when set, also gets stdout and stderr as the command writes
	// them, all of it, from two goroutines at once.
	Output io.Writer
}

// Result is what a command that ran left. A non-zero ExitCode is a result,
// not an error: callers decide what it means.
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
	// Truncated reports that stdout or stderr went past MaxOutput.
	Truncated bool
	Duration  time.Duration
}

// Runner runs commands. Run returns an error when the command could not
// start (errors.Is exec.ErrNotFound for a missing program), timed out
// (context.DeadlineExceeded) or was canceled (context.Canceled).
type Runner interface {
	Run(ctx context.Context, cmd Cmd) (Result, error)
}

// Exec runs commands on this machine.
type Exec struct{}

// Run implements Runner.
func (Exec) Run(ctx context.Context, c Cmd) (Result, error) {
	if len(c.Args) == 0 {
		return Result{}, errors.New("run: empty argv")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	limit := c.MaxOutput
	if limit <= 0 {
		limit = DefaultMaxOutput
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.Stdin = c.Stdin
	// A child that left grandchildren holding the pipes open must not keep
	// Run waiting past its end.
	cmd.WaitDelay = time.Second
	detach(cmd)
	stdout := &capped{limit: limit}
	stderr := &capped{limit: limit}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if c.Output != nil {
		cmd.Stdout, cmd.Stderr = io.MultiWriter(stdout, c.Output), io.MultiWriter(stderr, c.Output)
	}

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:    stdout.buf.Bytes(),
		Stderr:    stderr.buf.Bytes(),
		Truncated: stdout.dropped || stderr.dropped,
		Duration:  time.Since(start),
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		res.ExitCode = -1
		return res, fmt.Errorf("run %s: %w", c.Args[0], ctxErr)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	return res, nil
}

// Lines runs a command that keeps writing, as docker events does, and
// gives line every line of its stdout as it comes, on this goroutine. It
// returns when the command ends, with an error unless it exited with 0,
// or when ctx does. c.Output, when set, gets its stderr. c.Timeout does
// not apply; a line longer than c.MaxOutput stops the command with an
// error.
func Lines(ctx context.Context, c Cmd, line func(string)) error {
	return readLines(ctx, c, line, false)
}

// CombinedLines delivers stdout and stderr through the same pipe in write order.
func CombinedLines(ctx context.Context, c Cmd, line func(string)) error {
	return readLines(ctx, c, line, true)
}

func readLines(ctx context.Context, c Cmd, line func(string), combined bool) error {
	if len(c.Args) == 0 {
		return errors.New("run: empty argv")
	}
	limit := c.MaxOutput
	if limit <= 0 {
		limit = DefaultMaxOutput
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.WaitDelay = time.Second
	detach(cmd)
	cmd.Stderr = c.Output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	if combined {
		cmd.Stderr = cmd.Stdout
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	// A child of the command may still hold the write end after
	// CommandContext stops the command. Unblock the read on cancellation.
	stopRead := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stopRead()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(nil, limit)
	for sc.Scan() {
		line(sc.Text())
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		cancel()
		_ = cmd.Wait() // killed: the line is the error
		return fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	err = cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("run %s: %w", c.Args[0], ctxErr)
	}
	if err != nil {
		return fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	return nil
}

// Command is c as an exec.Cmd for a caller that talks to the program
// over its pipes, as an MCP client does: no shell, no console window on
// Windows, and ended with ctx. c.Timeout, c.MaxOutput and c.Output do not
// apply.
func Command(ctx context.Context, c Cmd) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.WaitDelay = time.Second
	detach(cmd)
	return cmd
}

// capped keeps the first limit bytes written to it and drops the rest.
type capped struct {
	buf     bytes.Buffer
	limit   int
	dropped bool
}

func (w *capped) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room < len(p) {
		w.dropped = true
		w.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return w.buf.Write(p)
}
