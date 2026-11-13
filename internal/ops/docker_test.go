package ops

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestEngineInstallPlan(t *testing.T) {
	brew := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	winget := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "winget"}
	apt := inventory.PackageManager{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}
	tests := []struct {
		name   string
		engine string
		m      inventory.PackageManager
		cli    bool
		want   [][]string // nil: Hearth does not install it so
		terms  bool
	}{
		{"Docker Desktop cask", "Docker Desktop", brew, true, [][]string{{"/opt/homebrew/bin/brew", "install", "--cask", "docker-desktop"}}, true},
		{"OrbStack cask", "OrbStack", brew, true, [][]string{{"/opt/homebrew/bin/brew", "install", "--cask", "orbstack"}}, true},
		{"Colima brings the CLI it needs", "Colima", brew, false, [][]string{{"/opt/homebrew/bin/brew", "install", "--formula", "colima", "docker"}}, false},
		{"Colima beside a CLI", "Colima", brew, true, [][]string{{"/opt/homebrew/bin/brew", "install", "--formula", "colima"}}, false},
		{"Docker Desktop through WinGet", "Docker Desktop", winget, false, [][]string{{"winget", "install", "--id", "Docker.DockerDesktop", "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}}, true},
		{"Docker Engine from the distribution", "Docker Engine", apt, false, [][]string{{"/bin/sh", "-c", "/usr/bin/apt-get -oDPkg::Lock::Timeout=600 update && /usr/bin/apt-get -oDPkg::Lock::Timeout=600 install -y docker.io"}}, false},
		{"Docker Desktop has no APT package", "Docker Desktop", apt, false, nil, false},
		{"OrbStack only for macOS", "OrbStack", winget, false, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := EngineInstallPlan(tt.engine, tt.m, tt.cli, nil)
			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, want %v", ok, tt.want != nil)
			}
			if !ok {
				return
			}
			var got [][]string
			for _, s := range p.Steps {
				got = append(got, s.Argv)
			}
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("steps = %q, want %q", got, tt.want)
			}
			hasTerms := slices.ContainsFunc(p.Notes, func(n Note) bool { return n.URL != "" })
			if hasTerms != tt.terms || (p.Action != "") != tt.terms {
				t.Errorf("terms note %v, action %q; want terms %v", hasTerms, p.Action, tt.terms)
			}
		})
	}
}

func TestEngineStartPlanWaitsForTheEngine(t *testing.T) {
	defer func(w time.Duration) { engineStartWait = w }(engineStartWait)
	engineStartWait = time.Second
	tries := 0
	stopped := inventory.ToolState{Docker: &inventory.DockerState{Selected: "orbstack", Contexts: []inventory.DockerContext{{Name: "orbstack", Status: "unavailable"}}}}
	started := inventory.ToolState{Docker: &inventory.DockerState{Selected: "orbstack", Contexts: []inventory.DockerContext{{Name: "orbstack", Status: "available", Version: "28.1.1"}}}}
	detect := func(context.Context) (inventory.ToolState, error) {
		tries++
		if tries < 3 {
			return stopped, nil
		}
		return started, nil
	}
	e := testEngine()
	p, ok := EngineStartPlan(e, "orbstack", detect)
	if !ok {
		t.Fatal("no plan")
	}
	found, err := p.Verify(context.Background())
	if err != nil || tries != 3 {
		t.Fatalf("Verify = %q, %v after %d tries", found, err, tries)
	}
	never := func(context.Context) (inventory.ToolState, error) { return stopped, errors.New("no answer") }
	p, _ = EngineStartPlan(e, "orbstack", never)
	engineStartWait = 20 * time.Millisecond
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("Verify passed for an engine that never answered")
	}
	if _, ok := EngineStartPlan(Engine{Name: e.Name}, "orbstack", nil); ok {
		t.Error("plan for an engine not installed")
	}
}

func TestContextPlan(t *testing.T) {
	selected := func(name string) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{Docker: &inventory.DockerState{Selected: name}}, nil
		}
	}
	p, ok := ContextPlan("/opt/homebrew/bin/docker", "orbstack", "colima", selected("colima"))
	if !ok {
		t.Fatal("no plan")
	}
	if want := []string{"/opt/homebrew/bin/docker", "context", "use", "colima"}; len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, want) {
		t.Errorf("steps = %+v, want %q", p.Steps, want)
	}
	if _, err := p.Verify(context.Background()); err != nil {
		t.Errorf("Verify: %v", err)
	}
	p, _ = ContextPlan("/opt/homebrew/bin/docker", "orbstack", "colima", selected("orbstack"))
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("Verify passed with the old context selected")
	}
	for _, name := range []string{"", "-H tcp://x", "a b"} {
		if _, ok := ContextPlan("/opt/homebrew/bin/docker", "default", name, nil); ok {
			t.Errorf("plan for context %q", name)
		}
	}
	if _, ok := ContextPlan("", "default", "colima", nil); ok {
		t.Error("plan without a CLI")
	}
}
