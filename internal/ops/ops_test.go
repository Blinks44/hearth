package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// recorder keeps every state Execute gave.
type recorder struct {
	mu     sync.Mutex
	states []State
}

func (r *recorder) update(s State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, s)
}

func plan(steps ...[]string) Plan {
	p := Plan{Title: "Install thing"}
	for _, argv := range steps {
		p.Steps = append(p.Steps, Step{Argv: argv})
	}
	return p
}

func init() {
	verifyPause, engineStartPause, undoPause = time.Millisecond, time.Millisecond, time.Millisecond
}

func TestExecute(t *testing.T) {
	fail := errors.New("broken")
	tests := []struct {
		name     string
		plan     Plan
		answers  func(f *run.Fake)
		phase    Phase
		steps    []StepStatus
		code     int
		err      string
		verified string
		log      []string
	}{
		{
			name: "success",
			plan: plan([]string{"brew", "install", "thing"}, []string{"thing", "--version"}),
			answers: func(f *run.Fake) {
				f.Stdout("==> Downloading\n10%\r50%\r100%\n==> Pouring", "brew", "install", "thing")
				f.Stdout("thing 1.0\n", "thing", "--version")
			},
			phase: Succeeded,
			steps: []StepStatus{Done, Done},
			log:   []string{"thing 1.0"},
		},
		{
			name: "a failing step skips the rest",
			plan: plan([]string{"brew", "install", "thing"}, []string{"thing", "--version"}),
			answers: func(f *run.Fake) {
				f.Set(run.Result{Stderr: []byte("Error: no network\n"), ExitCode: 1}, nil, "brew", "install", "thing")
			},
			phase: Failed,
			steps: []StepStatus{StepFailed, Skipped},
			code:  1,
			log:   []string{"Error: no network"},
		},
		{
			name:  "a missing program fails its step",
			plan:  plan([]string{"brew", "install", "thing"}),
			phase: Failed,
			steps: []StepStatus{StepFailed},
			code:  -1,
			err:   exec.ErrNotFound.Error(),
		},
		{
			name: "verified",
			plan: func() Plan {
				p := plan([]string{"brew", "install", "thing"})
				p.Verify = func(context.Context) (string, error) { return "thing 1.0 answers", nil }
				return p
			}(),
			answers:  func(f *run.Fake) { f.Stdout("", "brew", "install", "thing") },
			phase:    Succeeded,
			steps:    []StepStatus{Done},
			verified: "thing 1.0 answers",
		},
		{
			name: "the check passes on a later try",
			plan: func() Plan {
				p := plan([]string{"brew", "install", "thing"})
				tries := 0
				p.Verify = func(context.Context) (string, error) {
					if tries++; tries < 2 {
						return "", fail
					}
					return "thing 1.0 answers", nil
				}
				return p
			}(),
			answers:  func(f *run.Fake) { f.Stdout("", "brew", "install", "thing") },
			phase:    Succeeded,
			steps:    []StepStatus{Done},
			verified: "thing 1.0 answers",
		},
		{
			name: "the check fails",
			plan: func() Plan {
				p := plan([]string{"brew", "install", "thing"})
				p.Verify = func(context.Context) (string, error) { return "", fail }
				return p
			}(),
			answers: func(f *run.Fake) { f.Stdout("", "brew", "install", "thing") },
			phase:   NotVerified,
			steps:   []StepStatus{Done},
			err:     "broken",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &run.Fake{}
			if tt.answers != nil {
				tt.answers(f)
			}
			var journal strings.Builder
			var rec recorder
			got := Execute(t.Context(), f, tt.plan, &journal, rec.update)
			if got.Phase != tt.phase || !slices.Equal(got.Steps, tt.steps) || got.ExitCode != tt.code {
				t.Errorf("got phase %d steps %v code %d, want %d %v %d", got.Phase, got.Steps, got.ExitCode, tt.phase, tt.steps, tt.code)
			}
			if !strings.Contains(got.Err, tt.err) || (tt.err == "") != (got.Err == "") {
				t.Errorf("Err = %q, want %q", got.Err, tt.err)
			}
			if got.Verified != tt.verified {
				t.Errorf("Verified = %q, want %q", got.Verified, tt.verified)
			}
			if tt.log != nil && !slices.Equal(got.Log, tt.log) {
				t.Errorf("Log = %q, want %q", got.Log, tt.log)
			}
			if last := rec.states[len(rec.states)-1]; last.Phase != got.Phase {
				t.Errorf("last update has phase %d, Execute returned %d", last.Phase, got.Phase)
			}
			if !strings.Contains(journal.String(), "Install thing") || !strings.Contains(journal.String(), "$ brew install thing") {
				t.Errorf("journal misses the title or the command:\n%s", journal.String())
			}
		})
	}
}

