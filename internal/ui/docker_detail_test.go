package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/locales"
)

func TestDockerDetailsAndLogs(t *testing.T) {
	a := NewApp()
	a.Snapshot = colimaSnapshot()
	a.WatchDocker = (&engineWatches{}).watch
	item := docker.Container{ID: strings.Repeat("a", 64), Name: "shop-web-1"}
	item.State.Status = "running"
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		got([]docker.Container{item}, nil, nil)
		return func() {}
	}
	a.ReadDockerDetail = func(target docker.Target, id string, got func(docker.ContainerDetail, error)) func() {
		if id != item.ID || target.Context != "colima" {
			t.Fatalf("detail target = %+v %s", target, id)
		}
		got(docker.ContainerDetail{Image: "shop:latest", Platform: "linux/arm64", EnvKeys: []string{"KEY"}, Inspect: `{"Env":["KEY=••••••••"]}`}, nil)
		return func() {}
	}
	a.SecretTimer = func(time.Duration, func()) func() { return func() {} }
	a.ReadDockerEnv = func(_ docker.Target, id, key string, got func([]byte, error)) func() {
		if id != item.ID || key != "KEY" {
			t.Fatalf("read env %s of %s", key, id)
		}
		got([]byte("test-env-value"), nil)
		return func() {}
	}
	var gotLogs func([]containerlog.LogLine)
	var logsStopped int
	a.StreamDockerLogs = func(l docker.Logs, got func([]containerlog.LogLine), _ func(error)) func() {
		if len(l.Containers) != 1 || l.Containers[0].ID != item.ID || l.Target.Context != "colima" {
			t.Fatalf("log targets = %+v", l)
		}
		gotLogs = got
		return func() { logsStopped++ }
	}
	a.SaveLogs = func(name, text string, done func(string, error)) {
		if name != item.Name+".log" || text != "hello logs\n" {
			t.Fatalf("save %q: %q", name, text)
		}
		done("/tmp/sample.log", nil)
	}
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	a.engine.data = new(colimaEngine())
	tt.Advance(time.Second)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	click(item.Name)
	hasTexts(t, tt, "shop:latest", "linux/arm64", "Shell")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	status, ok := tt.Find("STATUS")
	if !ok || status.X+status.W > 550 {
		t.Fatalf("compact list status is outside its pane: %+v", status)
	}
	click("Env")
	hasTexts(t, tt, "KEY", "••••••••")
	click(locales.KubeSecretShow("KEY"))
	if a.engine.containers.detail.secret.revealed == nil || tt.HasText("test-env-value") {
		t.Fatal("reveal must draw the value outside the text tree")
	}
	tt.Advance(31 * time.Second)
	if a.engine.containers.detail.secret.revealed != nil {
		t.Fatal("value remained visible past 30 seconds")
	}
	click(locales.KubeSecretCopy("KEY"))
	if tt.Clipboard() != "test-env-value" || a.engine.containers.detail.secret.revealed != nil {
		t.Fatal("copy did not keep the value hidden")
	}
	click("Inspect")
	hasTexts(t, tt, `{"Env":["KEY=••••••••"]}`)
	click(locales.KubeLogsOpen())
	gotLogs([]containerlog.LogLine{{Text: "hello logs"}})
	tt.Advance(time.Second)
	hasTexts(t, tt, "hello logs")
	click(locales.KubeLogsSave())
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if logsStopped != 1 || a.engine.containers.logs != nil {
		t.Fatalf("closed log: stops=%d", logsStopped)
	}
	click(locales.KubeLogsOpen())
	a.router.Push(dockerPage)
	tt.Advance(time.Second)
	if logsStopped != 2 || a.engine.containers.detail.id != "" {
		t.Fatalf("route change: stops=%d, detail=%+v", logsStopped, a.engine.containers.detail)
	}
}
