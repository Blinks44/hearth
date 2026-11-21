package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestRowIssueDockerImagesUniformHeightAndTooltip(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Image, error)
	a.WatchDockerImages = func(_ docker.Target, got func([]docker.Image, error)) func() {
		receive = got
		return func() {}
	}
	a.engine.view = dockerImages
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())

	native := docker.Image{
		ID: "sha256:" + strings.Repeat("1", 64), Architecture: "arm64", OS: "linux", Size: 430_000_000,
		RepoTags: []string{"mariadb:10.11.19"},
		Users:    []docker.ImageUser{{Name: "planeta-dev-db-1"}},
	}
	emulated := docker.Image{
		ID: "sha256:" + strings.Repeat("2", 64), Architecture: "amd64", OS: "linux", Size: 1_200_000_000,
		RepoTags: []string{"mattermost/mattermost-team-edition:10.1"},
		Users:    []docker.ImageUser{{Name: "mm-app-1", Active: true}},
	}
	unused := docker.Image{
		ID: "sha256:" + strings.Repeat("3", 64), Architecture: "amd64", OS: "linux", Size: 157_000_000,
		RepoTags: []string{"planeta-nginx:latest"},
	}
	receive([]docker.Image{native, emulated, unused}, nil)
	tt.Advance(time.Second)

	// Emulated images keep their real usage status instead of hiding it.
	hasTexts(t, tt, locales.DockerImagesUsed(), locales.DockerImageStoppedUser(), locales.KubeStorageUnused())

	// Row spacing is uniform between native and emulated rows (no second warning sub-row).
	r0, ok0 := tt.Find("mariadb:10.11.19")
	r1, ok1 := tt.Find("mattermost/mattermost-team-edition:10.1")
	r2, ok2 := tt.Find("planeta-nginx:latest")
	if !ok0 || !ok1 || !ok2 {
		t.Fatalf("missing image rows: %v %v %v", ok0, ok1, ok2)
	}
	step1 := r1.Y - r0.Y
	step2 := r2.Y - r1.Y
	if math.Abs(float64(step1-step2)) > 1 {
		t.Fatalf("image row heights differ: native->emulated=%g, emulated->emulated=%g", step1, step2)
	}

	// Hovering the compact warning badge shows the rich tooltip card.
	badge, ok := tt.Find(locales.Details() + " · " + emulated.Name())
	if !ok {
		t.Fatalf("missing emulation badge in %q", tt.Texts())
	}
	tt.Move(badge.X+badge.W/2, badge.Y+badge.H/2)
	tt.Advance(time.Second)
	hasTexts(t, tt, "amd64 · "+locales.DockerImagesEmulation(), locales.DockerImageEmulationHint("amd64", "aarch64"))
	screenshot(t, tt, "row-issue-docker-images-tooltip")

	// Clicking opens the interactive popover and Escape dismisses it.
	if err := tt.Click(locales.Details() + " · " + emulated.Name()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "amd64 · "+locales.DockerImagesEmulation(), locales.DockerImageEmulationHint("amd64", "aarch64"))
	screenshot(t, tt, "row-issue-docker-images-popover")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText(locales.DockerImageEmulationHint("amd64", "aarch64")) {
		t.Fatal("Escape did not dismiss the row issue popover")
	}
}

