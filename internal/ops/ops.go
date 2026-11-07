// Package ops changes the system. Every change is a Plan the user sees in
// full before it runs: the commands as they are, what is worth knowing and
// what will be different after. Execute then runs its steps one by one,
// with their output as it comes, stops at the first that fails or when
// the user stops it, and checks the result.
package ops

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// DefaultTimeout ends a step that left Step.Timeout zero: installs
// download, so it is long.
const DefaultTimeout = 30 * time.Minute

// LogLines is how many of the last lines of output State keeps.
const LogLines = 200

// Step is one command of a plan, as argv: never a shell string.
type Step struct {
	Argv []string
	// Note says what else the step does, shown after the command, as
	// "wait for the engine to start".
	Note string
	// Timeout ends the step after it; zero is DefaultTimeout.
	Timeout time.Duration
}

// Note is something worth knowing before a plan runs, as that it asks for
// the administrator's password or what a third-party app's terms say.
type Note struct {
	Title, Text string
	// Tag is a short word on the right, as "sudo"; URL a page to open
	// instead, as the terms of use.
	Tag, URL string
}

// Plan is a change to the system the user confirms before it runs.
type Plan struct {
	// Title names the change, as "Install Docker Desktop"; Why says how
	// it goes and why.
	Title, Why string
	Steps      []Step
	Notes      []Note
	// Action labels the button that runs it, as "Accept the terms and
	// install"; empty is a plain "Run".
	Action string
	// Changes says what will be different after, one line each, shown
	// once the plan ran.
	Changes []string
	// Env is the environment of the steps, as KEY=value: the terminal's,
	// so they run as the user's own commands would.
	Env []string
	// Verify checks the result after the steps ran, saying what it found,
	// as "docker 27.4.1 answers"; nil checks nothing.
	Verify func(ctx context.Context) (string, error)
}

// Phase is where an execution of a plan is.
type Phase int

const (
	Running Phase = iota
	Verifying
	Succeeded
	Failed      // a step failed; those after it did not run
	NotVerified // the steps ran but the check found otherwise
	Stopped     // the user stopped it
)

// Over reports that the execution ended.
func (p Phase) Over() bool { return p >= Succeeded }

// StepStatus is how a step went.
type StepStatus int

const (
	Waiting StepStatus = iota
	Busy
	Done
	StepFailed
	StepStopped
	Skipped
)

// State is how an execution goes, given whole to the caller at every
// change.
type State struct {
	Phase Phase
	Steps []StepStatus
	// Current is the step running or the last that ran.
	Current int
	// Log holds the last LogLines lines of the current step's output.
	Log []string
	// ExitCode is the failed step's exit code; -1 when it did not start
	// or end by itself.
	ExitCode int
	// Err says why a step failed or the check did not pass; Verified
	// what the check found.
	Err, Verified string
}

// Command shows argv as a terminal would take it, quoting the arguments
// that need it.
func Command(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = a
		if a == "" || strings.ContainsAny(a, " \t\"'$\\;&|<>*?()[]{}`!#~") {
			parts[i] = strconv.Quote(a)
		}
	}
	return strings.Join(parts, " ")
}

