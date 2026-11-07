package ops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

// A step for the administrator runs elevated, its program found in the
// plan's PATH: the elevated command has a PATH of its own.
func TestExecuteAdmin(t *testing.T) {
	dir := t.TempDir()
	n := filepath.Join(dir, "n")
	if err := os.WriteFile(n, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtimeExe != "" {
		n += runtimeExe
		if err := os.WriteFile(n, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := Plan{Title: "Install Node 22.14.0", Env: []string{"PATH=" + dir},
		Steps: []Step{{Argv: []string{"n", "download", "22.14.0"}, Admin: true}}}
	want := elevate([]string{n, "download", "22.14.0"}, p.Title)
	f := &run.Fake{}
	f.Stdout("", want...)
	got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("phase %d, err %q; ran %v", got.Phase, got.Err, f.Calls())
	}
	if calls := f.Calls(); !slices.Equal(calls[0].Args, want) {
		t.Errorf("ran %q, want %q", calls[0].Args, want)
	}
}