func TestExecuteProgressLines(t *testing.T) {
	f := &run.Fake{}
	f.Stdout("==> Downloading\n10%\r50%\r100%\n==> Pouring", "brew", "install", "thing")
	got := Execute(t.Context(), f, plan([]string{"brew", "install", "thing"}), &strings.Builder{}, func(State) {})
	want := []string{"==> Downloading", "100%", "==> Pouring"}
	if !slices.Equal(got.Log, want) {
		t.Errorf("Log = %q, want %q", got.Log, want)
	}
}

// A progress bar drawn with terminal control sequences, as ollama pull's,
// keeps its last drawing, without the sequences.
func TestExecuteControlSequences(t *testing.T) {
	f := &run.Fake{}
	out := "\x1b[?2026h\x1b[?25l\x1b[1Gpulling manifest ⠋\x1b[K\x1b[?2026l" +
		"\x1b[1Gpulling manifest\x1b[K\npulling 797b: 50%\x1b[K" +
		"\x1b[A\x1b[1Gpulling manifest\x1b[K\npulling 797b: 100%\x1b[K\nverifying\x1b[K" +
		"\n\x1b[32msuccess\x1b[0m\x1b[?25h"
	f.Stdout(out, "ollama", "pull", "qwen3")
	got := Execute(t.Context(), f, plan([]string{"ollama", "pull", "qwen3"}), &strings.Builder{}, func(State) {})
	want := []string{"pulling manifest", "pulling 797b: 100%", "verifying", "success"}
	if !slices.Equal(got.Log, want) {
		t.Errorf("Log = %q, want %q", got.Log, want)
	}
}

// blocking runs its first command until the context ends and answers the
// rest at once.
type blocking struct {
	started chan struct{}
	calls   int
}

func (b *blocking) Run(ctx context.Context, c run.Cmd) (run.Result, error) {
	b.calls++
	if b.calls > 1 {
		return run.Result{}, nil
	}
	_, _ = c.Output.Write([]byte("downloading\n"))
	close(b.started)
	<-ctx.Done()
	return run.Result{ExitCode: -1}, ctx.Err()
}

func TestExecuteStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	b := &blocking{started: make(chan struct{})}
	go func() {
		<-b.started
		cancel()
	}()
	var journal strings.Builder
	got := Execute(ctx, b, plan([]string{"brew", "install", "thing"}, []string{"thing", "--version"}), &journal, func(State) {})
	if got.Phase != Stopped || !slices.Equal(got.Steps, []StepStatus{StepStopped, Skipped}) {
		t.Errorf("got phase %d steps %v, want Stopped [StepStopped Skipped]", got.Phase, got.Steps)
	}
	if b.calls != 1 {
		t.Errorf("ran %d commands after the stop, want only the first", b.calls)
	}
	if !strings.Contains(journal.String(), "stopped") {
		t.Errorf("journal does not say it stopped:\n%s", journal.String())
	}
}

func TestCommand(t *testing.T) {
	for argv, want := range map[string]string{
		"brew\x00install\x00--cask\x00docker": "brew install --cask docker",
		"open\x00-a\x00Docker Desktop":        `open -a "Docker Desktop"`,
		"echo\x00":                            `echo ""`,
		"apt-get\x00update\x00&&\x00apt-get\x00install\x00-y\x00jq": "apt-get update && apt-get install -y jq",
	} {
		if got := Command(strings.Split(argv, "\x00")); got != want {
			t.Errorf("Command(%q) = %s, want %s", argv, got, want)
		}
	}
}

// A step's program is found in the plan's PATH, the terminal's, not in
// Hearth's own.
func TestExecuteFindsProgramInPlanPath(t *testing.T) {
	dir := t.TempDir()
	name := "hearth-test-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &run.Fake{}
	p := Plan{Title: "t", Env: []string{"PATH=" + dir, "Path=" + dir, "PATHEXT=.EXE"}, Steps: []Step{{Argv: []string{"hearth-test-tool", "go"}}}}
	Execute(context.Background(), r, p, io.Discard, func(State) {})
	if calls, want := r.Calls(), filepath.Join(dir, name); len(calls) != 1 || !strings.EqualFold(calls[0].Args[0], want) {
		t.Errorf("ran %+v, want %q", calls, want)
	}
}

