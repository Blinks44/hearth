package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Apps that get another PATH than the terminal can be given its PATH;
// Hearth's launch agent, once there, can be taken back.
func TestAppsPath(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.Home = dir
	a.Snapshot = &inventory.Snapshot{
		TerminalPath: []inventory.PathEntry{{Dir: "/opt/homebrew/bin"}, {Dir: "/usr/bin"}},
		GUIPath:      []inventory.PathEntry{{Dir: "/usr/bin"}},
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "PATH=/opt/homebrew/bin:/usr/bin"}, nil))
	}
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "path-apps-give")
	if tt.HasText("Stop giving it…") {
		t.Fatal("stop offered without the agent")
	}
	if err := tt.Click("Give apps this PATH…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Give apps the terminal's PATH" {
		t.Fatalf("plan %+v", a.op)
	}
	a.op = nil
	a.Snapshot.GUIPath = a.Snapshot.TerminalPath
	a.Snapshot.AppsAgent = dir + "/Library/LaunchAgents/dev.hearth.path.plist"
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "path-apps")
	if tt.HasText("Give apps this PATH…") || !tt.HasText("Apps get the terminal's PATH from Hearth's launch agent") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Stop giving it…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Stop giving apps the terminal's PATH" {
		t.Fatalf("plan %+v", a.op)
	}
}
