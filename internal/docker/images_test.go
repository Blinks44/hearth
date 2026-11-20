package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"text/template"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadDockerImages(t *testing.T) {
	tagged := "sha256:" + strings.Repeat("a", 64)
	untagged := "sha256:" + strings.Repeat("b", 64)
	container := strings.Repeat("c", 64)
	var r run.Fake
	r.Stdout(strings.Repeat(`{"ID":"`+tagged+"\"}\n", 2),
		colima.Command(nil, "image", "ls", "--no-trunc", "--filter", "dangling=false", "--format", "{{json .}}").Args...)
	r.Stdout(`{"ID":"`+untagged+"\"}\n",
		colima.Command(nil, "image", "ls", "--no-trunc", "--filter", "dangling=true", "--format", "{{json .}}").Args...)
	r.Stdout(string(fixture(t, "images-inspect.jsonl")),
		colima.Command(nil, "image", "inspect", "--format", inspectImage, "--", tagged, untagged).Args...)
	r.Stdout(`{"ID":"`+container+`"}`, colima.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
	r.Stdout(`{"ID":"`+container+`","Name":"/legacy","Image":"different:tag","ImageID":"`+tagged+`","State":{"Status":"exited"}}`,
		colima.Command(nil, "container", "inspect", "--format", inspectContainer, "--", container).Args...)
	items, err := ReadImages(t.Context(), &r, []string{"DOCKER_CONTEXT=other"}, colima)
	if err != nil || len(items) != 2 {
		t.Fatalf("images = %+v, err = %v", items, err)
	}
	legacy, dangling := items[0], items[1]
	if legacy.Name() != "ghcr.io/demo/legacy:2.3" || legacy.Size != 123456 || legacy.Created.IsZero() ||
		!legacy.Emulated("aarch64") || legacy.Emulated("x86_64") || len(legacy.Users) != 1 ||
		legacy.Users[0] != (ImageUser{Name: "legacy"}) || legacy.Dangling() {
		t.Fatalf("legacy image = %+v", legacy)
	}
	if dangling.Name() != untagged || !dangling.Dangling() || dangling.Emulated("aarch64") {
		t.Fatalf("dangling image = %+v", dangling)
	}
	for _, cmd := range r.Calls() {
		if cmd.Args[2] != colima.Endpoint || cmd.Env[len(cmd.Env)-1] != "DOCKER_CONTEXT=" {
			t.Fatalf("unpinned command: %+v", cmd)
		}
	}
}

func TestDockerImageWatchCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var r run.Fake
	WatchImages(ctx, &r, nil, colima, func([]Image, error) { t.Error("read after cancellation") })
	if len(r.Calls()) != 0 {
		t.Fatal("ran commands after cancellation")
	}
}

func TestDockerImageInspectWithoutCreated(t *testing.T) {
	format := template.Must(template.New("inspect").Option("missingkey=error").Funcs(template.FuncMap{
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	}).Parse(inspectImage))
	for _, data := range []map[string]any{{"Id": "image", "Architecture": "arm64"}, {"Id": "image", "Created": ""}} {
		var row bytes.Buffer
		if err := format.Execute(&row, data); err != nil {
			t.Fatal(err)
		}
		var item Image
		if err := json.Unmarshal(row.Bytes(), &item); err != nil || !item.Created.IsZero() || item.ID != "image" {
			t.Fatalf("inspect row %s: %+v, %v", row.String(), item, err)
		}
	}
}