// Execute runs p's steps with r, writing all their output to journal, and
// gives update the state at every change, from this goroutine or the
// commands' output goroutines, one at a time. Canceling ctx stops the
// step running and skips the rest. It returns the last state.
func Execute(ctx context.Context, r run.Runner, p Plan, journal io.Writer, update func(State)) State {
	e := &execution{journal: journal, update: update}
	e.state = State{Phase: Running, Steps: make([]StepStatus, len(p.Steps))}
	e.printf("== %s  %s\n", time.Now().Format("2006-01-02 15:04:05"), p.Title)
	e.emit()
	for i, step := range p.Steps {
		e.mu.Lock()
		e.state.Current, e.state.Steps[i], e.state.Log = i, Busy, nil
		e.mu.Unlock()
		e.printf("$ %s\n", Command(step.Argv))
		e.emit()
		timeout := step.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		res, err := r.Run(ctx, run.Cmd{Args: step.Argv, Env: p.Env, Timeout: timeout, Output: e})
		e.mu.Lock()
		e.flushLine()
		switch {
		case ctx.Err() != nil:
			e.end(i, StepStopped, Stopped, -1, "")
		case err != nil:
			e.end(i, StepFailed, Failed, -1, err.Error())
		case res.ExitCode != 0:
			e.end(i, StepFailed, Failed, res.ExitCode, "")
		default:
			e.state.Steps[i] = Done
			e.mu.Unlock()
			e.printf("exit 0\n")
			e.emit()
			continue
		}
		e.mu.Unlock()
		e.emit()
		return e.snapshot()
	}
	if p.Verify != nil {
		e.mu.Lock()
		e.state.Phase = Verifying
		e.mu.Unlock()
		e.emit()
		found, err := p.Verify(ctx)
		e.mu.Lock()
		switch {
		case ctx.Err() != nil:
			e.state.Phase = Stopped
		case err != nil:
			e.state.Phase, e.state.Err = NotVerified, err.Error()
		default:
			e.state.Phase, e.state.Verified = Succeeded, found
		}
		e.mu.Unlock()
		e.printf("verify: %s%s\n", found, errText(err))
	} else {
		e.mu.Lock()
		e.state.Phase = Succeeded
		e.mu.Unlock()
	}
	e.emit()
	return e.snapshot()
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return " " + err.Error()
}

// execution is one run of a plan. It is the io.Writer the steps' output
// goes to, splitting it into lines for the log.
type execution struct {
	mu      sync.Mutex
	state   State
	line    []byte // the output after the last line break
	last    time.Time
	journal io.Writer
	update  func(State)
	emitMu  sync.Mutex // one update at a time
}

// end marks step i and the steps after it, with e.mu held.
func (e *execution) end(i int, status StepStatus, phase Phase, code int, why string) {
	e.state.Steps[i], e.state.Phase, e.state.ExitCode, e.state.Err = status, phase, code, why
	for j := i + 1; j < len(e.state.Steps); j++ {
		e.state.Steps[j] = Skipped
	}
	if why == "" && code != -1 {
		why = "exit " + strconv.Itoa(code)
	}
	if why == "" {
		why = "stopped"
	}
	_, _ = fmt.Fprintf(e.journal, "%s\n", why)
}

func (e *execution) Write(p []byte) (int, error) {
	e.mu.Lock()
	_, _ = e.journal.Write(p)
	for _, b := range p {
		switch b {
		case '\n':
			e.flushLine()
		case '\r':
			// A progress bar draws over its line: keep the last drawing.
			e.line = e.line[:0]
		default:
			e.line = append(e.line, b)
		}
	}
	// Output can come in fast: the caller redraws for it, so a tenth of a
	// second is soon enough.
	now := time.Now()
	due := now.Sub(e.last) >= 100*time.Millisecond
	if due {
		e.last = now
	}
	e.mu.Unlock()
	if due {
		e.emit()
	}
	return len(p), nil
}

// flushLine moves the unfinished line to the log, with e.mu held.
func (e *execution) flushLine() {
	if len(e.line) == 0 {
		return
	}
	e.state.Log = append(e.state.Log, string(e.line))
	if n := len(e.state.Log) - LogLines; n > 0 {
		e.state.Log = append([]string(nil), e.state.Log[n:]...)
	}
	e.line = e.line[:0]
}

func (e *execution) printf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = fmt.Fprintf(e.journal, format, args...)
}

func (e *execution) snapshot() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.state
	s.Steps = append([]StepStatus(nil), s.Steps...)
	s.Log = append([]string(nil), s.Log...)
	if len(e.line) > 0 {
		s.Log = append(s.Log, string(e.line))
	}
	return s
}

func (e *execution) emit() {
	e.emitMu.Lock()
	defer e.emitMu.Unlock()
	e.update(e.snapshot())
}
