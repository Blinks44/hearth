package scan

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func found(path string) func(context.Context, run.Runner, sysenv.Env, sysenv.Env) (inventory.ToolState, error) {
	return func(context.Context, run.Runner, sysenv.Env, sysenv.Env) (inventory.ToolState, error) {
		return inventory.ToolState{Installations: []inventory.Installation{{Path: path}}}, nil
	}
}

func noEnv(context.Context, run.Runner) (term, gui sysenv.Env) { return nil, nil }

// A detector's error leaves the other tools checked, and keeps what it
// found before failing.
func TestRunKeepsOtherTools(t *testing.T) {
	ds := []Detector{
		{"node", found("/opt/homebrew/bin/node")},
		{"bun", func(context.Context, run.Runner, sysenv.Env, sysenv.Env) (inventory.ToolState, error) {
			return inventory.ToolState{Installations: []inventory.Installation{{Path: "/x/bun"}}}, errors.New("bun --version: timed out")
		}},
		{"python", found("/usr/bin/python3")},
	}
	s := Run(context.Background(), &run.Fake{}, nil, nil, ds)
	if len(s.Tools) != 3 || s.Taken.IsZero() {
		t.Fatalf("snapshot %+v", s)
	}
	for i, want := range []string{"node", "bun", "python"} {
		if got := s.Tools[i].Tool; got != want {
			t.Errorf("tool %d = %q, want %q", i, got, want)
		}
	}
	if s.Tools[0].Err != "" || s.Tools[2].Err != "" || len(s.Tools[2].Installations) != 1 {
		t.Errorf("tools beside the failing one: %+v", s.Tools)
	}
	if bun := s.Tools[1]; bun.Err != "bun --version: timed out" || len(bun.Installations) != 1 {
		t.Errorf("failing tool: %+v", bun)
	}
}

// The snapshot lists both PATHs as written, marking the directories that
// are not there.
func TestRunListsPath(t *testing.T) {
	dir := t.TempDir()
	gone := filepath.Join(dir, "gone")
	term := sysenv.Env{"PATH=" + dir + string(filepath.ListSeparator) + gone + string(filepath.ListSeparator) + "."}
	gui := sysenv.Env{"PATH=" + dir}
	s := Run(context.Background(), &run.Fake{}, term, gui, nil)
	want := []inventory.PathEntry{{Dir: dir}, {Dir: gone, Missing: true}, {Dir: "."}}
	// Where entries come from depends on the machine: a runner's registry has ".".
	for i := range s.TerminalPath {
		s.TerminalPath[i].Sources = nil
	}
	if !reflect.DeepEqual(s.TerminalPath, want) {
		t.Errorf("terminal PATH %+v, want %+v", s.TerminalPath, want)
	}
	if want := []inventory.PathEntry{{Dir: dir}}; !reflect.DeepEqual(s.GUIPath, want) {
		t.Errorf("GUI PATH %+v, want %+v", s.GUIPath, want)
	}
}

// A second scan cancels the first, whose snapshot never arrives.
func TestStartDropsOlderScan(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	first := true
	slow := func(ctx context.Context, _ run.Runner, _, _ sysenv.Env) (inventory.ToolState, error) {
		if !first {
			return inventory.ToolState{}, nil
		}
		first = false
		close(started)
		<-ctx.Done()
		close(cancelled)
		return inventory.ToolState{}, ctx.Err()
	}
	s := &Scanner{Runner: &run.Fake{}, Detectors: []Detector{{"node", slow}}, Env: noEnv}
	snaps := make(chan string, 2)
	s.Start(func(inventory.Snapshot) { snaps <- "first" })
	<-started
	s.Start(func(inventory.Snapshot) { snaps <- "second" })
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the first scan was not cancelled")
	}
	if got := <-snaps; got != "second" {
		t.Errorf("got the %s scan's snapshot", got)
	}
	select {
	case got := <-snaps:
		t.Errorf("got the %s scan's snapshot too", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// A refresh runs only the tools asked for, and Replace puts their states
// in a copy of the snapshot.
func TestRefresh(t *testing.T) {
	s := &Scanner{Runner: &run.Fake{}, Env: noEnv, Detectors: []Detector{
		{"node", func(context.Context, run.Runner, sysenv.Env, sysenv.Env) (inventory.ToolState, error) {
			t.Error("node ran")
			return inventory.ToolState{}, nil
		}},
		{"docker", found("/new/docker")},
	}}
	got := make(chan []inventory.ToolState, 1)
	s.Refresh([]string{"docker"}, func(states []inventory.ToolState) { got <- states })
	states := <-got
	if len(states) != 1 || states[0].Tool != "docker" {
		t.Fatalf("%+v", states)
	}
	old := inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node"}, {Tool: "docker"}}}
	snap := Replace(old, states)
	if snap.Tools[1].Installations[0].Path != "/new/docker" || old.Tools[1].Installations != nil || snap.Tools[0].Tool != "node" {
		t.Fatalf("%+v %+v", snap, old)
	}
}

// An operation's check reads the terminal anew: the operation may have
// hooked mise into the shell, which the last scan's terminal lacks.
func TestDetectReadsNewTerminal(t *testing.T) {
	path := "/usr/bin"
	s := &Scanner{Runner: &run.Fake{}, Env: func(context.Context, run.Runner) (term, gui sysenv.Env) {
		return sysenv.Env{"PATH=" + path}, nil
	}, Detectors: []Detector{{"pnpm", func(_ context.Context, _ run.Runner, term, _ sysenv.Env) (inventory.ToolState, error) {
		return inventory.ToolState{Vars: map[string]string{"PATH": term.Get("PATH")}}, nil
	}}}}
	done := make(chan inventory.Snapshot, 1)
	s.Start(func(snap inventory.Snapshot) { done <- snap })
	<-done
	path = "/home/ann/.local/share/mise/shims:/usr/bin"
	st, err := s.Detect(context.Background(), "pnpm")
	if err != nil || st.Vars["PATH"] != path {
		t.Errorf("checked in %q, %v", st.Vars["PATH"], err)
	}
}
