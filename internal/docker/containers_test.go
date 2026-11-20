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
