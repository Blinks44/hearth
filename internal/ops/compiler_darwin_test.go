package ops

import (
	"slices"
	"testing"
)

// macOS asks its own installer for the Command Line Tools, with no
// package manager.
func TestCompilerPlanMac(t *testing.T) {
	p, ok := CompilerPlan(nil, nil)
	if !ok || len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, []string{"xcode-select", "--install"}) || p.Steps[0].Admin {
		t.Fatalf("%+v, %v", p.Steps, ok)
	}
}
