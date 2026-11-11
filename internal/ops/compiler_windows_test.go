package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Build Tools come through WinGet with the C++ workload; without WinGet
// Hearth has no way.
func TestCompilerPlanWindows(t *testing.T) {
	p, ok := CompilerPlan([]inventory.PackageManager{{ID: "winget", Name: "WinGet", Path: "winget"}}, nil)
	if !ok || len(p.Steps) != 1 {
		t.Fatalf("%+v, %v", p.Steps, ok)
	}
	argv := p.Steps[0].Argv
	if i := slices.Index(argv, "--override"); !slices.Contains(argv, "Microsoft.VisualStudio.BuildTools") || i < 0 || argv[i+1] != buildToolsOverride {
		t.Errorf("argv %q", argv)
	}
	if _, ok := CompilerPlan([]inventory.PackageManager{{ID: "scoop"}}, nil); ok {
		t.Error("a plan without WinGet")
	}
}
