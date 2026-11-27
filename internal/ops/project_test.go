package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func projectFixture(t *testing.T) (ProjectInput, sysenv.Env) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("22\n"), 0600); err != nil {
		t.Fatal(err)
	}
	in := ProjectInput{Project: projects.Read(dir), Title: func(s string) string { return s }, Releases: map[string][]string{"node": {"24.0.0", "22.14.0"}},
		Snapshot: &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Managers: []inventory.Manager{{Name: "fnm"}}}}}}
	return in, sysenv.Env{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
}

func TestProjectPreparationUsesExistingManagerAndNoGlobalDefault(t *testing.T) {
	in, env := projectFixture(t)
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 1 || !reflect.DeepEqual(got.Plan.Steps[0].Argv, []string{"fnm", "install", "22.14.0"}) {
		t.Fatalf("steps %+v", got.Plan.Steps)
	}
	if len(got.Items) != 1 || got.Items[0].Ready || !reflect.DeepEqual(got.Items[0].Command, []string{"fnm", "use", "22.14.0"}) {
		t.Fatalf("items %+v", got.Items)
	}
	if err := got.Plan.Before(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in.Project.Dir, ".nvmrc"), []byte("24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := &run.Fake{}
	state := Execute(context.Background(), fake, got.Plan, io.Discard, func(State) {})
	if state.Phase != NotVerified || len(fake.Calls()) != 0 || !strings.Contains(state.Err, "changed") {
		t.Fatalf("stale plan state %+v, calls %+v", state, fake.Calls())
	}
}

func TestProjectPreparationInstalledNeedsChoiceNotReinstallation(t *testing.T) {
	in, env := projectFixture(t)
	in.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.12.0", Source: inventory.Source{Manager: "fnm"}}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 0 || got.Items[0].Ready || got.Items[0].Version != "22.12.0" || len(got.Items[0].Command) == 0 {
		t.Fatalf("preparation %+v", got)
	}
}

func TestProjectPreparationConflictDoesNotInstallAnyVersion(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = append(in.Project.Requirements, projects.Requirement{Tool: "node", Want: "<20", File: "package.json"})
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 0 || got.Items[0].Ready || len(got.Items[0].Files) != 2 {
		t.Fatalf("conflict preparation %+v", got)
	}
}

