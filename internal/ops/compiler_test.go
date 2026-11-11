package ops

import (
	"context"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestCompilerFound(t *testing.T) {
	cc := inventory.Installation{Path: "/usr/bin/cc", OnPath: 1}
	stub := inventory.Installation{Path: "/usr/bin/cc", OnPath: 1, Stub: true}
	tests := []struct {
		name string
		st   inventory.ToolState
		want bool
	}{
		{"nothing", inventory.ToolState{Cpp: &inventory.CppState{}}, false},
		{"macOS's stand-in", inventory.ToolState{Installations: []inventory.Installation{stub}, Cpp: &inventory.CppState{}}, false},
		{"cc on PATH", inventory.ToolState{Installations: []inventory.Installation{cc}, Cpp: &inventory.CppState{}}, true},
		{"the toolset alone", inventory.ToolState{Cpp: &inventory.CppState{Base: &inventory.CppBase{ID: "msvc"}}}, true},
	}
	for _, tt := range tests {
		if got := CompilerFound(tt.st); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The check says what it found once the compiler is there, and that it
// is not when the waiting is over.
func TestCompilerCheck(t *testing.T) {
	defer func(d time.Duration) { compilerPoll = d }(compilerPoll)
	compilerPoll = time.Millisecond
	found := inventory.ToolState{Cpp: &inventory.CppState{Base: &inventory.CppBase{Name: "Command Line Tools", Version: "16.2"}}}
	got, err := compilerWaitFor(context.Background(), func(context.Context) (inventory.ToolState, error) { return found, nil })
	if err != nil || got != "Found Command Line Tools 16.2" {
		t.Errorf("found: %q, %v", got, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	missing := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Cpp: &inventory.CppState{}}, nil
	}
	if _, err := compilerWaitFor(ctx, missing); err == nil || err.Error() != "No C compiler found yet" {
		t.Errorf("missing: %v", err)
	}
}
