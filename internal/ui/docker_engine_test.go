package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
)

// engineWatches records the engine page's watches: what each reads, how
// to answer it, and whether it was stopped.
type engineWatches struct {
	targets []docker.Target
	got     []func(docker.Engine, error)
	stopped []bool
}

func (w *engineWatches) watch(t docker.Target, _ bool, got func(docker.Engine, error)) func() {
	i := len(w.targets)
	w.targets = append(w.targets, t)
	w.got = append(w.got, got)
	w.stopped = append(w.stopped, false)
	return func() { w.stopped[i] = true }
}

func (w *engineWatches) running() []docker.Target {
	var ts []docker.Target
	for i, t := range w.targets {
		if !w.stopped[i] {
			ts = append(ts, t)
		}
	}
	return ts
}

// colimaSnapshot is dockerSnapshot with Colima selected and answering.
func colimaSnapshot() *inventory.Snapshot {
	snap := dockerSnapshot()
	d := snap.Tools[0].Docker
	d.Selected = "colima"
	d.Contexts[0].Status = ""
	d.Contexts[1].Status = "available"
	d.Contexts = append(d.Contexts, inventory.DockerContext{Name: "build-server", Endpoint: "ssh://", Local: false})
	return snap
}

func colimaEngine() docker.Engine {
	return docker.Engine{Taken: time.Now(), Version: "27.5.1", OS: "Ubuntu 24.04.1 LTS", Arch: "aarch64", CPUs: 4, Memory: 8 << 30,
		Containers: 8, Running: 4, Stopped: 4, Images: 23}
}

func TestDockerEngineOpenAndBack(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	var w engineWatches
	a.WatchDocker = w.watch
	a.WatchDockerResources = func(_ docker.Target, _ docker.ResourceKind, got func([]docker.Resource, error, error)) func() {
		got(nil, nil, nil)
		return func() {}
	}
	tt := openDocker(a)
	if !tt.HasText("Open colima") {
		t.Fatalf("no header button: %q", tt.Texts())
	}
	if len(w.targets) != 0 {
		t.Fatal("the Docker page read an engine")
	}
	if err := tt.Click("Open colima"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != dockerEnginePage || a.router.Query("context") != "colima" {
		t.Fatalf("at %s", a.router.Path())
	}
	want := docker.Target{Docker: "/opt/homebrew/bin/docker", Context: "colima", Endpoint: "unix://" + home + "/.colima/default/docker.sock"}
	if got := w.running(); len(got) != 1 || got[0] != want {
		t.Fatalf("watching %+v", got)
	}
	if !tt.HasText("Reading colima…") {
		t.Fatalf("no skeleton: %q", tt.Texts())
	}
	w.got[0](colimaEngine(), nil)
	tt.Advance(time.Second)
	if a.engine.view != dockerContainers {
		t.Fatalf("first view = %d", a.engine.view)
	}
	a.engine.view = dockerOverview
	tt.Advance(time.Second)
	screenshot(t, tt, "docker-engine")
	for _, text := range []string{"Overview", "Containers", "Images", "Volumes", "Networks", "4 of 8 running", "23", "Docker 27.5.1", "8.6 GB"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Networks"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.engine.view != dockerNetworks || !tt.HasText("No matching networks.") {
		t.Fatalf("view %d: %q", a.engine.view, tt.Texts())
	}
	if len(w.running()) != 1 {
		t.Fatal("changing the view read the engine again")
	}
	if err := tt.Click("Back to Docker"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != dockerPage || len(w.running()) != 0 {
		t.Fatalf("at %s, watching %+v", a.router.Path(), w.running())
	}
	// The sidebar goes straight back to the engine, on the view left.
	if got := a.entryPath(dockerPage); got != dockerEnginePath("colima") || a.engine.view != dockerNetworks {
		t.Fatalf("entry %s, view %d", got, a.engine.view)
	}
}

func TestDockerEntryPath(t *testing.T) {
	a := NewApp()
	a.Snapshot = dockerSnapshot() // orbstack selected, not answering
	a.WatchDocker = (&engineWatches{}).watch
	if got := a.entryPath(dockerPage); got != dockerPage {
		t.Fatalf("not answering: %s", got)
	}
	a.Snapshot = colimaSnapshot()
	if got := a.entryPath(dockerPage); got != dockerEnginePath("colima") {
		t.Fatalf("answering: %s", got)
	}
	if got := a.entryPath(kubePage); got != kubePage {
		t.Fatalf("other page: %s", got)
	}
	a.WatchDocker = nil
	if got := a.entryPath(dockerPage); got != dockerPage {
		t.Fatalf("no engine view: %s", got)
	}
}

func TestDockerEngineContexts(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	tt := openDocker(a)
	// Open for each local context; none for the remote one.
	opens := 0
	for _, text := range tt.Texts() {
		if text == "Open" {
			opens++
		}
	}
	if opens != 3 || !tt.HasText("REMOTE") || !tt.HasText("Remote endpoint: Hearth does not connect during a scan.") {
		t.Fatalf("%d Open buttons: %q", opens, tt.Texts())
	}
}

func TestDockerEngineSwitchContext(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	var w engineWatches
	a.WatchDocker = w.watch
	a.Home = home
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	w.got[0](colimaEngine(), nil)
	tt.Advance(time.Second)
	// The path's context select, as clicking it does.
	a.engine.contextOpen = true
	tt.Advance(time.Second)
	if !tt.HasText("Choosing an engine here changes only what Hearth shows. The docker CLI keeps its context.") || tt.HasText("build-server") {
		t.Fatalf("popover: %q", tt.Texts())
	}
	if err := tt.Click("orbstack"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	got := w.running()
	if len(got) != 1 || got[0].Context != "orbstack" || a.engine.data != nil {
		t.Fatalf("watching %+v, data %+v", got, a.engine.data)
	}
	if a.op != nil {
		t.Fatal("switching the view proposed a plan")
	}
}

func TestDockerEngineDown(t *testing.T) {
	a := NewApp()
	a.Snapshot = dockerSnapshot() // orbstack selected, not answering
	var w engineWatches
	a.WatchDocker = w.watch
	a.Home = home
	a.router.Push(dockerEnginePath("orbstack"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	w.got[0](docker.Engine{}, &docker.Error{Text: "Cannot connect to the Docker daemon. Is the docker daemon running?"})
	tt.Advance(time.Second)
	screenshot(t, tt, "docker-engine-down")
	for _, text := range []string{"orbstack does not answer", "Cannot connect to the Docker daemon. Is the docker daemon running?", "Start OrbStack"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	if len(w.running()) != 1 {
		t.Fatal("the watch stopped on a failed read")
	}
}

func TestDockerEngineRemote(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	var w engineWatches
	a.WatchDocker = w.watch
	a.Home = home
	a.router.Push(dockerEnginePath("build-server"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if len(w.targets) != 0 || !tt.HasText("build-server is a remote context: Hearth opens local engines only.") {
		t.Fatalf("watches %+v: %q", w.targets, tt.Texts())
	}
}

func TestStopDocker(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	var w engineWatches
	a.WatchDocker = w.watch
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.StopDocker()
	if len(w.running()) != 0 {
		t.Fatal("still watching after the window closed")
	}
	// A read that arrives after the stop is dropped.
	w.got[0](colimaEngine(), nil)
	if a.engine.data != nil {
		t.Fatal("kept a read after the stop")
	}
}
