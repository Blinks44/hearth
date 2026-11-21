package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

func preparationApp(t *testing.T) (*App, *ui.Tester, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("22\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Managers: []inventory.Manager{{Name: "fnm"}}}}}
	a.Projects = []projects.Project{projects.Read(dir)}
	a.NodeVersions = &versions.Node{Releases: []versions.Release{{Version: "24.0.0"}, {Version: "22.14.0"}}}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(sysenv.Env{"PATH=/usr/bin"}, nil)
		done(p, err)
	}
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return a, tt, dir
}

func TestProjectPreparationPreviewAndConfirmation(t *testing.T) {
	a, tt, _ := preparationApp(t)
	runs := 0
	a.RunPlan = func(_ ops.Plan, _ func(ops.State)) func() { runs++; return func() {} }
	if err := tt.Click("Prepare environment…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "PROJECT REQUIREMENTS AND NEXT STEPS", "fnm install 22.14.0", "Copy command", "Open tool")
	if runs != 0 {
		t.Fatal("preview changed the system")
	}
	screenshot(t, tt, "prepare-plan")
	if err := tt.Click("Prepare environment"); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("confirmation ran %d times", runs)
	}
}

func TestProjectPreparationInstalledVersionHasManualPath(t *testing.T) {
	a, tt, dir := preparationApp(t)
	a.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.12.0", Source: inventory.Source{Manager: "fnm"}}}
	a.prepareProject(dir)
	tt.Frame()
	tt.Advance(time.Second)
	if len(a.op.plan.Steps) != 0 {
		t.Fatal("reinstalled existing version")
	}
	hasTexts(t, tt, "Check again", "Copy command", "Open tool")
	if err := tt.Click("Copy command"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Command copied. Run it in the project folder, then check again.")
	screenshot(t, tt, "prepare-installed")
}

func TestProjectPreparationContinueBuildsFreshPlan(t *testing.T) {
	for _, phase := range []ops.Phase{ops.Failed, ops.Stopped, ops.NotVerified, ops.Succeeded} {
		t.Run(string(rune('a'+phase)), func(t *testing.T) {
			a, tt, dir := preparationApp(t)
			var update func(ops.State)
			runs := 0
			a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() { runs++; update = u; return func() {} }
			a.prepareProject(dir)
			tt.Frame()
			a.startOperation()
			state := ops.State{Phase: phase, Steps: []ops.StepStatus{ops.Done}}
			if phase == ops.Failed {
				state.Steps[0] = ops.StepFailed
				state.ExitCode = 1
				state.Err = "download failed"
			}
			if phase == ops.Stopped {
				state.Steps[0] = ops.StepStopped
			}
			if phase == ops.NotVerified {
				state.Err = "detector unavailable"
			}
			update(state)
			tt.Frame()
			tt.Advance(time.Second)
			label := "Check again"
			if phase == ops.Failed || phase == ops.Stopped {
				label = "Continue preparation…"
			}
			hasTexts(t, tt, label)
			screenshot(t, tt, "prepare-result-"+string(rune('a'+phase)))
			a.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.14.0", Source: inventory.Source{Manager: "fnm"}}}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if runs != 1 || len(a.op.plan.Steps) != 0 {
				t.Fatalf("old plan repeated: runs %d, steps %+v", runs, a.op.plan.Steps)
			}
		})
	}
}

