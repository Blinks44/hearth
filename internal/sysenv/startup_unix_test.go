//go:build !windows

package sysenv

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

func TestStartupMeasures(t *testing.T) {
	trace, err := os.ReadFile(filepath.Join("testdata", "startup-zsh.txt"))
	if err != nil {
		t.Fatal(err)
	}
	zsh := []string{"/bin/zsh", "-i", "-l", "-c", "exit"}
	traced := []string{"/bin/zsh", "-o", "promptsubst", "-x", "-i", "-l", "-c", "exit"}
	gui := Env{"SHELL=/bin/zsh", "HOME=/home/u"}

	t.Run("fast start is not traced", func(t *testing.T) {
		r := &run.Fake{}
		r.Set(run.Result{Duration: 120 * time.Millisecond}, nil, zsh...)
		s := startup(t.Context(), r, gui, "M")
		if s == nil || s.Shell != "zsh" || s.Took != 120*time.Millisecond || s.Lines != nil {
			t.Fatalf("startup = %+v", s)
		}
		if n := len(r.Calls()); n != 1 {
			t.Errorf("%d commands, want 1", n)
		}
	})
	t.Run("slow start is traced", func(t *testing.T) {
		r := &run.Fake{}
		r.Set(run.Result{Duration: 600 * time.Millisecond}, nil, zsh...)
		r.Set(run.Result{Stderr: trace}, nil, traced...)
		s := startup(t.Context(), r, gui, "M")
		if s == nil || len(s.Lines) < 3 {
			t.Fatalf("startup = %+v", s)
		}
		if l := s.Lines[0]; l.File != "/home/u/.zshrc" || l.Line != 2 {
			t.Errorf("slowest line %s:%d, want ~/.zshrc:2", l.File, l.Line)
		}
		var sum time.Duration
		for _, f := range s.Files {
			sum += f.Took
		}
		if d := sum - s.Took; d < -time.Millisecond || d > time.Millisecond {
			t.Errorf("files sum to %v, start took %v", sum, s.Took)
		}
		if ps4 := r.Calls()[1].Env; Env(ps4).Get("PS4") == "" {
			t.Error("the trace has no PS4")
		}
	})
	t.Run("shell that does not start", func(t *testing.T) {
		if s := startup(t.Context(), &run.Fake{}, gui, "M"); s != nil {
			t.Errorf("startup = %+v, want nil", s)
		}
	})
}
