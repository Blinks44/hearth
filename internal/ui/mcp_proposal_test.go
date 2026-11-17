package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/proposals"
)

func TestMCPProposalConfirmation(t *testing.T) {
	a := NewApp()
	a.Home = t.TempDir()
	a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: filepath.Join(a.Home, "missing"), Missing: true, Sources: []inventory.PathSource{{File: filepath.Join(a.Home, ".bashrc"), Line: 1}}}}}
	var deliver func(ops.Plan, error)
	if runtime.GOOS == "windows" {
		a.Snapshot.TerminalPath[0].Sources = []inventory.PathSource{{File: ops.UserPathName}}
	}
	a.PlanPath = func(_ []inventory.PathEntry, _ []string, done func(ops.Plan, error)) { deliver = done }
	found := issues.Find(a.Snapshot, nil, nil, time.Now())
	q := proposals.Request{IssueID: issues.ID(found[0])}
	path := filepath.Join(t.TempDir(), "rc")
	before := "export PATH=/missing:$PATH\n"
	after := "# export PATH=/missing:$PATH\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	runs := 0
	a.RunPlan = func(p ops.Plan, _ func(ops.State)) func() {
		runs++
		if err := os.WriteFile(path, []byte(after), 0600); err != nil {
			t.Fatal(err)
		}
		return func() {}
	}
	if err := a.RequestFix(q); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestFix(q); err == nil {
		t.Fatal("pending plan replaced")
	}
	deliver(ops.Plan{Title: "Fix test PATH", Steps: []ops.Step{{Edit: &ops.FileEdit{Path: path, Before: before, After: after}}}}, nil)
	if a.op == nil || a.op.started || runs != 0 || !strings.Contains(a.op.plan.Why, "Requested by an AI agent") {
		t.Fatal("request did not open an unconfirmed plan")
	}
	data, _ := os.ReadFile(path)
	if string(data) != before {
		t.Fatal("proposal wrote file")
	}
	if err := a.RequestFix(q); err == nil {
		t.Fatal("open plan replaced")
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatal("cancel executed operation")
	}
	if err := a.RequestFix(q); err != nil {
		t.Fatal(err)
	}
	deliver(ops.Plan{Title: "Fix test PATH", Steps: []ops.Step{{Edit: &ops.FileEdit{Path: path, Before: before, After: after}}}}, nil)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Run"); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("human confirmation ran %d times", runs)
	}
	data, _ = os.ReadFile(path)
	if string(data) != after {
		t.Fatal("confirmed operation did not run")
	}
}

func TestMCPProposalRejectsStaleAndUnsupported(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: "relative"}}}
	found := issues.Find(a.Snapshot, nil, nil, time.Now())
	q := proposals.Request{IssueID: issues.ID(found[0])}
	if err := a.RequestFix(q); err == nil {
		t.Fatal("unsupported fix accepted")
	}
	a.Snapshot = &inventory.Snapshot{}
	if err := a.RequestFix(q); err == nil {
		t.Fatal("stale issue accepted")
	}
	if a.op != nil {
		t.Fatal("rejected request proposed plan")
	}
}

func TestMCPProposalWaitsForProjectPreparation(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: "/missing", Missing: true, Sources: []inventory.PathSource{{File: "/home/u/.bashrc", Line: 1}}}}}
	a.PlanPath = func([]inventory.PathEntry, []string, func(ops.Plan, error)) {
		t.Fatal("plan requested during preparation")
	}
	a.preparing = &preparing{dir: "/project", open: true, cancel: func() {}}
	found := issues.Find(a.Snapshot, nil, nil, time.Now())
	if err := a.RequestFix(proposals.Request{IssueID: issues.ID(found[0])}); err == nil {
		t.Fatal("proposal accepted while a project plan was being prepared")
	}
}
