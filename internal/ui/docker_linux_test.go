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
	title := locales.PkgInstallTitle("Docker Engine")
	hasTexts(t, tt, locales.ColimaRecommended(), title, "Colima")
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
