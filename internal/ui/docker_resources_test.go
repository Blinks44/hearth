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
	a.engine.resources.slots[1].query = "192.0.2"
	tt.Advance(time.Second)
	if !tt.HasText("shop_default") || tt.HasText("old-api_default") || tt.HasText("shop_uploads") {
		t.Fatalf("subnet filter = %q", tt.Texts())
	}
	a.engine.resources.slots[1].query = ""
	a.engine.resources.slots[1].filter = dockerResourceUnused
	tt.Advance(time.Second)
	if !tt.HasText("old-api_default") || tt.HasText("shop_default") {
		t.Fatalf("unused networks = %q", tt.Texts())
	}
}

func TestDockerVolumeRemovesByButton(t *testing.T) {
	a := NewApp()
	runs := 0
	a.RunPlan = func(ops.Plan, func(ops.State)) func() {
		runs++
		return func() {}
	}
	name := "shop_data"
	a.Propose(ops.Plan{Title: locales.DockerVolumeRemoveTitle(name), Action: locales.DockerContainerRemove(),
		DockerAction: ops.DockerResourceRemove, Steps: []ops.Step{{Argv: []string{"docker", "volume", "rm", "--", name}}}})
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	if !a.op.canConfirm() {
		t.Fatal("volume removal should confirm directly with the dialog button")
	}
	if err := tt.Click(locales.DockerContainerRemove()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if runs != 1 {
		t.Fatalf("after clicking remove, ran %d times", runs)
	}
}

func TestDockerResourcesBulkAndCache(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Resource, error, error)
	a.WatchDockerResources = func(_ docker.Target, _ docker.ResourceKind, got func([]docker.Resource, error, error)) func() {
		receive = got
		return func() {}
	}
	planned := 0
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), got func(ops.Plan, error)) {
		planned++
		got(ops.Plan{
			Title: locales.DockerVolumesRemoveTitle(2), Action: locales.DockerContainerRemove(),
			DockerAction: ops.DockerResourceRemove,
			Steps:        []ops.Step{{Argv: []string{"docker", "volume", "rm", "--", "v1"}}, {Argv: []string{"docker", "volume", "rm", "--", "v2"}}},
		}, nil)
	}
	a.engine.view = dockerVolumes
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())
	receive([]docker.Resource{
		{ID: "v1", Name: "vol-alpha", Size: 200},
		{ID: "v2", Name: "vol-beta", Size: 100},
	}, nil, nil)
	tt.Advance(time.Second)
	if err := tt.Click(locales.KubeBulkSelect("vol-alpha")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.KubeBulkSelect("vol-beta")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	bar, ok := tt.Find(locales.DockerSelected(2))
	if !ok {
		t.Fatalf("missing floating bulk bar: %q", tt.Texts())
	}
	tt.ClickAt(bar.X+bar.W+48, bar.Y+bar.H/2)
	tt.Advance(time.Second)
	if planned != 1 || a.op == nil || len(a.op.plan.Steps) != 2 {
		t.Fatalf("bulk volume plan = %+v, planned = %d", a.op, planned)
	}
	a.op.open = false
	tt.Advance(time.Second)
	// Switch to Networks and back to Volumes: cached volumes render immediately.
	if err := tt.Click(locales.DockerViewNetworks()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click(locales.DockerViewVolumes()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("vol-alpha") || !tt.HasText("vol-beta") {
		t.Fatalf("returning to Volumes did not reuse cached list immediately: %q", tt.Texts())
	}
}
