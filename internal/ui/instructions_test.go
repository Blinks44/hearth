package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestInstructionsPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Home = home
	for _, tt := range []struct{ name, state, a, c string }{
		{"shop-web", "different", "Run make check before finishing.", "Run npm test before finishing."},
		{"moduix", "linked", "Build", "@AGENTS.md"},
		{"hearth", "linked", "Build", "@AGENTS.md"},
		{"api", "linked", "Build", "@AGENTS.md"},
		{"legacy-admin", "claude", "", "Build"},
		{"ml-notebooks", "missing", "", ""},
	} {
		a.Projects = append(a.Projects, projects.Project{Dir: filepath.Join(home, "dev", tt.name), Instructions: projects.Instructions{Agents: projects.InstructionFile{Text: tt.a, Exists: tt.a != ""}, Claude: projects.InstructionFile{Text: tt.c, Exists: tt.c != ""}}})
	}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.router.Push(instructionsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "shop-web", "moduix", "AGENTS.md", "CLAUDE.md", "Different texts", "Run make check before finishing.")
	screenshot(t, tt, "instructions-en")
	if err := tt.Click("legacy-admin"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "CLAUDE.md", "Build")
	a.instructionsQuery = "ml-notebooks"
	tt.Frame()
	hasTexts(t, tt, "No instructions")
	a.instructionsQuery = ""
	a.instructionsChosen = ""
	a.Locale = "ru"
	i18n.SetLocaleFunc(func() string { return "ru" })
	defer i18n.SetLocaleFunc(nil)
	tt = ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "instructions-ru")
}

// Connecting only proposes a plan: opening its preview must not change files.
func TestInstructionsProposePlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("Build"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Projects = []projects.Project{projects.Read(dir)}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(nil, nil))
	}
	tt := openAI(t, a, instructionsPage)
	if err := tt.Click("Connect…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || len(a.op.plan.Steps) != 1 {
		t.Fatalf("no pending plan: %+v", a.op)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("preview changed the project")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "Build" {
		t.Fatal("preview changed the source")
	}
	hasTexts(t, tt, "@AGENTS.md")
	first := a.op
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Connect…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == first || !a.op.open || a.op.started {
		t.Fatal("cannot propose again after cancelling")
	}
}

func TestInstructionsEmptyOffersProject(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	chosen := false
	a.ChooseFolder = func() {
		chosen = true
		a.Offer(projects.Project{Dir: filepath.Join(home, "demo")}, nil)
	}
	tt := openAI(t, a, instructionsPage)
	hasTexts(t, tt, "No projects yet", "Add a project to compare its AGENTS.md and CLAUDE.md files.")
	if tt.HasText("Nothing matches") {
		t.Fatal("empty project list described as an unsuccessful search")
	}
	screenshot(t, tt, "instructions-empty")
	if err := tt.Click("Add folder"); err != nil {
		t.Fatal(err)
	}
	if !chosen {
		t.Fatal("add folder did not open the folder chooser")
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Add project", "demo", "Add 1 project")
	screenshot(t, tt, "instructions-add-project")
}

func TestInstructionsGlobalDiffCreateAndExtras(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.Home = home
	snap := aiSnapshot()
	snap.Agents[1].Instructions = &inventory.GlobalInstructions{
		State:  "shared",
		Shared: inventory.InstructionFile{Path: filepath.Join(home, ".agents", "AGENTS.md"), Exists: true},
		Native: inventory.InstructionFile{Path: filepath.Join(home, ".codex", "AGENTS.md")},
	}
	a.Snapshot = snap
	a.Projects = []projects.Project{
		{
			Dir: filepath.Join(home, "dev", "diff-app"),
			Instructions: projects.Instructions{
				Agents:  projects.InstructionFile{Text: "Run make check\n", Exists: true},
				Claude:  projects.InstructionFile{Text: "Run npm test\n", Exists: true},
				Gemini:  projects.InstructionFile{Text: "Use Gemini style\n", Exists: true},
				Copilot: projects.InstructionFile{Text: "Copilot tips\n", Exists: true},
				CursorRules: []projects.InstructionFile{
					{Path: ".cursor/rules/go.mdc", Text: "Use gofmt\n", Exists: true},
				},
			},
		},
		{
			Dir: dir,
			Instructions: projects.Instructions{
				Gemini: projects.InstructionFile{Text: "Only Gemini\n", Exists: true},
			},
		},
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(nil, nil))
	}
	tt := openAI(t, a, instructionsPage)
	hasTexts(t, tt, "GLOBAL", "Codex", "PROJECTS", "diff-app", "Differences", "AGENTS.md ↔ CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md", ".cursor/rules/go.mdc")
	// Switch to global Codex instructions
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Global instructions", "Connect…")
	// Switch to project without AGENTS.md (only GEMINI.md) and open Create AGENTS.md dialog
	if err := tt.Click(filepath.Base(dir)); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Other instruction files", "GEMINI.md", "Create AGENTS.md…")
	if err := tt.Click("Create AGENTS.md…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.projectInstructionsCreate == nil || !a.projectInstructionsCreate.open {
		t.Fatal("create dialog did not open")
	}
	if err := tt.Click("Review plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Action != "Create AGENTS.md" {
		t.Fatalf("unexpected create plan: %+v", a.op)
	}
}