// A step with Askpass points sudo inside it to Hearth's program, which
// says the plan's title; others get no askpass.
func TestExecuteAskpass(t *testing.T) {
	old := AskpassProgram
	AskpassProgram = "/Applications/Hearth.app/Contents/MacOS/Hearth"
	t.Cleanup(func() { AskpassProgram = old })
	r := &run.Fake{}
	p := Plan{Title: "Update 5 packages", Steps: []Step{
		{Argv: []string{"/opt/homebrew/bin/brew", "upgrade", "--formula", "jq"}},
		{Argv: []string{"/opt/homebrew/bin/brew", "upgrade", "--cask", "forklift"}, Askpass: true},
	}}
	for _, s := range p.Steps {
		r.Stdout("", s.Argv...)
	}
	Execute(context.Background(), r, p, io.Discard, func(State) {})
	calls := r.Calls()
	if len(calls) != 2 {
		t.Fatalf("ran %+v", calls)
	}
	if slices.ContainsFunc(calls[0].Env, func(e string) bool { return strings.HasPrefix(e, "SUDO_ASKPASS=") }) {
		t.Errorf("formula step got askpass: %q", calls[0].Env)
	}
	for _, want := range []string{"SUDO_ASKPASS=" + AskpassProgram, AskpassEnv + "=Update 5 packages"} {
		if !slices.Contains(calls[1].Env, want) {
			t.Errorf("cask step env %q lacks %q", calls[1].Env, want)
		}
	}
}

// A step that may leave part of its work puts it back when it fails or
// is stopped itself; another step's Undo runs only once it got through.
func TestExecuteUndoAlways(t *testing.T) {
	create := Step{Argv: []string{"kind", "create", "cluster"}, UndoAlways: true, Undo: &Step{Argv: []string{"kind", "delete", "cluster"}}}
	other := Step{Argv: []string{"kind", "load"}, Undo: &Step{Argv: []string{"kind", "unload"}}}
	f := &run.Fake{}
	f.Set(run.Result{ExitCode: 1}, nil, "kind", "create", "cluster")
	f.Set(run.Result{}, nil, "kind", "delete", "cluster")
	got := Execute(t.Context(), f, Plan{Steps: []Step{create}}, io.Discard, func(State) {})
	if got.Phase != Failed || !slices.ContainsFunc(f.Calls(), func(c run.Cmd) bool { return slices.Equal(c.Args, []string{"kind", "delete", "cluster"}) }) {
		t.Fatalf("failed step not undone: %v %v", got.Phase, f.Calls())
	}
	if !slices.Equal(got.Undone, []string{"kind delete cluster"}) || len(got.Restored) != 0 {
		t.Fatalf("undone %q, restored %q", got.Undone, got.Restored)
	}
	f = &run.Fake{}
	f.Set(run.Result{ExitCode: 1}, nil, "kind", "load")
	Execute(t.Context(), f, Plan{Steps: []Step{other}}, io.Discard, func(State) {})
	if len(f.Calls()) != 1 {
		t.Fatalf("undid a step that did nothing: %v", f.Calls())
	}

	ctx, cancel := context.WithCancel(t.Context())
	b := &blocking{started: make(chan struct{})}
	go func() {
		<-b.started
		cancel()
	}()
	got = Execute(ctx, b, Plan{Steps: []Step{create}}, io.Discard, func(State) {})
	if got.Phase != Stopped || b.calls != 2 {
		t.Fatalf("stopped step not undone: %v, %d calls", got.Phase, b.calls)
	}
}

// An Undo runs again while what its step started is still coming.
func TestExecuteUndoAgain(t *testing.T) {
	left := 2
	create := Step{Argv: []string{"kind", "create", "cluster"}, UndoAlways: true, Undo: &Step{
		Argv:  []string{"kind", "delete", "cluster"},
		Again: func(context.Context) bool { left--; return left > 0 },
	}}
	f := &run.Fake{}
	f.Set(run.Result{ExitCode: 1}, nil, "kind", "create", "cluster")
	f.Set(run.Result{}, nil, "kind", "delete", "cluster")
	got := Execute(t.Context(), f, Plan{Steps: []Step{create}}, io.Discard, func(State) {})
	deletes := 0
	for _, c := range f.Calls() {
		if slices.Equal(c.Args, []string{"kind", "delete", "cluster"}) {
			deletes++
		}
	}
	if deletes != 2 || len(got.Undone) != 1 {
		t.Fatalf("deleted %d times, undone %q", deletes, got.Undone)
	}

	// What never goes is not reported undone.
	create.Undo.Again = func(context.Context) bool { return true }
	got = Execute(t.Context(), f, Plan{Steps: []Step{create}}, io.Discard, func(State) {})
	if len(got.Undone) != 0 {
		t.Fatalf("undone %q", got.Undone)
	}
}
