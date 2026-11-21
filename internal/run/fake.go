package run

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
)

// Fake is a Runner for tests: it answers by argv and records the calls.
// An argv without an answer fails as a missing program does.
type Fake struct {
	mu      sync.Mutex
	answers map[string]fakeAnswer
	calls   []Cmd
}

type fakeAnswer struct {
	res Result
	err error
}

// Set makes Run answer argv with res and err.
func (f *Fake) Set(res Result, err error, argv ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.answers == nil {
		f.answers = map[string]fakeAnswer{}
	}
	f.answers[key(argv)] = fakeAnswer{res, err}
}

// Stdout makes Run answer argv with exit code 0 and that output.
func (f *Fake) Stdout(out string, argv ...string) {
	f.Set(Result{Stdout: []byte(out)}, nil, argv...)
}

// Calls returns the commands Run received, in order.
func (f *Fake) Calls() []Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Cmd(nil), f.calls...)
}

// Run implements Runner.
func (f *Fake) Run(ctx context.Context, c Cmd) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: -1}, err
	}
	a, ok := f.answers[key(c.Args)]
	if !ok {
		return Result{}, fmt.Errorf("run %s: %w", strings.Join(c.Args, " "), exec.ErrNotFound)
	}
	if c.Output != nil {
		_, _ = c.Output.Write(a.res.Stdout)
		_, _ = c.Output.Write(a.res.Stderr)
	}
	res := a.res
	res.Stdout, res.Stderr = slices.Clone(res.Stdout), slices.Clone(res.Stderr)
	return res, a.err
}

// key joins an argv with a separator no argument contains.
func key(argv []string) string { return strings.Join(argv, "\x00") }
