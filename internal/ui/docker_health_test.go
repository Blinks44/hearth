package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerHealthUI(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = colimaSnapshot()
	d := a.Snapshot.Tools[0].Docker
	d.ConfigDir = home + "/.docker"
	d.CredentialIssues = []inventory.DockerCredentialIssue{{Registry: "ghcr.io", Key: "ghcr.io", Name: "desktop", Replacement: "osxkeychain"}}
	d.Registries = []inventory.DockerRegistry{{Name: "registry.example:5000"}, {Name: "ghcr.io", MissingHelper: true}}
	d.BrokenPlugins = []string{"docker-compose"}
	d.PluginDir = d.ConfigDir + "/cli-plugins"
	d.Health = inventory.DockerHealth{Emulated: []inventory.DockerEmulatedContainer{
		{ID: "legacy", Name: "legacy-api", ImageArch: "amd64", EngineArch: "arm64"},
	}}
	a.Shell = &fakeShell{}
	var planBuilt bool
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		planBuilt = true
		done(ops.Plan{Title: locales.DockerHelperPlanTitle(), Steps: []ops.Step{{Edit: &ops.FileEdit{
			Path: d.ConfigDir + "/config.json", Before: `{"credsStore":"desktop"}`, After: `{"credsStore":"osxkeychain"}`,
		}}}}, nil)
	}
	var login []string
	a.OpenDockerLogin = func(argv []string, done func(error)) { login = argv; done(nil) }
	var w engineWatches
	a.WatchDocker = w.watch
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		got([]docker.Container{{ID: "legacy", Name: "legacy-api"}}, nil, nil)
		return func() {}
	}
	a.router.Push(dockerPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	tt.SetSize(1280, 1100)
	tt.Advance(time.Second)
	if err := tt.Click(locales.DockerRegistryLogin()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/opt/homebrew/bin/docker", "--config", d.ConfigDir, "login", "registry.example:5000"}; !slices.Equal(login, want) {
		t.Errorf("login argv = %q, want %q", login, want)
	}
	for _, size := range [][2]int{{1000, 680}, {1280, 820}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		for _, label := range []string{
			locales.DockerAttention(), locales.DockerHelperRegistry("ghcr.io"),
			locales.DockerBrokenPlugin("docker-compose"), locales.DockerEmulatedContainer("legacy-api"),
		} {
			if !tt.HasText(label) {
				t.Fatalf("at %v missing %q: %v", size, label, tt.Texts())
			}
		}
		screenshot(t, tt, fmt.Sprintf("docker-health-%d", size[0]))
	}
	if err := tt.Click(locales.DockerHelperFix()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !planBuilt || a.op == nil || a.op.started {
		t.Fatal("diagnosis did not open a confirmation plan")
	}
	screenshot(t, tt, "docker-health-repair")
	a.op.open = false
	tt.Advance(time.Second)
	d.Colima = &inventory.ColimaState{Profiles: []inventory.ColimaProfile{{
		Name: "default", Runtime: "docker", Status: "running", CPU: 4, Memory: 8 << 30, Disk: 60 << 30,
		Socket: d.Contexts[1].Endpoint,
	}}}
	d.Health.ContainerMemory, d.Health.Memory = 13<<29, 8<<30
	a.engine.view = dockerOverview
	a.router.Push(dockerEnginePath("colima"))
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	e := colimaEngine()
	e.Health = d.Health
	w.got[len(w.got)-1](e, nil)
	for _, width := range []int{1280, 1000} {
		height := 820
		if width == 1000 {
			height = 680
		}
		tt.SetSize(width, height)
		tt.Advance(time.Second)
		if !tt.HasText(locales.DockerLowMemory()) {
			t.Fatal("the overview did not show Colima memory pressure")
		}
		screenshot(t, tt, fmt.Sprintf("docker-health-overview-%d", width))
	}
	if box, ok := tt.Find(locales.KubeClusterOpen()); ok && box.Y+box.H > 680 {
		tt.Scroll(500, 340, 0, box.Y-340)
		tt.Advance(time.Second)
	}
	if err := tt.Click(locales.KubeClusterOpen()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.engine.view != dockerContainers || a.engine.containers.detail.id != "legacy" {
		t.Fatal("opening a diagnosis did not select its container")
	}
}

func TestDockerDefaultSocketLinkPlan(t *testing.T) {
	if goos == "windows" {
		t.Skip("default /var/run/docker.sock symlink plan is Unix-only")
	}
	a := NewApp()
	a.Home = home
	a.Snapshot = colimaSnapshot()
	d := a.Snapshot.Tools[0].Docker
	d.DefaultSocketIssue = "different"
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home}, nil))
	}
	a.router.Push(dockerPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	btn := locales.DockerSocketLinkButton(d.Selected)
	if !tt.HasText(btn) {
		t.Fatalf("missing link socket button %q in %q", btn, tt.Texts())
	}
	if err := tt.Click(btn); err != nil {
		t.Fatalf("click %q: %v", btn, err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || len(a.op.plan.Steps) != 1 || !a.op.plan.Steps[0].Admin {
		t.Fatalf("expected pending admin symlink plan, got %+v", a.op)
	}
	wantArgv := []string{"ln", "-sfn", home + "/.colima/default/docker.sock", "/var/run/docker.sock"}
	if !slices.Equal(a.op.plan.Steps[0].Argv, wantArgv) {
		t.Fatalf("symlink argv = %q, want %q", a.op.plan.Steps[0].Argv, wantArgv)
	}
}
