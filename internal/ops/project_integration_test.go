//go:build integration

package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// This opt-in test downloads tools into temporary manager data directories.
// It must only be enabled on disposable VMs, never the developer's host.
func TestProjectPreparationLive(t *testing.T) {
	if os.Getenv("HEARTH_PROJECT_LIVE") != "1" {
		t.Skip("enable only on disposable VMs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	env, err := sysenv.Terminal(ctx, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sysenv.LookPathAll("mise", env)) == 0 || len(sysenv.LookPathAll("uv", env)) == 0 {
		t.Fatal("live test needs installed mise and uv")
	}
	root := t.TempDir()
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = canonical
	}
	env = append(env, "MISE_DATA_DIR="+filepath.Join(root, "mise-data"), "MISE_CACHE_DIR="+filepath.Join(root, "mise-cache"), "UV_PYTHON_INSTALL_DIR="+filepath.Join(root, "uv-python"), "UV_PYTHON_BIN_DIR="+filepath.Join(root, "uv-bin"))
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors, Env: func(context.Context, run.Runner) (sysenv.Env, sysenv.Env) { return env, env }}
	detect := scanner.Detect
	t.Run("node-pnpm", func(t *testing.T) {
		dir := filepath.Join(root, "web")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for file, text := range map[string]string{".nvmrc": "22.14.0\n", "package.json": `{"packageManager":"pnpm@9.15.0","scripts":{"postinstall":"hearth-project-code-must-not-run"}}`} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		input := ProjectInput{Project: projects.Read(dir), Snapshot: &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true}}}, {Tool: "pnpm", Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true}}}}}, Title: func(s string) string { return s }, Detect: detect}
		got, err := PrepareProject(input, env)
		if err != nil {
			t.Fatal(err)
		}
		// Existing mise must already be activated in the VM. Refuse rather
		// than silently installing another manager or changing its profiles.
		for _, s := range got.Plan.Steps {
			if s.UserPath != nil || s.Edit != nil && !strings.HasPrefix(s.Edit.Path, dir) {
				t.Fatal("VM mise needs activation before this isolated test")
			}
		}
		var log strings.Builder
		state := Execute(ctx, run.Exec{}, got.Plan, &log, func(State) {})
		t.Log(log.String())
		if state.Phase != Succeeded {
			t.Fatalf("real setup: %+v", state)
		}
		for _, tool := range []string{"node", "pnpm"} {
			st, err := detect(ctx, tool)
			if err != nil {
				t.Fatal(err)
			}
			if len(st.Installations) == 0 {
				t.Fatalf("%s not detected after installation: %+v", tool, st)
			}
		}
		if !slices.Contains(projects.Read(dir).Files, "mise.toml") {
			t.Fatal("local selection config missing")
		}
		t.Log("Node and pnpm installed, project scripts were not run")
	})
	t.Run("python-uv", func(t *testing.T) {
		dir := filepath.Join(root, "python")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".python-version"), []byte("3.12.12\n"), 0600); err != nil {
			t.Fatal(err)
		}
		input := ProjectInput{Project: projects.Read(dir), Snapshot: &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "python", Managers: []inventory.Manager{{Name: "uv"}}}}}, Title: func(s string) string { return s }, Detect: detect}
		got, err := PrepareProject(input, env)
		if err != nil {
			t.Fatal(err)
		}
		state := Execute(ctx, run.Exec{}, got.Plan, io.Discard, func(State) {})
		if state.Phase != Succeeded {
			t.Fatalf("real Python setup: %+v", state)
		}
		st, err := detect(ctx, "python")
		if err != nil {
			t.Fatal(err)
		}
		input.Snapshot.Tools = []inventory.ToolState{st}
		next, err := PrepareProject(input, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Plan.Steps) > 0 {
			t.Fatalf("installed Python offered again: %+v", next.Plan.Steps)
		}
		if _, err := os.Stat(filepath.Join(dir, ".venv")); !os.IsNotExist(err) {
			t.Fatalf("unexpected virtual environment: %v", err)
		}
		t.Log("Python installed through uv without .venv or application dependencies")
	})
}

// TestProjectDockerLive observes the selected local engine only. It never
// stops a working engine or starts containers belonging to the project.
func TestProjectDockerLive(t *testing.T) {
	if os.Getenv("HEARTH_PROJECT_LIVE") != "1" {
		t.Skip("enable only on disposable VMs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	env, err := sysenv.Terminal(ctx, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors, Env: func(context.Context, run.Runner) (sysenv.Env, sysenv.Env) { return env, env }}
	st, err := scanner.Detect(ctx, "docker")
	if err != nil {
		t.Fatal(err)
	}
	if st.Docker == nil {
		t.Skip("Docker not installed in this VM")
	}
	localReady := slices.ContainsFunc(st.Docker.Contexts, func(c inventory.DockerContext) bool {
		return c.Name == st.Docker.Selected && c.Local && c.Status == "available"
	})
	if !localReady {
		t.Skip("selected local Docker engine is not available; do not alter other workloads")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := PrepareProject(ProjectInput{Project: projects.Read(dir), Snapshot: &inventory.Snapshot{Tools: []inventory.ToolState{st}}, Title: func(s string) string { return s }}, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 0 || len(got.Items) != 1 || !got.Items[0].Ready {
		t.Fatalf("running local engine not ready: %+v", got)
	}
	t.Log("selected local Docker engine is ready, no containers started")
}

// This test is enabled only after confirming that the disposable VM has no
// running containers and stopping its engine. It restores that engine by
// executing the real project plan; it never stops an engine itself.
func TestProjectDockerStartLive(t *testing.T) {
	if os.Getenv("HEARTH_PROJECT_DOCKER_START") != "1" {
		t.Skip("enable only for a stopped engine in a disposable VM")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	env, err := sysenv.Terminal(ctx, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors, Env: func(context.Context, run.Runner) (sysenv.Env, sysenv.Env) { return env, env }}
	st, err := scanner.Detect(ctx, "docker")
	if err != nil {
		t.Fatal(err)
	}
	if st.Docker == nil {
		t.Fatal("Docker not installed")
	}
	t.Logf("Docker state: %+v; installations: %+v; PATH: %q", *st.Docker, st.Installations, env.Get("PATH"))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := PrepareProject(ProjectInput{Project: projects.Read(dir), Snapshot: &inventory.Snapshot{Tools: []inventory.ToolState{st}}, Title: func(s string) string { return s }, Detect: scanner.Detect}, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.Steps) != 1 || !slices.Equal(got.Plan.Steps[0].Argv, []string{"systemctl", "start", "docker"}) {
		t.Fatalf("unexpected engine start plan: %+v; items: %+v", got.Plan.Steps, got.Items)
	}
	var journal strings.Builder
	state := Execute(ctx, run.Exec{}, got.Plan, &journal, func(State) {})
	t.Log(journal.String())
	if state.Phase != Succeeded {
		t.Fatalf("engine start failed: %+v", state)
	}
	t.Log("stopped local engine started through the project plan; no project containers started")
}
