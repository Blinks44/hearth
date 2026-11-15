package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

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
}
