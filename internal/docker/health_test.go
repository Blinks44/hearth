package docker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadDockerHealth(t *testing.T) {
	id := strings.Repeat("a", 64)
	image := "sha256:" + strings.Repeat("b", 64)
	for _, tc := range []struct {
		name, imageArch, stats string
		used                   int64
		emulated               bool
	}{
		{"running foreign image", "amd64", "6.5GiB / 8GiB", 13 << 29, true},
		{"native architecture aliases", "arm64", "2.5MiB / 8GiB", 5 << 19, false},
		{"unknown sample", "amd64", "--", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r run.Fake
			r.Stdout(fmt.Sprintf(`{"ID":%q}`, id), colima.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
			r.Stdout(fmt.Sprintf(`{"ID":%q,"Name":"legacy-api","ImageID":%q,"State":{"Status":"running"}}`, id, image),
				colima.Command(nil, "container", "inspect", "--format", inspectContainer, "--", id).Args...)
			r.Stdout(fmt.Sprintf(`{"ID":%q,"Architecture":%q}`, image, tc.imageArch),
				colima.Command(nil, "image", "inspect", "--format", inspectImage, "--", image).Args...)
			r.Stdout(fmt.Sprintf(`{"ID":%q,"MemUsage":%q}`, id, tc.stats),
				colima.Command(nil, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}").Args...)
			h := ReadHealth(t.Context(), &r, nil, colima, Engine{Arch: "aarch64", Running: 1, Memory: 8 << 30})
			if h.ContainerMemory != tc.used || (h.Memory != 0) != (tc.used != 0) || (len(h.Emulated) == 1) != tc.emulated {
				t.Fatalf("unexpected health reading: %+v", h)
			}
			if tc.emulated && (h.Emulated[0].ID != id || h.Emulated[0].EngineArch != "arm64") {
				t.Errorf("wrong emulated container: %+v", h.Emulated[0])
			}
		})
	}
}
