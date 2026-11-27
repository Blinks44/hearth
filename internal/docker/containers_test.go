package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerContainerData(t *testing.T) {
	var r run.Fake
	r.Stdout(string(fixture(t, "containers-ps.jsonl")), colima.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
	r.Stdout(string(fixture(t, "containers-inspect.jsonl")), colima.Command(nil, "container", "inspect", "--format", inspectContainer, "--",
		strings.Repeat("a", 64), strings.Repeat("b", 64)).Args...)
	r.Stdout(string(fixture(t, "containers-stats.jsonl")), colima.Command(nil, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}").Args...)
	items, err := ReadContainers(t.Context(), &r, []string{"DOCKER_CONTEXT=elsewhere"}, colima)
	if err != nil || len(items) != 2 {
		t.Fatalf("list: %+v, %v", items, err)
	}
	if err := sampleContainers(t.Context(), &r, nil, colima, items); err != nil {
		t.Fatal(err)
	}
	broken, web := items[0], items[1]
	if broken.Name != "port-conflict" || !broken.Problem() || broken.Active() || broken.ConflictPort() != 5432 || broken.CPU != "" {
		t.Fatalf("port conflict: %+v", broken)
	}
	if web.Compose.Name != "shop" || web.Compose.Dir != "/work/shop" || !web.Active() || !web.Problem() ||
		web.State.StartedAt.IsZero() || web.CPU != "0.12%" || web.Memory != "2.5MiB" || !strings.Contains(web.Ports, "8080") {
		t.Fatalf("running container: %+v", web)
	}
	if !slices.Contains(r.Calls()[0].Env, "DOCKER_CONTEXT=") {
		t.Fatal("the command did not clear the terminal's Docker context")
	}
}

func TestDockerWatchCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var runner run.Fake
	WatchContainers(ctx, &runner, nil, colima, func([]Container, error, error) { t.Error("read after cancellation") })
	if len(runner.Calls()) != 0 {
		t.Fatal("ran commands after cancellation")
	}
}

func TestDockerInspectTemplateWithNoHealthCheck(t *testing.T) {
	data := map[string]any{
		"Id": strings.Repeat("a", 64), "Name": "/app",
		"Config": map[string]any{"Image": "example", "Labels": map[string]string{}},
		"State": map[string]any{"Status": "running", "Error": "", "ExitCode": 0,
			"StartedAt": "2026-11-20T05:04:35.612936585Z"},
	}
	format := template.Must(template.New("inspect").Option("missingkey=error").Funcs(template.FuncMap{
		"json": func(value any) string {
			b, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		},
	}).Parse(inspectContainer))
	var output bytes.Buffer
	if err := format.Execute(&output, data); err != nil {
		t.Fatal(err)
	}
	var got Container
	if err := json.Unmarshal(output.Bytes(), &got); err != nil || !got.Active() || got.State.Health != nil {
		t.Fatalf("container = %+v, error = %v", got, err)
	}
}

func TestContainerStoppedHealthAndExitCodes(t *testing.T) {
	unhealthy := &struct{ Status string }{Status: "unhealthy"}
	for _, tc := range []struct {
		name     string
		status   string
		exit     int
		oom      bool
		health   *struct{ Status string }
		wantErr  bool
		wantProb bool
	}{
		{name: "stopped clean 0 with stale unhealthy", status: "exited", exit: 0, health: unhealthy, wantErr: false, wantProb: false},
		{name: "stopped SIGTERM 143", status: "exited", exit: 143, wantErr: false, wantProb: false},
		{name: "stopped SIGINT 130 with stale unhealthy", status: "exited", exit: 130, health: unhealthy, wantErr: false, wantProb: false},
		{name: "stopped SIGKILL 137 without OOM", status: "exited", exit: 137, wantErr: false, wantProb: false},
		{name: "stopped SIGKILL 137 with OOMKilled", status: "exited", exit: 137, oom: true, wantErr: true, wantProb: true},
		{name: "stopped OOMKilled even on 143", status: "exited", exit: 143, oom: true, wantErr: true, wantProb: true},
		{name: "stopped crash exit 1", status: "exited", exit: 1, wantErr: true, wantProb: true},
		{name: "running unhealthy", status: "running", exit: 0, health: unhealthy, wantErr: false, wantProb: true},
		{name: "running healthy", status: "running", exit: 0, health: &struct{ Status string }{Status: "healthy"}, wantErr: false, wantProb: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Container
			c.State.Status = tc.status
			c.State.ExitCode = tc.exit
			c.State.OOMKilled = tc.oom
			c.State.Health = tc.health
			if got := c.ExitedError(); got != tc.wantErr {
				t.Errorf("ExitedError() = %v, want %v", got, tc.wantErr)
			}
			if got := c.Problem(); got != tc.wantProb {
				t.Errorf("Problem() = %v, want %v", got, tc.wantProb)
			}
		})
	}
}

func TestFirstPublishedURL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw      string
		wantURL  string
		wantPort string
	}{
		{"", "", ""},
		{"80/tcp", "", ""},
		{"0.0.0.0:8080->80/tcp", "http://127.0.0.1:8080", "8080"},
		{"[::]:3000->3000/tcp", "http://127.0.0.1:3000", "3000"},
		{"127.0.0.1:5353->53/udp, 127.0.0.1:8443->443/tcp", "http://127.0.0.1:8443", "8443"},
		{"192.168.1.5:9000->9000/tcp", "http://192.168.1.5:9000", "9000"},
	} {
		gotURL, gotPort := FirstPublishedURL(tc.raw)
		if gotURL != tc.wantURL || gotPort != tc.wantPort {
			t.Errorf("FirstPublishedURL(%q) = (%q, %q), want (%q, %q)", tc.raw, gotURL, gotPort, tc.wantURL, tc.wantPort)
		}
	}
}
