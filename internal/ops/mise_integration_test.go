//go:build integration

package ops

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// TestMisePlanReal sets mise up on this computer for real, as the app does,
// and checks a new terminal: run it only on a test machine.
func TestMisePlanReal(t *testing.T) {
	env, err := sysenv.Terminal(t.Context(), run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	terminal := func(ctx context.Context) (sysenv.Env, error) { return sysenv.Terminal(ctx, run.Exec{}) }
	p, err := MisePlan(env, terminal)
	if errors.Is(err, ErrMiseReady) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Steps {
		t.Logf("step: %v %+v %+v", s.Argv, s.Edit != nil, s.UserPath)
	}
	p.Env = env
	s := Execute(t.Context(), run.Exec{}, p, io.Discard, func(State) {})
	t.Logf("phase %v, verified %q, err %q, log %q", s.Phase, s.Verified, s.Err, s.Log)
	if s.Phase != Succeeded {
		t.Fatal("not set up")
	}
}
