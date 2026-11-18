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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
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
	// Admin runs it as the administrator, asking the user through the
	// system's own dialog each time: never a silent sudo.
	Admin bool
	// Edit, in place of Argv, changes a file of the user's after a copy
	// of it, which comes back if the plan does not get through: a later
	// step fails or is stopped, or the check does not pass.
	Edit *FileEdit
	// Skill installs previewed files into a new directory.
	Skill *SkillInstall
	// Instructions creates a global instruction file or file symlink.
	Instructions *InstructionInstall
	// UserPath, in place of Argv, sets the user's Path in the Windows
	// registry, which comes back as an Edit does.
	UserPath *UserPathEdit
	// UserVar, in place of Argv, sets one of the user's environment
	// variables in the Windows registry, which comes back as an Edit does.
	UserVar *UserVarEdit
	// Remove, in place of Argv, removes this folder and all in it for
	// good: it does not come back if the plan does not get through.
	Remove string
	// Env adds to the plan's environment for this step, as KEY=value.
	Env []string
	// Runs, when Argv is a wrapper, is the command it runs, which the
	// plan shows in Argv's place.
	Runs []string
	// Undo, for a command, is the command that puts back what it did if
	// the plan does not get through, as an Edit's copy does; its Note,
	// if any, names what came back in State.Restored.
	Undo *Step
}

// Shown is the command a step shows: what it runs.
func (s Step) Shown() []string {
	if len(s.Runs) > 0 {
		return s.Runs
	}
	return s.Argv
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
	// Before refuses a stale plan before its first side effect.
	Before func(context.Context) error
	// Isolated runs commands outside project directories to avoid config hooks.
	Isolated bool
	Steps    []Step
	Notes    []Note
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
	// Parts are the first steps of the plan's parts that stand on their
	// own, as the new computer's tools: a step that fails puts back only
	// what its part changed. None makes the plan one part.
	Parts []int
}

// Phase is where an execution of a plan is.
type Phase int

const (
	Running Phase = iota
	Verifying
	Succeeded
	Failed      // a step failed; those after it did not run
	NotVerified // the steps ran but the check found otherwise
	Stopped     // the user stopped a step, or the check after them all
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
	// Restored are the files put back from their copies as the plan did
	// not get through.
	Restored []string
	// InstructionCopyAvailable reports a refused file symlink on Windows.
	InstructionCopyAvailable bool
}

