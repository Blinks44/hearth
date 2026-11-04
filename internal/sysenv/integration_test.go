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
