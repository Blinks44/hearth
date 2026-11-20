package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerResourcesView(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Resource, error, error)
	var watched docker.ResourceKind
	stops := 0
	a.WatchDockerResources = func(target docker.Target, kind docker.ResourceKind, got func([]docker.Resource, error, error)) func() {
		if target.Context != "colima" {
			t.Errorf("target = %+v", target)
		}
		receive, watched = got, kind
		return func() { stops++ }
	}
	a.engine.view = dockerVolumes
	a.router.Push(dockerEnginePath("colima"))
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), got func(ops.Plan, error)) {
		got(ops.Plan{Title: "Remove volume orphan?", Why: locales.DockerVolumeRemoveWhy("colima"),
			Action: locales.DockerContainerRemove(), DockerAction: ops.DockerResourceRemove,
			Steps: []ops.Step{{Argv: []string{"docker", "volume", "rm", "--", "orphan"}}}}, nil)
	}
	a.RunPlan = func(ops.Plan, func(ops.State)) func() {
		t.Fatal("review started removal")
		return nil
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())
	if watched != docker.VolumeResource {
		t.Fatalf("watching %q", watched)
	}
	created := time.Now().AddDate(0, -2, 0)
	volumes := []docker.Resource{
		{ID: "orphan", Name: "orphan", Size: 1200000000, Created: created},
		{ID: "shop_pgdata", Name: "shop_pgdata", Project: "shop", Size: 4100000000, Created: created,
			Users: []docker.ResourceUser{{Name: "shop-db-1", Project: "shop", Active: true}}},
		{ID: "shop_uploads", Name: "shop_uploads", Project: "shop", Size: 1200000000, Created: created,
			Users: []docker.ResourceUser{{Name: "shop-api-1", Project: "shop"}}},
		{ID: "billing_redis", Name: "billing_redis", Project: "billing", Size: 120000000, Created: created,
			Users: []docker.ResourceUser{{Name: "billing-redis-1", Project: "billing"}}},
	}
	receive(volumes, nil, nil)
	tt.Advance(time.Second)
	checkColumns := func(size [2]int, text string) {
		t.Helper()
		head, headOK := tt.Find(strings.ToUpper(locales.DockerImageUsers()))
		cell, cellOK := tt.Find(text)
		if !headOK || !cellOK || head.X != cell.X || cell.X+cell.W > float32(size[0]) {
			t.Fatalf("Used by at %v: header %+v, cell %+v", size, head, cell)
		}
	}
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		checkColumns(size, "shop-api-1")
		screenshot(t, tt, fmt.Sprintf("docker-volumes-%d", size[0]))
	}
	if err := tt.Click(locales.DockerContainerRemove() + "…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.canConfirm() || a.op.started {
		t.Fatal("expected removal preview")
	}
	screenshot(t, tt, "docker-volume-removal")
	a.op.open = false
	tt.Advance(time.Second)
	receiveVolumes := receive
	if err := tt.Click(locales.DockerViewNetworks()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if watched != docker.NetworkResource || stops != 1 {
		t.Fatalf("watching %q, stops = %d", watched, stops)
	}
	receiveVolumes(volumes, nil, nil)
	networks := []docker.Resource{
		{ID: "bridge", Name: "bridge", Driver: "bridge", Created: created},
		{ID: "old-api", Name: "old-api_default", Driver: "bridge", Project: "old-api"},
		{ID: "shop", Name: "shop_default", Driver: "bridge", Project: "shop", Subnets: []string{"192.0.2.0/24"},
			Users: []docker.ResourceUser{{Name: "shop-api-1", Project: "shop", Active: true}}},
	}
	receive(networks, nil, nil)
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		checkColumns(size, "shop-api-1")
		screenshot(t, tt, fmt.Sprintf("docker-networks-%d", size[0]))
	}
	a.engine.resources.query[1] = "192.0.2"
	tt.Advance(time.Second)
	if !tt.HasText("shop_default") || tt.HasText("old-api_default") || tt.HasText("shop_uploads") {
		t.Fatalf("subnet filter = %q", tt.Texts())
	}
	a.engine.resources.query[1] = ""
	a.engine.resources.filter[1] = dockerResourceUnused
	tt.Advance(time.Second)
	if !tt.HasText("old-api_default") || tt.HasText("shop_default") {
		t.Fatalf("unused networks = %q", tt.Texts())
	}
}

func TestDockerVolumeRequiresExactName(t *testing.T) {
	a := NewApp()
	runs := 0
	a.RunPlan = func(ops.Plan, func(ops.State)) func() {
		runs++
		return func() {}
	}
	name := "shop_data"
	a.Propose(ops.Plan{Title: locales.DockerVolumeRemoveTitle(name), Action: locales.DockerContainerRemove(),
		ConfirmName: name, DockerAction: ops.DockerResourceRemove, Steps: []ops.Step{{Argv: []string{"docker", "volume", "rm", "--", name}}}})
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	if !tt.HasText(locales.KubeResourceConfirm(name)) || a.op.canConfirm() {
		t.Fatal("name confirmation missing")
	}
	for _, decision := range []struct {
		typed string
		runs  int
	}{{"SHOP_DATA", 0}, {name, 1}} {
		a.op.confirm = decision.typed
		tt.Advance(time.Second)
		_ = tt.Click(locales.DockerContainerRemove())
		tt.Advance(time.Second)
		if runs != decision.runs {
			t.Fatalf("after %q, ran %d times", decision.typed, runs)
		}
	}
}
