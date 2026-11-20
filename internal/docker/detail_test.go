package docker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadContainerDetailMasksEnvironment(t *testing.T) {
	id := strings.Repeat("a", 64)
	var r run.Fake
	inspect := fmt.Sprintf(`{"Id":%q,"Image":"sha256:image","Platform":"linux","Config":{
		"Image":"shop:latest","Env":["KEY=test-value=with-equals","EMPTY=","EMPTY="],
		"Labels":{"com.docker.compose.project":"shop"},"ExposedPorts":{"90/tcp":{}}
	},"NetworkSettings":{"Ports":{"80/tcp":[{"HostIp":"::","HostPort":"8080"}]},
		"Networks":{"shop_default":{"IPAddress":"172.19.0.2"}}},
	"Mounts":[{"Type":"volume","Name":"shop_data","Destination":"/data","RW":false}],
	"State":{"Health":{"Log":[{"ExitCode":1,"Output":"unhealthy"}]}}}`, id)
	r.Stdout(inspect, colima.Command(nil, "container", "inspect", "--format", "{{json .}}", "--", id).Args...)
	r.Stdout(`{"Os":"linux","Architecture":"arm64","RepoDigests":["shop@sha256:digest"]}`,
		colima.Command(nil, "image", "inspect", "--format", "{{json .}}", "--", "sha256:image").Args...)
	r.Stdout(`["KEY=test-value=with-equals","EMPTY="]`, colima.Command(nil, "container", "inspect", "--format", "{{json .Config.Env}}", "--", id).Args...)
	detail, err := ReadContainerDetail(t.Context(), &r, nil, colima, id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(detail.EnvKeys, ",") != "EMPTY,KEY" || strings.Contains(detail.Inspect, "test-value") ||
		!strings.Contains(detail.Inspect, "KEY=••••••••") {
		t.Fatal("Inspect did not mask the environment")
	}
	if detail.Platform != "linux/arm64" || len(detail.Digests) != 1 || detail.Networks["shop_default"] != "172.19.0.2" ||
		len(detail.Health) != 1 || len(detail.Mounts) != 1 || detail.Mounts[0].RW {
		t.Fatalf("incomplete details: %+v", detail)
	}
	if bindings := detail.Ports["80/tcp"]; len(bindings) != 1 || bindings[0].URL("80/tcp") != "http://[::1]:8080" {
		t.Fatalf("port binding: %+v", bindings)
	}
	value, err := ReadContainerEnv(t.Context(), &r, nil, colima, id, "KEY")
	if err != nil || string(value) != "test-value=with-equals" {
		t.Fatalf("read one key: %q, %v", value, err)
	}
}
