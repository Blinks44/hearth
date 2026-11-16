package ops

import (
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestWindowsDatabaseSetup(t *testing.T) {
	for _, id := range []string{"mysql"} {
		t.Run(id, func(t *testing.T) {
			p, parts, err := SetupPlan(Setup{GOOS: "windows", Tools: []string{id}, Have: map[string]bool{}, Managers: []inventory.PackageManager{{ID: "winget", Path: "winget"}}, Title: func(s string) string { return s }}, sysenv.Env{})
			if err != nil || len(parts) != 1 || len(p.Steps) < 3 {
				t.Fatalf("plan: %+v %v", p, err)
			}
			if !strings.Contains(strings.Join(p.Steps[0].Argv, " "), "no changes made") {
				t.Fatal("missing existing service guard")
			}
			all := ""
			for _, step := range p.Steps {
				all += strings.Join(step.Argv, " ") + strings.Join(step.Env, " ") + "\n"
			}
			if !strings.Contains(all, "Start-Process") {
				t.Fatal("missing vendor wizard")
			}
			if !strings.Contains(all, "Get-NetTCPConnection") || !strings.Contains(all, "greeting") {
				t.Fatal("missing running service verification")
			}
			if len(p.Notes) == 0 {
				t.Fatal("missing configuration explanation")
			}
		})
	}
}
