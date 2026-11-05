package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
)

func dockerSnapshot() *inventory.Snapshot {
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "docker", Installations: []inventory.Installation{
		{Tool: "docker", Version: "27.4.1", Path: "/opt/homebrew/bin/docker", RealPath: "/opt/homebrew/Cellar/docker/27.4.1/bin/docker", OnPath: 1, InGUI: 1, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
	}, Docker: &inventory.DockerState{Selected: "orbstack", Contexts: []inventory.DockerContext{
		{Name: "orbstack", Endpoint: "unix://" + home + "/.orbstack/run/docker.sock", Engine: "OrbStack", Status: "unavailable", Local: true},
		{Name: "colima", Endpoint: "unix://" + home + "/.colima/default/docker.sock", Engine: "Colima", Local: true},
		{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true},
	}, Engines: []inventory.DockerEngine{{Name: "OrbStack", Path: "/Applications/OrbStack.app"}, {Name: "Colima", Path: "/opt/homebrew/bin/colima"}, {Name: "Docker Desktop"}, {Name: "Podman Desktop"}, {Name: "Rancher Desktop"}}, Plugins: []inventory.Package{{Name: "compose", Commands: []string{"docker compose"}}, {Name: "buildx", Commands: []string{"docker buildx"}}}}}}}
}

func openDocker(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/docker")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func TestDockerPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = dockerSnapshot()
	a.Snapshot.Tools[0].Docker.Override = "DOCKER_CONTEXT"
	a.Projects = []projects.Project{{Dir: home + "/dev/api", Requirements: []projects.Requirement{{Tool: "docker", File: "compose.yaml"}}}}
	tt := openDocker(a)
	screenshot(t, tt, "docker")
	for _, text := range []string{"27.4.1", "unavailable", "orbstack", "colima", "How the terminal finds docker", "Container engines", "docker compose", "not checked", "The DOCKER_CONTEXT environment variable overrides the selected context."} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	if issuePage(issues.Issue{Tool: "docker"}) != "/tools/docker" {
		t.Fatal("wrong issue destination")
	}
	if title, explain := issueText(issues.Issue{Kind: issues.DockerUnavailable, Tool: "docker"}, home); title == "" || explain == "" {
		t.Fatal("missing issue text")
	}
	if err := tt.Click("Projects"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("~/dev/api") || tt.HasText("docker compose") {
		t.Fatalf("%q", tt.Texts())
	}
}

func TestDockerWithoutCLI(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "docker", Docker: &inventory.DockerState{Selected: "default", Contexts: []inventory.DockerContext{{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true}}, Engines: []inventory.DockerEngine{{Name: "Docker Desktop"}}}}}}
	tt := openDocker(a)
	if !tt.HasText("Docker CLI is not on the terminal PATH. Engine availability has not been checked.") || !tt.HasText("not checked") {
		t.Fatalf("%q", tt.Texts())
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("How the terminal finds docker") {
		t.Fatal(tt.Texts())
	}
}
