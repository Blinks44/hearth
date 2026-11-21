package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerOverviewProblemsAndLayout(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	var w engineWatches
	a.WatchDocker = w.watch
	problem := docker.Container{ID: "loop-id", Name: "payments-worker", RestartCount: 12}
	problem.State.Status, problem.State.ExitCode = "restarting", 7
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		got([]docker.Container{problem}, nil, nil)
		return func() {}
	}
	var target docker.Logs
	a.StreamDockerLogs = func(log docker.Logs, _ func([]containerlog.LogLine), _ func(error)) func() {
		target = log
		return func() {}
	}
	a.engine.view = dockerOverview
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	e := colimaEngine()
	e.Overview = docker.Overview{Problems: []docker.Container{problem}, CPURead: true, MemoryRead: true, Cores: 1.5, Memory: 6 << 30,
		Disk: docker.OverviewDisk{Usage: []inventory.DockerUsage{{Type: "Images", Bytes: 16_700_000_000, Reclaimable: 9_400_000_000}}}}
	w.got[0](e, nil)
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		cpu, cpuOK := tt.Find(locales.DockerOverviewCPU())
		memory, memoryOK := tt.Find(locales.DockerOverviewMemory())
		if !cpuOK || !memoryOK || cpu.Y != memory.Y || cpu.X+cpu.W > memory.X || memory.X+memory.W > float32(size[0]) {
			t.Fatalf("at %dx%d, CPU=%+v and memory=%+v", size[0], size[1], cpu, memory)
		}
		if !tt.HasText("38%") || !tt.HasText("75%") || !tt.HasText("16.7 GB") || !tt.HasText(problem.Name) {
			t.Fatalf("at %dx%d: %q", size[0], size[1], tt.Texts())
		}
		screenshot(t, tt, "docker-overview-"+map[int]string{1280: "wide", 1000: "narrow"}[size[0]])
	}
	if err := tt.Click(locales.KubeLogsOpen()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if target.Target.Context != "colima" || len(target.Containers) != 1 || target.Containers[0].ID != problem.ID {
		t.Fatalf("log target: %+v", target)
	}
}

func TestDockerOverviewDropsPendingSelectionOnNavigation(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	a.engine.view = dockerOverview
	a.router.Push(dockerEnginePath("colima"))
	var watched func([]docker.Container, error, error)
	a.WatchDockerContainers = func(_ docker.Target, done func([]docker.Container, error, error)) func() {
		watched = done
		done(nil, errors.New("waiting for Docker"), nil)
		return func() {}
	}
	tt := ui.NewTester(a.View, 1280, 820)
	a.requestDockerContainer("colima", "failed-id", "failed", true, false)
	tt.Advance(time.Second)
	if a.engine.containers.pending == nil {
		t.Fatal("selection was lost after a temporary read error")
	}
	a.router.Push(overview.path)
	tt.Advance(time.Second)
	a.router.Push(dockerEnginePath("colima"))
	tt.Advance(time.Second)
	watched([]docker.Container{{ID: "failed-id", Name: "failed"}}, nil, nil)
	tt.Advance(time.Second)
	if a.engine.containers.detail.id != "" || a.engine.containers.query != "" {
		t.Fatal("the previous overview selection survived navigation")
	}
}
