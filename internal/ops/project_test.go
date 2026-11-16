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
	fake.Set(run.Result{ExitCode: 1}, nil, "corepack", "pack", "pnpm@9.15.0", "--output", "corepack.tgz")
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
	if len(got.Plan.Steps) != 1 || !reflect.DeepEqual(got.Plan.Steps[0].Argv, []string{"corepack", "pack", "pnpm@9.15.0", "--output", "corepack.tgz"}) {
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
	if len(got.Plan.Steps) != 2 || !slices.Contains(got.Plan.Steps[0].Argv, "java@temurin-21.0.5") || got.Plan.Steps[1].Edit == nil || !strings.Contains(got.Plan.Steps[1].Edit.After, `java = "temurin-21.0.5"`) {
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
