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
	"github.com/Blinks44/hearth/internal/sysenv"
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

func TestMCPExtraProposalsRequireConfirmation(t *testing.T) {
	t.Run("stop_process rejects non-owned and proposes owned without running", func(t *testing.T) {
		a := NewApp()
		runs := 0
		a.RunPlan = func(ops.Plan, func(ops.State)) func() {
			runs++
			return func() {}
		}
		a.Snapshot = &inventory.Snapshot{
			Tools: []inventory.ToolState{{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Listeners: []inventory.DatabaseListener{
						{Port: "3000", PID: "111", Process: "node", Path: filepath.Join(t.TempDir(), "node"), Own: false},
						{Port: "3001", PID: "222", Process: "node", Path: filepath.Join(t.TempDir(), "node"), Own: true},
					},
				},
			}},
		}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindStopProcess, Port: "3000", PID: "111"}); err == nil {
			t.Fatal("non-owned process accepted")
		}
		if a.op != nil {
			t.Fatal("rejected request opened plan")
		}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindStopProcess, Port: "3001", PID: "222"}); err != nil {
			t.Fatalf("owned listener refused: %v", err)
		}
		if a.op == nil || a.op.started || runs != 0 || !strings.Contains(a.op.plan.Why, "Requested by an AI agent") {
			t.Fatalf("unexpected plan state: op=%+v runs=%d", a.op, runs)
		}
	})

	t.Run("tool_install version proposes unconfirmed plan", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = &inventory.Snapshot{
			Tools: []inventory.ToolState{{
				Tool:     "node",
				Managers: []inventory.Manager{{Name: "fnm"}},
			}},
		}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindToolInstall, Tool: "node", Version: "22.14.0"}); err != nil {
			t.Fatalf("tool_install refused: %v", err)
		}
		if a.op == nil || a.op.started || !strings.Contains(a.op.plan.Why, "Requested by an AI agent") {
			t.Fatalf("unexpected tool_install plan: %+v", a.op)
		}
	})

	t.Run("project_prepare rejects unregistered project", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = &inventory.Snapshot{}
		a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindProjectPrepare, Project: t.TempDir()}); err == nil {
			t.Fatal("unregistered project accepted")
		}
	})

	t.Run("cleanup refuses when sizes missing", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = &inventory.Snapshot{}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindCleanup}); err == nil {
			t.Fatal("cleanup accepted without Sizes")
		}
	})

	t.Run("tool_install rejects non-version-managed or arbitrary backends", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = &inventory.Snapshot{
			Tools: []inventory.ToolState{{Tool: "node", Managers: []inventory.Manager{{Name: "mise"}}}},
		}
		for _, req := range []proposals.Request{
			{Kind: proposals.KindToolInstall, Tool: "ubi:attacker/repo", Version: "1.0.0"},
			{Kind: proposals.KindToolInstall, Tool: "git", Version: "2.48.0"},
			{Kind: proposals.KindToolInstall, Tool: "node", Version: "--flag"},
		} {
			if err := a.RequestFix(req); err == nil {
				t.Fatalf("tool_install accepted invalid request: %+v", req)
			}
		}
	})

	t.Run("skill_action disambiguates valid and broken skill with same name", func(t *testing.T) {
		h := t.TempDir()
		brokenPath := filepath.Join(h, ".codex", "skills", "deploy")
		validPath := filepath.Join(h, ".claude", "skills", "deploy")
		if err := os.MkdirAll(filepath.Dir(brokenPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(validPath, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(h, "missing", "deploy"), brokenPath); err != nil {
			t.Fatal(err)
		}
		a := NewApp()
		a.Home = h
		a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
			done(build(sysenv.Env{"HOME=" + h}, nil))
		}
		a.Snapshot = &inventory.Snapshot{
			Agents: []inventory.Agent{
				{ID: "claude", Dir: filepath.Join(h, ".claude"), Installations: []inventory.Installation{{Tool: "claude", Version: "1.0.0", Path: "/usr/bin/claude"}}},
				{ID: "codex", Dir: filepath.Join(h, ".codex"), Installations: []inventory.Installation{{Tool: "codex", Version: "1.0.0", Path: "/usr/bin/codex"}}},
			},
			Skills: []inventory.Skill{
				{Name: "deploy", Dir: filepath.Join(h, "missing", "deploy"), Broken: true, Paths: []inventory.SkillPath{{Path: brokenPath, Link: true, Agents: []string{"codex"}}}},
				{Name: "deploy", Dir: validPath, Broken: false, Paths: []inventory.SkillPath{{Path: validPath, Agents: []string{"claude"}}}},
			},
		}
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindSkillAction, Action: "remove_broken", Skill: "deploy"}); err != nil {
			t.Fatalf("remove_broken failed: %v", err)
		}
		if a.op == nil {
			t.Fatalf("expected remove_broken plan, notice=%q", a.notice)
		}
		a.op = nil
		if err := a.RequestFix(proposals.Request{Kind: proposals.KindSkillAction, Action: "link_all", Skill: "deploy"}); err != nil {
			t.Fatalf("link_all failed despite valid skill present: %v", err)
		}
		if a.op == nil {
			t.Fatalf("expected link_all plan, notice=%q", a.notice)
		}
	})
}
