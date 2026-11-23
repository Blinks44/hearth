package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerLinuxInstallsNativeEngineWithHomebrew(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "docker", Docker: &inventory.DockerState{Engines: []inventory.DockerEngine{
			{Name: "Docker Engine"}, {Name: "Colima", Path: "/brew/bin/colima"},
		}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{ID: "brew", Path: "/brew/bin/brew"},
			{ID: "apt", Path: "/usr/bin/apt-get"},
		}},
	}}
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), _ func(ops.Plan, error)) {
		t.Fatal("opening the Docker page built a Colima installation plan")
	}
	tt := openDocker(a)
	title := locales.DockerEngineInstall()
	hasTexts(t, tt, locales.DockerHearthTitle(), locales.DockerHearthNeedToKnow(), locales.ColimaRecommended(), title, "Colima")
	if tt.HasText(locales.ColimaInstall()) || tt.HasText(locales.ColimaMigrate()) {
		t.Fatalf("offered Colima installation: %v", tt.Texts())
	}
	if err := tt.Click(title); err != nil {
		t.Fatal(err)
	}
	if a.op == nil || !slices.ContainsFunc(a.op.plan.Steps, func(step ops.Step) bool {
		return step.Admin && strings.Contains(strings.Join(step.Argv, " "), "docker.io")
	}) {
		t.Fatalf("expected native apt plan: %+v", a.op)
	}
}

func TestDockerLinuxKiriServiceAndSwitchNotes(t *testing.T) {
	if got := dockerManagementLabel("Kiri"); got != locales.ColimaExternalService() {
		t.Fatalf("Kiri management label = %q, want %q", got, locales.ColimaExternalService())
	}
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool: "docker",
			Installations: []inventory.Installation{
				{Tool: "docker", Version: "28.0.1", Path: "/usr/local/bin/docker", OnPath: 1},
			},
			Docker: &inventory.DockerState{
				Selected: "kiri",
				Contexts: []inventory.DockerContext{{Name: "kiri", Engine: "Kiri", Endpoint: "unix:///run/kiri.sock", Local: true, Status: "available"}},
				Engines: []inventory.DockerEngine{
					{Name: "Kiri", Path: "/usr/bin/kiri"},
					{Name: "Docker Engine"},
				},
			},
		},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{ID: "apt", Path: "/usr/bin/apt-get"},
		}},
	}}
	tt := openDocker(a)
	hasTexts(t, tt,
		locales.ColimaExternalService(),
		locales.DockerHearthTitle(),
		locales.DockerEngineSwitchHint("Kiri"),
		locales.DockerEngineInstall(),
	)
	if err := tt.Click(locales.DockerEngineInstall()); err != nil {
		t.Fatal(err)
	}
	if a.op == nil {
		t.Fatal("expected native Docker Engine switch plan")
	}
	for _, want := range []string{
		locales.DockerSwitchUntouchedTitle(),
		locales.DockerSwitchAutostartTitle(),
		locales.DockerSwitchRollbackTitle(),
	} {
		if !slices.ContainsFunc(a.op.plan.Notes, func(n ops.Note) bool { return n.Title == want }) {
			t.Errorf("missing switch consequence note %q in %+v", want, a.op.plan.Notes)
		}
	}
}
