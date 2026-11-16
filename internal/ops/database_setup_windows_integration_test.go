//go:build integration && windows

package ops

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Run only in a disposable Windows VM with the chosen server configured.
func TestDatabaseServiceProbe(t *testing.T) {
	id := os.Getenv("HEARTH_TEST_DB")
	if id != "mysql" {
		t.Skip("set HEARTH_TEST_DB to the installed test server")
	}
	want := "$false"
	wrong := "$true"
	for _, tt := range []struct {
		name, kind string
		pass       bool
	}{{"matching server", want, true}, {"wrong server", wrong, false}} {
		t.Run(tt.name, func(t *testing.T) {
			step := databaseScript(strings.ReplaceAll(databaseServiceCheck, "EXPECTED_MARIA", tt.kind))
			res, err := (run.Exec{}).Run(context.Background(), run.Cmd{Args: step.Argv, Timeout: step.Timeout})
			if (err == nil && res.ExitCode == 0) != tt.pass {
				t.Fatalf("exit %d, err %v, output %s %s", res.ExitCode, err, res.Stdout, res.Stderr)
			}
		})
	}
}

// An existing configured service must stop a new installation before WinGet.
func TestDatabaseServiceGuard(t *testing.T) {
	if os.Getenv("HEARTH_TEST_DB") == "" {
		t.Skip("requires a configured test server")
	}
	steps, _ := windowsDatabaseSetup("mysql", []Step{{Argv: []string{"winget"}}}, nil)
	res, err := (run.Exec{}).Run(context.Background(), run.Cmd{Args: steps[0].Argv, Timeout: steps[0].Timeout})
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("guard exit %d, err %v", res.ExitCode, err)
	}
}

// Exercise the final probe through Execute without installing packages again
// or touching existing server data.
func TestConfiguredDatabasePlan(t *testing.T) {
	id := os.Getenv("HEARTH_TEST_DB")
	if id != "mysql" {
		t.Skip("requires a configured test server")
	}
	p, _, err := SetupPlan(Setup{GOOS: "windows", Tools: []string{id}, Managers: []inventory.PackageManager{{ID: "winget", Path: "winget"}}, Title: func(s string) string { return s }}, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	p.Steps = p.Steps[len(p.Steps)-1:]
	p.Parts = nil
	state := Execute(context.Background(), run.Exec{}, p, os.Stdout, func(State) {})
	if state.Phase != Succeeded {
		t.Fatalf("probe plan failed: %+v", state)
	}
}