func TestProjectPreparationConflictAndReadErrorKeepActions(t *testing.T) {
	a, tt, dir := preparationApp(t)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":"<20"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.prepareProject(dir)
	tt.Frame()
	tt.Advance(time.Second)
	if len(a.op.plan.Steps) != 0 {
		t.Fatal("conflict installed a version")
	}
	hasTexts(t, tt, "Check again", "Open tool")
	screenshot(t, tt, "prepare-conflict")
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{broken`), 0600); err != nil {
		t.Fatal(err)
	}
	a.prepareProject(dir)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Check again", "Some project files could not be read. Resolve these errors before preparing the environment.")
}

func TestProjectPreparationCancelRejectsLateResult(t *testing.T) {
	a, tt, dir := preparationApp(t)
	var done func(ops.Plan, error)
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), complete func(ops.Plan, error)) {
		done = complete
	}
	a.prepareProject(dir)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	done(ops.Plan{Title: "stale"}, nil)
	if a.op != nil || a.preparing != nil {
		t.Fatal("cancelled request opened stale plan")
	}
}

func TestProjectPreparationFreshDetectionDoesNotMutatePublishedSnapshot(t *testing.T) {
	a, tt, dir := preparationApp(t)
	original := a.Snapshot.Tools[0]
	a.DetectTool = func(_ context.Context, tool string) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: tool, Err: "cannot check"}, nil
	}
	a.prepareProject(dir)
	tt.Frame()
	if a.Snapshot.Tools[0].Err != original.Err {
		t.Fatal("published snapshot mutated")
	}
	if len(a.op.plan.Steps) != 0 || !strings.Contains(a.op.project.Items[0].Text, "could not check") {
		t.Fatalf("detection error %+v", a.op.project)
	}
}

func TestProjectPreparationAppearanceAndFolderAction(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		dark          bool
	}{{"prepare-light", 1280, 820, false}, {"prepare-dark", 1280, 820, true}, {"prepare-narrow", 1000, 680, false}} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, dir := preparationApp(t)
			shell := &fakeShell{}
			a.Shell = shell
			a.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.12.0", Source: inventory.Source{Manager: "fnm"}}}
			a.prepareProject(dir)
			tt := ui.NewTesterWith(a.View, tc.width, tc.height, ui.TesterOptions{Dark: tc.dark})
			tt.Advance(time.Second)
			hasTexts(t, tt, "fnm use 22.12.0", "Copy command", "Open folder", "Check again")
			for _, label := range []string{"Copy command", "Open folder", "Check again"} {
				rect, ok := tt.Find(label)
				if !ok || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > float32(tc.width) || rect.Y+rect.H > float32(tc.height) {
					t.Fatalf("%s outside viewport: %+v", label, rect)
				}
			}
			screenshot(t, tt, tc.name)
			if err := tt.Click("Open folder"); err != nil {
				t.Fatal(err)
			}
			if len(shell.opened) != 1 || shell.opened[0] != dir {
				t.Fatalf("opened %+v", shell.opened)
			}
		})
	}
}

func TestProjectResultUsesPostRescanAndSeparatesReadiness(t *testing.T) {
	for _, tc := range []struct {
		lang          string
		width, height int
		readyTitle    string
		checkingTitle string
		selectedText  string
		dockerText    string
		terminalNote  string
		depsNote      string
		readmeBtn     string
		toProjectBtn  string
	}{
		{
			lang: "en", width: 1280, height: 820,
			readyTitle:    "Project tools are ready",
			checkingTitle: "Checking project requirements",
			selectedText:  "Node 22.14.0 is selected in this folder",
			dockerText:    "Local engine OrbStack answers",
			terminalNote:  "Open terminals may still use the old version",
			depsNote:      "Dependencies and app startup were not checked",
			readmeBtn:     "Open README.md",
			toProjectBtn:  "Go to project",
		},
		{
			lang: "ru", width: 1000, height: 680,
			readyTitle:    "Инструменты проекта готовы",
			checkingTitle: "Проверка требований проекта",
			selectedText:  "В папке проекта выбирается Node 22.14.0",
			dockerText:    "Локальный движок OrbStack отвечает",
			terminalNote:  "В открытом терминале версия может быть прежней",
			depsNote:      "Зависимости и запуск приложения не проверены",
			readmeBtn:     "Открыть README.md",
			toProjectBtn:  "К проекту",
		},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return tc.lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			a, _, dir := preparationApp(t)
			if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			a.Projects = []projects.Project{projects.Read(dir)}
			shell := &fakeShell{}
			a.Shell = shell
			a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
				{Tool: "node", Managers: []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".nvmrc"}}}},
				{Tool: "docker", Docker: &inventory.DockerState{
					Selected: "orbstack",
					Contexts: []inventory.DockerContext{{Name: "orbstack", Local: true, Status: "unavailable", Engine: "OrbStack"}},
					Engines:  []inventory.DockerEngine{{Name: "OrbStack", Path: "/Applications/OrbStack.app"}},
				}},
			}}
			scans := 0
			a.OnScan = func() { scans++ }
			var update func(ops.State)
			a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
			tt := ui.NewTester(a.View, tc.width, tc.height)
			a.prepareProject(dir)
			tt.Frame()
			tt.Advance(time.Second)
			a.startOperation()
			update(ops.State{Phase: ops.Succeeded, Steps: []ops.StepStatus{ops.Done, ops.Done}, Current: 1})
			tt.Frame()
			tt.Advance(time.Second)
			if scans != 1 {
				t.Fatalf("post-operation scan triggered %d times, want 1", scans)
			}
			if tt.HasText(tc.readyTitle) {
				t.Fatalf("claimed ready while post-operation scan is still running: %q", tt.Texts())
			}
			hasTexts(t, tt, tc.checkingTitle)
			screenshot(t, tt, "r36-checking-"+tc.lang)

			// Deliver fresh snapshot from R-33 rescan.
			a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
				{
					Tool:          "node",
					Managers:      []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".nvmrc"}}},
					Installations: []inventory.Installation{{Version: "22.14.0", OnPath: 1, Source: inventory.Source{Manager: "fnm"}}},
				},
				{Tool: "docker", Docker: &inventory.DockerState{
					Selected: "orbstack",
					Contexts: []inventory.DockerContext{{Name: "orbstack", Local: true, Status: "available", Engine: "OrbStack"}},
					Engines:  []inventory.DockerEngine{{Name: "OrbStack", Path: "/Applications/OrbStack.app"}},
				}},
			}}
			a.Scanning = false
			tt.Frame()
			tt.Advance(time.Second)
			hasTexts(t, tt, tc.readyTitle, tc.selectedText, tc.dockerText, tc.terminalNote, tc.depsNote, tc.readmeBtn, tc.toProjectBtn)
			screenshot(t, tt, "r36-ready-"+tc.lang)
			if err := tt.Click(tc.readmeBtn); err != nil {
				t.Fatal(err)
			}
			wantReadme := filepath.Join(dir, "README.md")
			if len(shell.opened) != 1 || shell.opened[0] != wantReadme {
				t.Fatalf("opened %+v, want %q", shell.opened, wantReadme)
			}
			if err := tt.Click(tc.toProjectBtn); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if a.op.open {
				t.Fatal("Go to project did not close the dialog")
			}
		})
	}
}

func TestProjectResultPostRescanNeedsActionAndCopyDoesNotTurnReady(t *testing.T) {
	a, tt, dir := preparationApp(t)
	a.Shell = &fakeShell{}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool:          "node",
			Managers:      []inventory.Manager{{Name: "fnm"}},
			Installations: []inventory.Installation{{Version: "24.0.0", OnPath: 1, Source: inventory.Source{Manager: "fnm"}}},
		},
	}}
	a.OnScan = func() {}
	var update func(ops.State)
	a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
	a.prepareProject(dir)
	tt.Frame()
	a.startOperation()
	update(ops.State{Phase: ops.Succeeded, Steps: []ops.StepStatus{ops.Done}})
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool:     "node",
			Managers: []inventory.Manager{{Name: "fnm"}},
			Installations: []inventory.Installation{
				{Version: "24.0.0", OnPath: 1, Source: inventory.Source{Manager: "fnm"}},
				{Version: "22.14.0", Source: inventory.Source{Manager: "fnm"}},
			},
		},
	}}
	a.Scanning = false
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Tools installed. Your action is needed", "Installed via fnm: Node 22.14.0", "Currently selected in this folder: Node 24.0.0", "Copy fnm use 22.14.0", "Check again")
	screenshot(t, tt, "r36-actions-en")
	if err := tt.Click("Copy fnm use 22.14.0"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.Clipboard() != "fnm use 22.14.0" {
		t.Fatalf("clipboard = %q", tt.Clipboard())
	}
	if tt.HasText("Project tools are ready") {
		t.Fatal("copying a command must not mark the project ready")
	}
}

func TestProjectResultPartialFailureStoppedUnverifiedAndSourceOpen(t *testing.T) {
	a, tt, dir := preparationApp(t)
	shell := &fakeShell{}
	a.Shell = shell
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"packageManager":"pnpm@9.15.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.Projects = []projects.Project{projects.Read(dir)}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "node", Managers: []inventory.Manager{{Name: "fnm"}}},
		{Tool: "pnpm", Managers: []inventory.Manager{{Name: "corepack"}}},
	}}
	var update func(ops.State)
	a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
	a.prepareProject(dir)
	tt.Frame()
	a.startOperation()

	// Partial failure: step 1 succeeded, step 2 failed.
	update(ops.State{
		Phase:    ops.Failed,
		Current:  1,
		Steps:    []ops.StepStatus{ops.Done, ops.StepFailed},
		ExitCode: 1,
		Log:      []string{"Error: registry timeout"},
	})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Preparation finished partially", "Completed installations stay", "Continue builds a fresh plan", "Copy log", "Continue preparation…")
	screenshot(t, tt, "r36-partial-en")

	// Stopped by user.
	a.prepareProject(dir)
	tt.Frame()
	a.startOperation()
	update(ops.State{
		Phase:   ops.Stopped,
		Current: 1,
		Steps:   []ops.StepStatus{ops.Done, ops.StepStopped},
	})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Preparation stopped by you", "Completed installations stay", "Continue preparation…")
	screenshot(t, tt, "r36-stopped-en")

	// Commands succeeded without a post-scan snapshot -> must report unverified, not ready.
	a.prepareProject(dir)
	tt.Frame()
	a.startOperation()
	update(ops.State{
		Phase:   ops.Succeeded,
		Current: 1,
		Steps:   []ops.StepStatus{ops.Done, ops.Done},
	})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Commands finished. Readiness not verified", "Successful commands do not confirm readiness", "Check again")
	if tt.HasText("Project tools are ready") {
		t.Fatal("unverified command completion claimed project ready")
	}
	screenshot(t, tt, "r36-unverified-en")

	// Conflict opens its source file directly from its row.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":"<20"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.prepareProject(dir)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Conflicting requirements", "Files are not rewritten automatically", "Open")
	screenshot(t, tt, "r36-conflict-en")
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if len(shell.opened) != 1 || filepath.Dir(shell.opened[0]) != dir {
		t.Fatalf("opened source = %+v", shell.opened)
	}
}
