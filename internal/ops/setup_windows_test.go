package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Rust on Windows brings Visual Studio Build Tools first, unless a C++
// compiler is there.
func TestSetupPlanRustMSVC(t *testing.T) {
	winget := []inventory.PackageManager{{ID: "winget", Name: "WinGet", Path: "winget"}}
	title := func(id string) string { return id }
	for _, have := range []bool{false, true} {
		s := Setup{GOOS: "windows", Tools: []string{"rust"}, Managers: winget, Have: map[string]bool{"cpp": have}, Title: title}
		p, _, err := SetupPlan(s, sysenv.Env{})
		if err != nil {
			t.Fatal(err)
		}
		tools := slices.ContainsFunc(p.Steps, func(st Step) bool { return slices.Contains(st.Argv, "Microsoft.VisualStudio.BuildTools") })
		if tools == have {
			t.Errorf("a compiler there %v, Build Tools in the plan %v: %+v", have, tools, p.Steps)
		}
		if !have && !slices.Contains(p.Steps[len(p.Steps)-1].Argv, "Rustlang.Rustup") {
			t.Errorf("rustup is not last: %+v", p.Steps)
		}
	}
}
