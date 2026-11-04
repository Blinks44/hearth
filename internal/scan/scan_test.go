package scan

import (
	"context"
	"errors"
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
