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
	a.previewGlobalInstructions("codex", false)
	if a.globalInstructions.err == "" {
		t.Fatal("unbounded text reached the UI diff")
	}
}

func TestGlobalInstructionsAllAgents(t *testing.T) {
	root := t.TempDir()
	write := func(rel, txt string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(txt), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".agents/AGENTS.md", "Shared rules\n")
	write(".claude/CLAUDE.md", "@~/.agents/AGENTS.md\n")

	env := sysenv.Env{"HOME=" + root}
	agyIn, ok := ai.AgentInstructions("antigravity", env)
	if !ok || agyIn.State != "shared" {
		t.Fatalf("antigravity instructions: %+v (%v)", agyIn, ok)
	}
	claudeIn, ok := ai.AgentInstructions("claude", env)
	if !ok || claudeIn.State != "linked" {
		t.Fatalf("claude instructions: %+v (%v)", claudeIn, ok)
	}

	a := NewApp()
	a.Home = root
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{
				ID:            "claude",
				Dir:           filepath.Join(root, ".claude"),
				Installations: []inventory.Installation{{Tool: "claude", Version: "2.1.311", Path: "/opt/homebrew/bin/claude", OnPath: 1}},
				Files:         []inventory.AgentFile{{Path: filepath.Join(root, ".claude/CLAUDE.md"), Kind: inventory.FileInstructions}},
				Instructions:  &claudeIn,
			},
			{
				ID:            "antigravity",
				Dir:           filepath.Join(root, ".gemini/antigravity-cli"),
				Installations: []inventory.Installation{{Tool: "antigravity", Version: "1.5.13", Path: "/opt/homebrew/bin/agy", OnPath: 1}},
				Instructions:  &agyIn,
			},
		},
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}

	// Agents table surfaces connectable shared instructions for Antigravity.
	tt := openAI(t, a, "/ai/agents")
	hasTexts(t, tt, "Antigravity CLI", locales.GlobalInstructionsChipShared(), "CLAUDE.md")

	// Opening Antigravity page shows the Global instructions card and proposes a link plan.
	if err := tt.Click("Antigravity CLI"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.GlobalInstructionsTitle(), locales.GlobalInstructionsConnect())
	if err := tt.Click(locales.GlobalInstructionsConnect()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || len(a.op.plan.Steps) != 1 {
		t.Fatalf("expected pending link plan: %+v", a.op)
	}
	in := a.op.plan.Steps[0].Instructions
	if in == nil || in.Agent != "antigravity" || in.Mode != "link" || in.Observed.Native.Path != filepath.Join(root, ".gemini/config/AGENTS.md") {
		t.Fatalf("unexpected antigravity plan: %+v", in)
	}

	// Copy fallback after a refused symlink targets Antigravity, not Codex.
	a.op = &operation{
		plan:    a.op.plan,
		open:    true,
		started: true,
		state:   ops.State{Phase: ops.Failed, InstructionCopyAvailable: true, Steps: []ops.StepStatus{ops.StepFailed}},
	}
	tt.Frame()
	if err := tt.Click(locales.GlobalInstructionsCopy()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op.started || a.op.plan.Steps[0].Instructions.Agent != "antigravity" || a.op.plan.Steps[0].Instructions.Mode != "copy" {
		t.Fatalf("copy fallback lost target agent: %+v", a.op.plan.Steps[0].Instructions)
	}

	// Claude page recognizes @~/.agents/AGENTS.md import as linked and hides Connect/Compare.
	a.op = nil
	a.router.Push(agentPath("claude"))
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.GlobalInstructionsTitle(), locales.GlobalInstructionsChipLinked(), locales.GlobalInstructionsImportedFile())
	if tt.HasText(locales.GlobalInstructionsConnect()) || tt.HasText(locales.GlobalInstructionsCompare()) {
		t.Fatal("linked Claude import should not offer Connect or Compare")
	}

	if os.Getenv("HEARTH_SCREENSHOTS") != "" {
		i18n.SetLocaleFunc(func() string { return "ru" })
		defer i18n.SetLocaleFunc(nil)
		a.Home = root
		a.Shell = &fakeShell{}
		for _, page := range []struct{ name, route string }{
			{"agents-table-ru", "/ai/agents"},
			{"antigravity-instructions-ru", agentPath("antigravity")},
			{"claude-instructions-ru", agentPath("claude")},
		} {
			for _, size := range []struct {
				suffix string
				w, h   int
			}{{"", 1280, 820}, {"-narrow", 1000, 680}} {
				a.router.Push(page.route)
				st := ui.NewTester(a.View, size.w, size.h)
				st.Advance(time.Second)
				screenshot(t, st, page.name+size.suffix)
			}
		}
	}
}
