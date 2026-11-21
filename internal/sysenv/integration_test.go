//go:build integration

package sysenv

import (
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestTerminalReal(t *testing.T) {
	env, err := Terminal(t.Context(), run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	if env.Get("PATH") == "" {
		t.Fatalf("terminal PATH is empty: %q", env)
	}
	t.Logf("GUI PATH:      %s", GUI().Get("PATH"))
	t.Logf("terminal PATH: %s", env.Get("PATH"))
	for i, e := range Paths(env) {
		t.Logf("#%d %s sources=%+v missing=%v", i+1, e.Dir, e.Sources, e.Missing)
	}
}

func TestReadOSReal(t *testing.T) {
	o, err := ReadOS()
	if err != nil {
		t.Fatal(err)
	}
	if o.Name == "" || o.Version == "" || o.Arch == "" {
		t.Errorf("ReadOS = %+v, want every field set", o)
	}
	t.Logf("%+v", o)
}

func TestStartupReal(t *testing.T) {
	s := Startup(t.Context(), run.Exec{}, GUI(), GUI())
	if s == nil || s.Took <= 0 {
		t.Fatalf("Startup = %+v", s)
	}
	t.Logf("%s took %v, %v bare", s.Shell, s.Took, s.Bare)
	for _, f := range s.Files {
		t.Logf("file %s %v", f.File, f.Took)
	}
	for _, l := range s.Lines {
		t.Logf("line %s:%d %v %q", l.File, l.Line, l.Took, l.Text)
	}
}