// Command shows argv as a terminal would take it, quoting the arguments
// that need it as the system's terminal does.
func Command(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = quoteArg(a)
		// A step's Runs shows && between the commands it runs one after
		// the other, as a shell does.
		if a == "&&" {
			parts[i] = a
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
	if p.Before != nil {
		if err := p.Before(ctx); err != nil {
			e.mu.Lock()
			e.state.Phase, e.state.Err = NotVerified, err.Error()
			for i := range e.state.Steps {
				e.state.Steps[i] = Skipped
			}
			e.mu.Unlock()
			e.emit()
			return e.snapshot()
		}
	}
	commandDir := ""
	if p.Isolated {
		dir, err := os.MkdirTemp("", "hearth-operation-")
		if err != nil {
			e.state.Phase, e.state.Err = Failed, err.Error()
			e.emit()
			return e.snapshot()
		}
		commandDir = dir
		defer func() { _ = os.RemoveAll(dir) }()
	}
	// applied are the changes to the user's files and settings, which a
	// plan that does not get through puts back.
	type applied struct {
		name    string
		restore func() error
	}
	var changes []applied
	// dropped are the copies of the files that go once the plan got through.
	var dropped []string
	undo := func() {
		for _, a := range slices.Backward(changes) {
			err := a.restore()
			e.printf("restore %s%s\n", a.name, errText(err))
			e.mu.Lock()
			// An undo command that names nothing is not a thing put back.
			if err == nil && !strings.HasPrefix(a.name, "$ ") {
				e.state.Restored = append(e.state.Restored, a.name)
			}
			e.mu.Unlock()
		}
	}
	for i, step := range p.Steps {
		// The parts done keep what they changed.
		if slices.Contains(p.Parts, i) {
			changes = nil
		}
		e.mu.Lock()
		e.state.Current, e.state.Steps[i], e.state.Log = i, Busy, nil
		e.mu.Unlock()
		if step.Skill != nil || step.Instructions != nil {
			var name string
			var restore func() error
			var err error
			if in := step.Instructions; in != nil {
				name = in.Observed.Native.Path
				e.printf("create instructions %s\n", name)
				e.emit()
				restore, err = applyInstructionInstall(ctx, *in)
				e.mu.Lock()
				e.state.InstructionCopyAvailable = in.Mode == "link" && instructionLinkDenied(err)
				e.mu.Unlock()
			} else {
				name = step.Skill.Path
				e.printf("install skill %s\n", name)
				e.emit()
				restore, err = applySkillInstall(ctx, *step.Skill)
			}
			if err == nil {
				changes = append(changes, applied{name, restore})
			}
			e.mu.Lock()
			if err != nil {
				e.end(i, StepFailed, Failed, -1, err.Error())
			} else {
				e.state.Steps[i] = Done
			}
			e.mu.Unlock()
			if err != nil {
				undo()
				e.emit()
				return e.snapshot()
			}
			e.emit()
			continue
		}
		if step.Remove != "" {
			e.printf("remove %s\n", step.Remove)
			e.emit()
			err := removeAll(step.Remove)
			e.mu.Lock()
			if err != nil {
				e.end(i, StepFailed, Failed, -1, err.Error())
			} else {
				e.state.Steps[i] = Done
			}
			e.mu.Unlock()
			if err != nil {
				undo()
				e.emit()
				return e.snapshot()
			}
			e.emit()
			continue
		}
		if step.Edit != nil || step.UserPath != nil || step.UserVar != nil {
			var err error
			if v := step.UserVar; v != nil {
				name := UserVarsName + `\` + v.Name
				e.printf("edit %s\n", name)
				e.emit()
				var undo func() error
				if undo, err = applyUserVar(*v); err == nil {
					changes = append(changes, applied{name, undo})
				}
			} else if edit := step.Edit; edit != nil {
				e.printf("edit %s\n", edit.Path)
				e.emit()
				var backup string
				if backup, err = applyEdit(*edit, time.Now()); err == nil {
					changes = append(changes, applied{edit.Path, func() error { return restoreEdit(*edit, backup) }})
					if edit.DropCopy && backup != "" {
						dropped = append(dropped, backup)
					}
					e.printf("backup %s\n", backup)
				}
			} else {
				e.printf("edit %s\n", UserPathName)
				e.emit()
				var before string
				if before, err = applyUserPath(*step.UserPath); err == nil {
					// The value that was is the copy: the log keeps it.
					e.printf("backup %s\n", before)
					changes = append(changes, applied{UserPathName, func() error { return restoreUserPath(before) }})
				}
			}
			e.mu.Lock()
			if err != nil {
				e.end(i, StepFailed, Failed, -1, err.Error())
			} else {
				e.state.Steps[i] = Done
			}
			e.mu.Unlock()
			if err != nil {
				undo()
				e.emit()
				return e.snapshot()
			}
			e.emit()
			continue
		}
		// The log keeps what the plan showed: a step's real argv may carry
		// a secret, as an MCP server's token.
		e.printf("$ %s\n", Command(step.Shown()))
		e.emit()
		timeout := step.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		// The program is found in the terminal's PATH, as the user's own
		// command would be: Hearth's own PATH, as an app's, may lack it.
		env := slices.Concat(p.Env, step.Env)
		argv := fullPath(step.Argv, env)
		if step.Admin {
			argv = elevate(argv, p.Title)
		}
		res, err := r.Run(ctx, run.Cmd{Args: argv, Env: env, Dir: commandDir, Timeout: timeout, Output: e})
		e.mu.Lock()
		e.flushLine()
		refused := ""
		if step.Admin {
			refused = adminRefused(res.ExitCode, string(res.Stdout)+string(res.Stderr))
		}
		switch {
		case ctx.Err() != nil:
			e.end(i, StepStopped, Stopped, -1, "")
		case err != nil:
			e.end(i, StepFailed, Failed, -1, err.Error())
		case refused != "":
			e.end(i, StepFailed, Failed, res.ExitCode, refused)
		case res.ExitCode != 0:
			e.end(i, StepFailed, Failed, res.ExitCode, "")
		default:
			e.state.Steps[i] = Done
			e.mu.Unlock()
			e.printf("exit 0\n")
			if u := step.Undo; u != nil {
				name := u.Note
				if name == "" {
					name = "$ " + Command(u.Shown())
				}
				changes = append(changes, applied{name, func() error {
					return runUndo(ctx, r, *u, env, commandDir)
				}})
			}
			e.emit()
			continue
		}
		e.mu.Unlock()
		undo()
		e.emit()
		return e.snapshot()
	}
	if p.Verify != nil {
		e.mu.Lock()
		e.state.Phase = Verifying
		e.mu.Unlock()
		e.emit()
		found, err := verify(ctx, p.Verify)
		e.mu.Lock()
		switch {
		case ctx.Err() != nil:
			e.state.Phase = Stopped
		case err != nil:
			e.state.Phase, e.state.Err = NotVerified, err.Error()
		default:
			e.state.Phase, e.state.Verified = Succeeded, found
		}
		phase := e.state.Phase
		e.mu.Unlock()
		e.printf("verify: %s%s\n", found, errText(err))
		// Stopping the check is not its failing: what the steps did
		// stays, unchecked.
		if phase == NotVerified {
			undo()
		}
	} else {
		e.mu.Lock()
		e.state.Phase = Succeeded
		e.mu.Unlock()
	}
	if e.state.Phase == Succeeded {
		for _, b := range dropped {
			err := os.Remove(b)
			e.printf("remove %s%s\n", b, errText(err))
		}
	}
	e.emit()
	return e.snapshot()
}

// verify runs check, and runs it again a few times a second apart while
// it fails: a program that asked for the administrator, as nvm-windows
// does, may leave its elevated part still at work as it exits.
func verify(ctx context.Context, check func(context.Context) (string, error)) (string, error) {
	found, err := check(ctx)
	for range verifyTries - 1 {
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return found, err
		case <-time.After(verifyPause):
		}
		found, err = check(ctx)
	}
	return found, err
}

// verifyTries and verifyPause are how often and how far apart verify
// checks; tests shorten the pause.
var (
	verifyTries = 3
	verifyPause = time.Second
)

// fullPath returns argv with its program found in env's PATH: the
// runner would look in Hearth's own, and an elevated command runs with a
// PATH of its own. argv as it is when the program is not found, for the
// step to fail saying so.
func fullPath(argv []string, env []string) []string {
	if len(argv) == 0 || filepath.IsAbs(argv[0]) {
		return argv
	}
	found := sysenv.LookPathAll(argv[0], env)
	if len(found) == 0 {
		return argv
	}
	return append([]string{found[0]}, argv[1:]...)
}

// runUndo runs u, a step's Undo, with the step's environment and folder,
// even after the plan was stopped: what the step did must come back.
func runUndo(ctx context.Context, r run.Runner, u Step, env []string, dir string) error {
	timeout := u.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	env = slices.Concat(env, u.Env)
	res, err := r.Run(context.WithoutCancel(ctx), run.Cmd{Args: fullPath(u.Argv, env), Env: env, Dir: dir, Timeout: timeout})
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("%s: exit code %d", Command(u.Shown()), res.ExitCode)
	}
	return err
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
	mu    sync.Mutex
	state State
	line  []byte // the output after the last line break
	// esc is the terminal control sequence being read, from its ESC.
	esc     []byte
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
		if len(e.esc) > 0 || b == 0x1b {
			e.control(b)
			continue
		}
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

// control reads b, a byte of a terminal control sequence, with e.mu
// held. A progress bar, as ollama pull's, draws with them even where no
// terminal reads: moving to the line's start is as '\r', moving up n
// lines takes back the n last lines it drew over, and the rest, colors
// and the cursor's look, show nothing.
func (e *execution) control(b byte) {
	e.esc = append(e.esc, b)
	switch {
	case len(e.esc) == 2 && b != '[':
		// Not a CSI sequence: ESC and the byte after it.
	case len(e.esc) < 3 || b < 0x40 || b > 0x7e:
		if len(e.esc) < 64 {
			return
		}
	case b == 'G':
		e.line = e.line[:0]
	case b == 'A':
		n, err := strconv.Atoi(string(e.esc[2 : len(e.esc)-1]))
		if err != nil {
			n = 1
		}
		e.line = e.line[:0]
		e.state.Log = e.state.Log[:max(0, len(e.state.Log)-n)]
	}
	e.esc = e.esc[:0]
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
	s.Restored = append([]string(nil), s.Restored...)
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
