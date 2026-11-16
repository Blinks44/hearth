package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func globalInstructionApp(t *testing.T, shared, native string) (*App, string) {
	t.Helper()
	root := t.TempDir()
	for name, txt := range map[string]string{".agents/AGENTS.md": shared, ".codex/AGENTS.md": native} {
		if txt == "" {
			continue
		}
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(txt), 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := sysenv.Env{"HOME=" + root}
	in := ai.CodexInstructions(env)
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Agents: []inventory.Agent{{ID: "codex", Dir: filepath.Join(root, ".codex"), Instructions: &in}}}
	a.Home = root
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	return a, root
}

func TestGlobalInstructionsConnectOnlyProposes(t *testing.T) {
	a, root := globalInstructionApp(t, "Private rules", "")
	tt := openAI(t, a, agentPath("codex"))
	if err := tt.Click(locales.GlobalInstructionsConnect()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || len(a.op.plan.Steps) != 1 || a.op.plan.Steps[0].Instructions.Mode != "link" {
		t.Fatal("not a pending link plan")
	}
	if _, err := os.Lstat(filepath.Join(root, ".codex/AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("preview wrote a file")
	}
	hasTexts(t, tt, "Private rules")
	if err := tt.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op.started {
		t.Fatal("cancellation started operation")
	}
}

func TestGlobalInstructionsCreatePreview(t *testing.T) {
	a, root := globalInstructionApp(t, "", "")
	tt := openAI(t, a, agentPath("codex"))
	if err := tt.Click(locales.GlobalInstructionsCreate()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.globalInstructions == nil || !a.globalInstructions.open || !a.globalInstructions.create {
		t.Fatal("no editor")
	}
	a.globalInstructions.text = "My chosen rules\n"
	tt.Frame()
	if err := tt.Click(locales.GlobalInstructionsPlan()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || a.op.plan.Steps[0].Instructions.Text != "My chosen rules\n" {
		t.Fatal("editor text not used")
	}
	if _, err := os.Lstat(filepath.Join(root, ".codex/AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("preview created file")
	}
}

func TestGlobalInstructionsCompareOnly(t *testing.T) {
	a, root := globalInstructionApp(t, "Shared rules", "Codex rules")
	tt := openAI(t, a, agentPath("codex"))
	if err := tt.Click(locales.GlobalInstructionsCompare()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Shared rules", "Codex rules")
	if a.op != nil {
		t.Fatal("comparison proposed an operation")
	}
	data, _ := os.ReadFile(filepath.Join(root, ".codex/AGENTS.md"))
	if string(data) != "Codex rules" {
		t.Fatal("comparison changed native file")
	}
}

func TestGlobalInstructionsCopyNeedsNewConfirmation(t *testing.T) {
	a, root := globalInstructionApp(t, "Rules", "")
	p, err := ops.CodexInstructionPlan(sysenv.Env{"HOME=" + root}, "link", "")
	if err != nil {
		t.Fatal(err)
	}
	a.op = &operation{plan: p, open: true, started: true, state: ops.State{Phase: ops.Failed, InstructionCopyAvailable: true, Steps: []ops.StepStatus{ops.StepFailed}}}
	tt := openAI(t, a, agentPath("codex"))
	if err := tt.Click(locales.GlobalInstructionsCopy()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op.started || a.op.plan.Steps[0].Instructions.Mode != "copy" {
		t.Fatal("fallback did not propose a separate plan")
	}
	if _, err := os.Lstat(filepath.Join(root, ".codex/AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("fallback copied without confirmation")
	}
}

func TestGlobalInstructionsScreenshot(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Home = home
	for i := range a.Snapshot.Agents {
		ag := &a.Snapshot.Agents[i]
		if ag.ID == "codex" {
			ag.Files = []inventory.AgentFile{{Path: filepath.Join(home, ".codex/config.toml"), Kind: inventory.FileSettings}}
			ag.Instructions = &inventory.GlobalInstructions{State: "shared", Shared: inventory.InstructionFile{Path: filepath.Join(home, ".agents/AGENTS.md"), Exists: true}, Native: inventory.InstructionFile{Path: filepath.Join(home, ".codex/AGENTS.md")}, Override: inventory.InstructionFile{Path: filepath.Join(home, ".codex/AGENTS.override.md")}}
		}
	}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.router.Push(agentPath("codex"))
	i18n.SetLocaleFunc(func() string { return "ru" })
	defer i18n.SetLocaleFunc(nil)
	for _, w := range []int{1280, 1000} {
		tt := ui.NewTester(a.View, w, 940)
		tt.Advance(time.Second)
		hasTexts(t, tt, "Глобальные инструкции", "Подключить…")
		name := "global-instructions-ru"
		if w == 1000 {
			name += "-narrow"
		}
		screenshot(t, tt, name)
	}
}

func TestGlobalInstructionsLargeComparison(t *testing.T) {
	a, _ := globalInstructionApp(t, strings.Repeat("shared\n", 501), "Native")
	a.previewGlobalInstructions(false)
	if a.globalInstructions.err == "" {
		t.Fatal("unbounded text reached the UI diff")
	}
}
