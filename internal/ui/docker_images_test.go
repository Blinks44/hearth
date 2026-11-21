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

func TestDockerImagesView(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Image, error)
	stops := 0
	a.WatchDockerImages = func(target docker.Target, got func([]docker.Image, error)) func() {
		if target.Context != "colima" {
			t.Errorf("target = %+v", target)
		}
		receive = got
		return func() { stops++ }
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), got func(ops.Plan, error)) {
		got(build(nil, nil))
	}
	a.engine.view = dockerImages
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())
	legacy := docker.Image{
		ID: "sha256:" + strings.Repeat("a", 64), Architecture: "amd64", OS: "linux", Size: 1200000,
		RepoTags: []string{"ghcr.io/organization/full-length-legacy-application:2.3", "legacy:latest"},
		Users:    []docker.ImageUser{{Name: "legacy-worker"}},
	}
	dangling := docker.Image{ID: "sha256:" + strings.Repeat("b", 64), Architecture: "arm64"}
	items := []docker.Image{legacy, dangling}
	receive(items, nil)
	tt.Advance(time.Second)
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		head, headOK := tt.Find(strings.ToUpper(locales.DockerImageUsers()))
		user, userOK := tt.Find("legacy-worker")
		if !headOK || !userOK || head.X != user.X {
			t.Fatalf("Used by column at %v: header %+v, cell %+v", size, head, user)
		}
		if !tt.HasText("amd64 · Emulation") || !tt.HasText("Dangling images") || tt.HasText(dangling.ID) {
			t.Fatalf("image list at %v: %q", size, tt.Texts())
		}
		screenshot(t, tt, fmt.Sprintf("docker-images-%d", size[0]))
	}
	a.engine.images.query = "legacy:latest"
	tt.Advance(time.Second)
	if !tt.HasText("legacy-worker") || tt.HasText("Dangling images") {
		t.Fatalf("search by another tag: %q", tt.Texts())
	}
	if err := tt.Click(locales.DockerImageDownload() + "…"); err != nil {
		t.Fatal(err)
	}
	a.engine.images.pullRef = "alpine:3.23"
	tt.Advance(time.Second)
	if err := tt.Click(locales.DockerImageReview()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.op == nil || a.op.plan.DockerAction != ops.DockerImagePull || a.op.started {
		t.Fatal("the pull form did not propose a download")
	}
	a.op.open = false
	tt.Advance(time.Second)
	if err := tt.Click("Networks"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	receive(items[:1], nil)
	if stops != 1 || len(a.engine.images.items) != 2 {
		t.Fatalf("stops = %d, cached images = %+v (late callback should be ignored)", stops, a.engine.images.items)
	}
	if err := tt.Click("Images"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("legacy-worker") {
		t.Fatalf("returning to Images did not reuse cached items immediately: %q", tt.Texts())
	}
}

func TestDockerImageBlockedPlan(t *testing.T) {
	a := NewApp()
	a.RunPlan = func(ops.Plan, func(ops.State)) func() {
		t.Fatal("blocked plan started")
		return nil
	}
	a.Propose(ops.Plan{
		Title: locales.DockerImageRemoveTitle(1), Why: locales.DockerImageRemoveBlocked(),
		Notes: []ops.Note{{Text: locales.DockerImageInUse("stopped-worker")}}, DockerAction: ops.DockerImageRemove,
	})
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	if a.op.canConfirm() || !tt.HasText("Used by containers: stopped-worker") {
		t.Fatalf("blocked plan: %q", tt.Texts())
	}
}
