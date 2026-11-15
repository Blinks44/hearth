package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestHearthMCPPreset(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Hearth")
	if err := os.WriteFile(exe, nil, 0755); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.HearthExecutable = exe
	a.mcpChosen = "\x00hearth"
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir}, nil))
	}
	tt := openAI(t, a, mcpPage)
	hasTexts(t, tt, "hearth", "Available", "Agents can request")
	screenshot(t, tt, "mcp-hearth")
	if err := tt.Click("Connect"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started {
		t.Fatal("connect did not propose a plan")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude.json")); !os.IsNotExist(err) {
		t.Fatal("preview changed agent config")
	}
	tt = ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "mcp-hearth-plan")
}
