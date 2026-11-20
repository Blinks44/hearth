package ui

import (
	"slices"
	"strings"
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
	for _, text := range []string{"27.4.1", "unavailable", "orbstack", "colima", "LOCAL", "How Hearth works with Docker", "OrbStack · ~/.orbstack/run/docker.sock · Own app · current in docker CLI", "/var/run/docker.sock", "The DOCKER_CONTEXT environment variable overrides the selected context."} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	for tab, text := range map[string]string{"Engines": "Container engines", "Plugins": "docker compose", "PATH": "How the terminal finds docker"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText(text) {
			t.Errorf("%s: missing %q: %q", tab, text, tt.Texts())
		}
	}
	for _, text := range []string{"unix://", "not checked"} {
		if slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.Contains(s, text) }) {
			t.Errorf("unexpected %q: %q", text, tt.Texts())
		}
	}
	if issuePage(issues.Issue{Tool: "docker"}) != "/tools/docker" {
		t.Fatal("wrong issue destination")
	}
	for _, kind := range []issues.Kind{issues.DockerUnavailable, issues.DockerDenied} {
		if title, explain := issueText(issues.Issue{Kind: kind, Tool: "docker"}, home); title == "" || explain == "" {
			t.Fatal("missing issue text")
		}
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

func TestDockerOperations(t *testing.T) {
	a := NewApp()
	a.Snapshot = dockerSnapshot()
	d := a.Snapshot.Tools[0].Docker
	d.Selected = "podman"
	d.Contexts = append(d.Contexts, inventory.DockerContext{Name: "podman", Endpoint: "unix://" + home + "/.local/share/containers/podman.sock", Engine: "Podman", Status: "unavailable", Local: true})
	d.Engines = append(d.Engines, inventory.DockerEngine{Name: "Podman", Path: "/opt/homebrew/bin/podman"})
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}}})
	tt := openDocker(a)
	screenshot(t, tt, "docker-actions")
	if err := tt.Click("Start Podman"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Start Podman" || a.op.plan.Verify == nil && a.DetectTool != nil {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Use this context"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/docker", "context", "use", "orbstack"}) {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Engines"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Install Docker Desktop"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "install", "--cask", "docker-desktop"}) || a.op.plan.Action != "Accept the terms and install" {
		t.Fatalf("plan %+v", a.op)
	}
	screenshot(t, tt, "docker-install")
}

// Kiri is in no system manager: its row opens the maker's page, and an
// installed Kiri whose context does not answer starts from the header.
func TestDockerKiri(t *testing.T) {
	a := NewApp()
	a.Snapshot = dockerSnapshot()
	d := a.Snapshot.Tools[0].Docker
	d.Engines = append(d.Engines, inventory.DockerEngine{Name: "Kiri"})
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}}})
	tt := openDocker(a)
	if err := tt.Click("Engines"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "docker-kiri")
	if tt.HasText("Install Kiri") {
		t.Fatal("Kiri installs through a manager")
	}
	if err := tt.Click("Download from runkiri.com"); err != nil {
		t.Fatal(err)
	}
	if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://runkiri.com/releases/"}) || a.op != nil {
		t.Fatalf("opened %q, op %+v", got, a.op)
	}

	d.Engines[len(d.Engines)-1].Path = "/Applications/Kiri.app"
	d.Selected = "kiri"
	d.Contexts = append(d.Contexts, inventory.DockerContext{Name: "kiri", Endpoint: "unix://" + home + "/.kiri/run/docker.sock", Engine: "Kiri", Status: "unavailable", Local: true})
	tt = openDocker(a)
	if err := tt.Click("Start Kiri"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Start Kiri" {
		t.Fatalf("plan %+v", a.op)
	}
}