func TestRowIssueDockerContainerPortConflictAction(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	var receive func([]docker.Container, error, error)
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		receive = got
		return func() {}
	}
	a.engine.view = dockerContainers
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.engine.data = new(colimaEngine())

	ok1 := docker.Container{ID: strings.Repeat("a", 64), Name: "shop-web-1", Image: "nginx", Compose: docker.ComposeProject{Name: "shop"}}
	ok1.State.Status = "running"
	conflict := docker.Container{ID: strings.Repeat("b", 64), Name: "shop-db-1", Image: "postgres:16", Compose: ok1.Compose}
	conflict.State.Status = "created"
	conflict.State.Error = "Bind for 0.0.0.0:5432 failed: port is already allocated"
	ok2 := docker.Container{ID: strings.Repeat("c", 64), Name: "shop-cache-1", Image: "redis:7", Compose: ok1.Compose}
	ok2.State.Status = "running"
	receive([]docker.Container{ok1, conflict, ok2}, nil, nil)
	tt.Advance(time.Second)

	// Uniform row height between healthy and port-conflict rows.
	y0, _ := tt.Find("shop-web-1")
	y1, _ := tt.Find("shop-db-1")
	y2, _ := tt.Find("shop-cache-1")
	if math.Abs(float64((y1.Y-y0.Y)-(y2.Y-y1.Y))) > 1 {
		t.Fatalf("container row steps differ: %g vs %g", y1.Y-y0.Y, y2.Y-y1.Y)
	}

	// Clicking the issue badge opens the popover with the Open ports action without selecting container details.
	if err := tt.Click(locales.Details() + " · shop-db-1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.engine.containers.detail.id != "" {
		t.Fatalf("clicking container issue badge selected detail %q", a.engine.containers.detail.id)
	}
	hasTexts(t, tt, locales.DockerPortConflict("5432"), locales.DockerOpenPorts())
	screenshot(t, tt, "row-issue-docker-container-popover")

	if err := tt.Click(locales.DockerOpenPorts()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.router.Path() != portsPage || a.portsFilter != "5432" {
		t.Fatalf("after Open ports: page=%q filter=%q", a.router.Path(), a.portsFilter)
	}
}

func TestRowIssueKubePodAndHelmClickDoesNotOpenDetails(t *testing.T) {
	// Pods: clicking the issue badge opens the popover without toggling pod details,
	// and trouble/healthy pod rows share uniform spacing.
	a, tt, _ := openShop(t)
	r0, ok0 := tt.Find("checkout-api-7d9f8c5b-x2k4p")
	r1, ok1 := tt.Find("checkout-api-7d9f8c5b-m8q2z")
	r2, ok2 := tt.Find("checkout-api-7d9f8c5b-p9w4d")
	if !ok0 || !ok1 || !ok2 {
		t.Fatalf("missing pods: %v %v %v", ok0, ok1, ok2)
	}
	if math.Abs(float64((r1.Y-r0.Y)-(r2.Y-r1.Y))) > 1 {
		t.Fatalf("pod row heights differ: crash->ok=%g, ok->ok=%g", r1.Y-r0.Y, r2.Y-r1.Y)
	}
	if err := tt.Click(locales.Details() + " · checkout-api-7d9f8c5b-x2k4p"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cluster.pod != "" {
		t.Fatalf("clicking pod issue badge opened pod details %q", a.cluster.pod)
	}
	hasTexts(t, tt, "Container api keeps failing after it starts",
		"Its last run ended with exit code 1 (Error) 3s after it started")
	screenshot(t, tt, "row-issue-kube-pods-popover")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	// Helm: clicking the issue badge opens the popover without opening Helm details.
	a2 := kubeApp()
	var w clusterWatches
	a2.WatchCluster = w.watch
	a2.cluster.kind = kindHelm
	a2.openCluster("kind-dev")
	tt2 := ui.NewTester(a2.View, 1280, 820)
	tt2.Advance(time.Second)
	w.got[0](kube.Cluster{Taken: time.Now(), HelmReleases: helmTestReleases()}, nil)
	tt2.Advance(time.Second)
	h0, _ := tt2.Find("helm-z")
	h1, _ := tt2.Find("helm-pending")
	h2, _ := tt2.Find("helm-a")
	if math.Abs(float64((h1.Y-h0.Y)-(h2.Y-h1.Y))) > 1 {
		t.Fatalf("helm row heights differ: failed->pending=%g, pending->deployed=%g", h1.Y-h0.Y, h2.Y-h1.Y)
	}
	if err := tt2.Click(locales.Details() + " · helm-z"); err != nil {
		t.Fatal(err)
	}
	tt2.Frame()
	if a2.cluster.helmDetail != nil {
		t.Fatal("clicking Helm issue badge opened release details")
	}
	hasTexts(t, tt2, "The deployment timed out")
	screenshot(t, tt2, "row-issue-kube-helm-popover")
	ttDark := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(ui.DarkTheme())
		a2.View(c)
	}, 1280, 820)
	ttDark.Advance(time.Second)
	screenshot(t, ttDark, "row-issue-kube-helm-popover-dark")
}