func TestProjectPreparationFailedThenContinuedSkipsCompletedInstall(t *testing.T) {
	in, env := projectFixture(t)
	if err := os.WriteFile(filepath.Join(in.Project.Dir, "package.json"), []byte(`{"packageManager":"pnpm@9.15.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	in.Project = projects.Read(in.Project.Dir)
	in.Snapshot.Tools = append(in.Snapshot.Tools, inventory.ToolState{Tool: "pnpm", Managers: []inventory.Manager{{Name: "corepack"}}})
	in.Detect = func(_ context.Context, tool string) (inventory.ToolState, error) {
		return inventory.ToolState{}, errors.New("not checked")
	}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	fake := &run.Fake{}
	fake.Stdout("installed", "fnm", "install", "22.14.0")
	fake.Set(run.Result{ExitCode: 1}, nil, "corepack", "prepare", "pnpm@9.15.0")
	state := Execute(context.Background(), fake, got.Plan, io.Discard, func(State) {})
	if state.Phase != Failed || state.Steps[0] != Done {
		t.Fatalf("partial failure %+v", state)
	}
	in.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.14.0", Source: inventory.Source{Manager: "fnm"}}}
	next, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plan.Steps) != 1 || next.Plan.Steps[0].Argv[0] != "corepack" {
		t.Fatalf("completed Node reinstalled: %+v", next.Plan.Steps)
	}
}

func TestProjectPreparationDockerNeverChangesRemoteContext(t *testing.T) {
	for _, tt := range []struct {
		name   string
		local  bool
		status string
		want   int
	}{
		{"local stopped", true, "unavailable", 1}, {"local running", true, "available", 0}, {"remote stopped", false, "unavailable", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in, env := projectFixture(t)
			in.Project.Requirements = []projects.Requirement{{Tool: "docker", File: "compose.yaml"}}
			in.Snapshot.Tools = []inventory.ToolState{{Tool: "docker", Docker: &inventory.DockerState{Selected: "work", Contexts: []inventory.DockerContext{{Name: "work", Local: tt.local, Status: tt.status, Engine: "Docker Desktop"}}, Engines: []inventory.DockerEngine{{Name: "Docker Desktop", Path: "/Applications/Docker.app"}}}}}
			got, err := PrepareProject(in, env)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Plan.Steps) != tt.want {
				t.Fatalf("steps %+v", got.Plan.Steps)
			}
			if !tt.local && got.Items[0].Ready {
				t.Fatal("remote engine called ready")
			}
		})
	}
}

func TestProjectPreparationCorepackKeepsGlobalSelection(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{{Tool: "pnpm", Want: "9.15.0", File: "package.json", Key: "packageManager"}}
	in.Snapshot.Tools = []inventory.ToolState{{Tool: "pnpm", Managers: []inventory.Manager{{Name: "corepack"}}}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 1 || !reflect.DeepEqual(got.Plan.Steps[0].Argv, []string{"corepack", "prepare", "pnpm@9.15.0"}) {
		t.Fatalf("changed Corepack default: %+v", got.Plan.Steps)
	}
	if !got.Plan.Isolated || !slices.Contains(got.Plan.Steps[0].Env, "COREPACK_DEFAULT_TO_LATEST=0") {
		t.Fatal("Corepack not isolated from project/default changes")
	}
}

func TestProjectPreparationUvDoesNotClaimPython3Selection(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{{Tool: "python", Want: "3.12", File: ".python-version"}}
	in.Snapshot.Tools = []inventory.ToolState{{Tool: "python", Managers: []inventory.Manager{{Name: "uv"}}, Installations: []inventory.Installation{{Version: "3.12.12", Source: inventory.Source{Manager: "uv"}}}}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 0 || !strings.Contains(got.Items[0].Text, "does not change python3") {
		t.Fatalf("Python selection incorrectly claimed: %+v", got)
	}
}

func TestProjectCommandsRunOutsideProjectAndRemoveTemporaryDirectory(t *testing.T) {
	in, env := projectFixture(t)
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	fake := &run.Fake{}
	fake.Stdout("done", "fnm", "install", "22.14.0")
	state := Execute(context.Background(), fake, got.Plan, io.Discard, func(State) {})
	if state.Phase != Succeeded {
		t.Fatalf("execution %+v", state)
	}
	calls := fake.Calls()
	if len(calls) != 1 || calls[0].Dir == "" || calls[0].Dir == in.Project.Dir {
		t.Fatalf("command directory %+v", calls)
	}
	if _, err := os.Stat(calls[0].Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}

func TestProjectMiseLocalJavaSelectionMatchesInstalledDistribution(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{{Tool: "java", Want: "21.0.5", File: ".java-version"}}
	in.Snapshot.Tools = []inventory.ToolState{{Tool: "java", Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true}}}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 3 || !slices.Contains(got.Plan.Steps[0].Argv, "java@temurin-21.0.5") ||
		got.Plan.Steps[1].Edit == nil || !strings.Contains(got.Plan.Steps[1].Edit.After, `java = "temurin-21.0.5"`) ||
		!slices.Equal(got.Plan.Steps[2].Argv[:2], []string{"mise", "trust"}) {
		t.Fatalf("selection differs from installation: %+v", got.Plan.Steps)
	}
}

func TestProjectPreparationDoesNotRewriteExistingPin(t *testing.T) {
	in, env := projectFixture(t)
	in.Snapshot.Tools[0].Managers[0].SwitchOnCD = true
	in.Snapshot.Tools[0].Managers[0].Files = []string{".nvmrc"}
	in.Snapshot.Tools[0].Installations = []inventory.Installation{{Version: "22.12.0", Source: inventory.Source{Manager: "fnm"}}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range got.Plan.Steps {
		if step.Edit != nil && !step.Edit.New {
			t.Fatal("existing requirement was rewritten")
		}
	}
}

func TestAssessProjectClassifiesStates(t *testing.T) {
	in, _ := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{
		{Tool: "node", Want: "22", File: ".nvmrc"},
		{Tool: "python", Want: "3.12", File: ".python-version"},
		{Tool: "docker", File: "compose.yaml"},
		{Tool: "go", Want: "not-a-semver!!!", File: "go.mod"},
		{Tool: "ruby", Want: "3.3", File: ".ruby-version"},
	}
	in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool:          "node",
			Managers:      []inventory.Manager{{Name: "fnm"}},
			Installations: []inventory.Installation{{Version: "24.0.0", OnPath: 1, Source: inventory.Source{Manager: "fnm"}}, {Version: "22.14.0", Source: inventory.Source{Manager: "fnm"}}},
		},
		{
			Tool:          "python",
			Managers:      []inventory.Manager{{Name: "uv"}},
			Installations: []inventory.Installation{{Version: "3.12.12", Source: inventory.Source{Manager: "uv"}}},
		},
		{
			Tool: "docker",
			Docker: &inventory.DockerState{
				Selected: "orbstack",
				Contexts: []inventory.DockerContext{{Name: "orbstack", Local: true, Status: "available", Engine: "OrbStack"}},
			},
		},
		{Tool: "go"},
		{Tool: "ruby", Err: "detector failed"},
	}}
	got := AssessProject(in)
	byTool := map[string]ProjectItem{}
	for _, it := range got.Items {
		byTool[it.Tool] = it
	}
	if n := byTool["node"]; n.Kind != ProjectNeedsSelection || n.Version != "22.14.0" || n.Selected != "24.0.0" || n.Manager != "fnm" {
		t.Fatalf("node assessment = %+v", n)
	}
	if py := byTool["python"]; py.Kind != ProjectPythonAvailable || py.Version != "3.12.12" {
		t.Fatalf("python assessment = %+v", py)
	}
	if d := byTool["docker"]; d.Kind != ProjectReady || d.SelectedEngine != "OrbStack" {
		t.Fatalf("docker assessment = %+v", d)
	}
	if g := byTool["go"]; g.Kind != ProjectUnknown || len(g.Sources) != 1 || g.Sources[0].File != "go.mod" {
		t.Fatalf("go assessment = %+v", g)
	}
	if r := byTool["ruby"]; r.Kind != ProjectUnchecked {
		t.Fatalf("ruby assessment = %+v", r)
	}
}

func TestPlansMarkTerminalRestart(t *testing.T) {
	def, ok := VersionPlan(VersionDefault, VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.14.0"}, nil)
	if !ok || !def.RestartTerminal {
		t.Fatalf("default version plan RestartTerminal=%v ok=%v", def.RestartTerminal, ok)
	}
	inst, ok := VersionPlan(VersionInstall, VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.14.0"}, nil)
	if !ok || inst.RestartTerminal {
		t.Fatalf("install-only plan unexpectedly sets RestartTerminal=%v", inst.RestartTerminal)
	}
	mv, ok := MovePlan(VersionTarget{Tool: "node", Title: "Node.js", Version: "22.14.0"}, "Homebrew", nil)
	if !ok || !mv.RestartTerminal {
		t.Fatalf("move plan RestartTerminal=%v ok=%v", mv.RestartTerminal, ok)
	}
	joined, ok := UpdatePlan([]Plan{inst, def})
	if !ok || !joined.RestartTerminal {
		t.Fatalf("joined plan did not propagate RestartTerminal=%v", joined.RestartTerminal)
	}
}

func TestPrepareProjectCorepackPrepare(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{
		{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"},
	}
	in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool:          "corepack",
			Installations: []inventory.Installation{{Tool: "corepack", Version: "0.32.0", Path: "/opt/homebrew/bin/corepack", OnPath: 1}},
		},
		{
			Tool:     "pnpm",
			Managers: []inventory.Manager{{Name: "corepack", SwitchOnCD: true}},
			Installations: []inventory.Installation{
				{Tool: "pnpm", Version: "9.15.0", Path: "/opt/homebrew/bin/pnpm", OnPath: 1, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
			},
		},
	}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) == 0 {
		t.Fatalf("expected corepack prepare step, got empty plan: %+v", got)
	}
	var sawPrepare bool
	for _, step := range got.Plan.Steps {
		cmd := strings.Join(step.Argv, " ")
		if strings.Contains(cmd, "corepack pack") {
			t.Fatalf("plan must never use corepack pack: %q", cmd)
		}
		if slices.Equal(step.Argv, []string{"corepack", "prepare", "pnpm@10.6.0"}) {
			sawPrepare = true
		}
	}
	if !sawPrepare {
		t.Fatalf("expected corepack prepare pnpm@10.6.0, got steps: %+v", got.Plan.Steps)
	}
}

func TestAssessProjectSingleJSFileNoConflict(t *testing.T) {
	in, _ := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{
		{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"},
	}
	in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "pnpm"},
	}}
	got := AssessProject(in)
	if len(got.Items) != 1 {
		t.Fatalf("items = %+v", got.Items)
	}
	if got.Items[0].Kind == ProjectConflict {
		t.Fatalf("single package.json requirement must not report ProjectConflict: %+v", got.Items[0])
	}
	if got.Items[0].Kind != ProjectInstall && got.Items[0].Kind != ProjectManual {
		t.Fatalf("unexpected kind for missing pnpm: %+v", got.Items[0])
	}
}

func TestPrepareProjectCorepackWithMiseNode(t *testing.T) {
	in, env := projectFixture(t)
	in.Project.Requirements = []projects.Requirement{
		{Tool: "node", Want: "20", File: ".nvmrc"},
		{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"},
	}
	in.Releases = map[string][]string{"node": {"20.20.0"}}
	in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool:     "node",
			Managers: []inventory.Manager{{Name: "mise", Activated: true, SwitchOnCD: true}},
		},
		{
			Tool:          "corepack",
			Installations: []inventory.Installation{{Tool: "corepack", Version: "0.32.0", Path: "/opt/homebrew/bin/corepack", OnPath: 1}},
		},
		{
			Tool:     "pnpm",
			Managers: []inventory.Manager{{Name: "corepack", SwitchOnCD: true}},
		},
	}}
	got, err := PrepareProject(in, env)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mise", "exec", "node@20.20.0", "--", "corepack", "prepare", "pnpm@10.6.0"}
	if !slices.ContainsFunc(got.Plan.Steps, func(s Step) bool { return slices.Equal(s.Argv, want) }) {
		t.Fatalf("expected step %q, got %+v", want, got.Plan.Steps)
	}
}

func TestPrepareProjectComposerAndPHP(t *testing.T) {
	t.Run("missing PHP and Composer both classify as manual without ErrNoCommonVersion or mise.toml", func(t *testing.T) {
		in, env := projectFixture(t)
		in.Project.Requirements = []projects.Requirement{
			{Tool: "php", Want: "~8.3.36", File: "composer.json", Key: "require.php"},
			{Tool: "composer", File: "composer.json"},
		}
		in.Releases = map[string][]string{"php": {"8.3.36", "8.4.5"}}
		in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
			{Tool: "php"},
			{Tool: "composer"},
		}}
		in.Setup = Setup{
			GOOS:     "darwin",
			Managers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}},
			Have:     map[string]bool{"homebrew": true},
			Title:    func(s string) string { return s },
		}
		assessed := AssessProject(in)
		for _, it := range assessed.Items {
			if it.Tool == "composer" && it.Kind != ProjectManual {
				t.Fatalf("AssessProject composer kind = %v text=%q, want ProjectManual", it.Kind, it.Text)
			}
		}

		got, err := PrepareProject(in, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Plan.Steps) != 0 {
			t.Fatalf("unexpected plan steps when PHP cannot be installed by mise: %+v", got.Plan.Steps)
		}
		byTool := map[string]ProjectItem{}
		for _, it := range got.Items {
			byTool[it.Tool] = it
		}
		if php := byTool["php"]; php.Kind != ProjectManual {
			t.Fatalf("php item = %+v, want ProjectManual", php)
		}
		if comp := byTool["composer"]; comp.Kind != ProjectManual || strings.Contains(comp.Text, "no common version") {
			t.Fatalf("composer item = %+v, want ProjectManual without ErrNoCommonVersion", comp)
		}
	})

	t.Run("installed PHP allows Composer install via system package manager", func(t *testing.T) {
		in, env := projectFixture(t)
		in.Project.Requirements = []projects.Requirement{
			{Tool: "php", Want: "~8.3.0", File: "composer.json", Key: "require.php"},
			{Tool: "composer", File: "composer.json"},
		}
		in.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
			{Tool: "php", Installations: []inventory.Installation{{Version: "8.3.15", OnPath: 1, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}}},
			{Tool: "composer"},
		}}
		in.Setup = Setup{
			GOOS:     "darwin",
			Managers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}},
			Have:     map[string]bool{"homebrew": true, "php": true},
			Title:    func(s string) string { return s },
		}
		got, err := PrepareProject(in, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Plan.Steps) == 0 {
			t.Fatalf("expected brew install composer step, got empty plan: %+v", got)
		}
		if !slices.ContainsFunc(got.Items, func(it ProjectItem) bool { return it.Tool == "composer" && it.Kind == ProjectInstall }) {
			t.Fatalf("expected composer ProjectInstall, got %+v", got.Items)
		}
	})
}
