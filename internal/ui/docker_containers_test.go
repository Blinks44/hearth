package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerContainersView(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	dir := filepath.Join(t.TempDir(), "shop")
	a.Projects = []projects.Project{{Dir: dir, Files: []string{"compose.yaml"}}}
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Container, error, error)
	stops := 0
	a.WatchDockerContainers = func(target docker.Target, got func([]docker.Container, error, error)) func() {
		if target.Context != "colima" {
			t.Errorf("target = %+v", target)
		}
		receive = got
		return func() { stops++ }
	}
	a.engine.view = dockerContainers
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())
	web := docker.Container{ID: strings.Repeat("a", 64), Name: "shop-web-1", Image: "nginx", CPU: "4.18%", Ports: "5432/tcp, 0.0.0.0:8080->80/tcp, [::]:8080->80/tcp",
		Compose: docker.ComposeProject{Name: "shop", Dir: dir}}
	web.State.Status = "running"
	crash := docker.Container{ID: strings.Repeat("b", 64), Name: "shop-api-1", Compose: web.Compose}
	crash.State.Status, crash.State.ExitCode = "exited", 127
	kind := docker.Container{ID: strings.Repeat("c", 64), Name: "dev-node", Kind: "dev"}
	kind.State.Status = "running"
	receive([]docker.Container{web, crash, kind}, nil, nil)
	tt.Advance(time.Second)
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		head, headOK := tt.Find("CPU")
		cell, cellOK := tt.Find(web.CPU)
		if !headOK || !cellOK || head.X != cell.X {
			t.Fatalf("CPU column at %v: header %+v, cell %+v", size, head, cell)
		}
		screenshot(t, tt, "docker-containers-"+strconv.Itoa(size[0]))
		if !tt.HasText(locales.Details()+" · shop-api-1") || !tt.HasText("shop-web-1") || !tt.HasText("8080 → 80") {
			t.Fatalf("container rows: %q", tt.Texts())
		}
	}
	if err := tt.Click(locales.Details() + " · shop-api-1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("The startup command was not found (exit 127).") {
		t.Fatalf("container issue popover: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	a.engine.containers.query = "8080"
	tt.Advance(time.Second)
	if tt.HasText("shop-api-1") || !tt.HasText("shop-web-1") {
		t.Fatalf("port search: %q", tt.Texts())
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), got func(ops.Plan, error)) {
		got(build(nil, nil))
	}
	if err := tt.Click(locales.KubeBulkSelect(web.Name)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(locales.DockerContainerRemove() + "…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.op == nil || !a.op.open || len(a.op.plan.Steps) != 1 || a.op.plan.DockerAction != ops.DockerRemove {
		t.Fatal("the selected container did not open a remove plan")
	}
	if err := tt.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	a.engine.refresh++
	tt.Advance(time.Second)
	if stops != 1 || !tt.HasText("8080 → 80") || !a.engine.containers.bulk.selected[web.ID] {
		t.Fatalf("refresh lost the visible list or selection: %q", tt.Texts())
	}
	if err := tt.Click("Networks"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	receive([]docker.Container{web}, nil, nil)
	if stops != 2 || len(a.engine.containers.items) != 3 {
		t.Fatalf("stops = %d, cached items = %+v (late callback should be ignored)", stops, a.engine.containers.items)
	}
	if err := tt.Click("Containers"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("8080 → 80") {
		t.Fatalf("returning to Containers did not reuse cached items immediately: %q", tt.Texts())
	}
}
