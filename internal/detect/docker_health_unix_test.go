//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerBrokenPluginLinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "exists"), "")
	for name, target := range map[string]string{"docker-buildx": "missing", "docker-compose": "exists", "unrelated": "missing"} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if got := brokenDockerPlugins(dir); !slices.Equal(got, []string{"docker-buildx"}) {
		t.Errorf("broken plugins = %q", got)
	}
}

func TestDockerDefaultUnixSocketAlias(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "actual")
	writeFile(t, sock, "")
	link := filepath.Join(dir, "default")
	var r run.Fake
	selected := inventory.DockerContext{Endpoint: "unix://" + sock, Status: "available"}
	if got := dockerSocketHealth(t.Context(), &r, nil, "docker", "unix://"+link, selected); got != "missing" {
		t.Errorf("missing: got %q", got)
	}
	if err := os.Symlink(sock, link); err != nil {
		t.Fatal(err)
	}
	if got := dockerSocketHealth(t.Context(), &r, nil, "docker", "unix://"+link, selected); got != "" || len(r.Calls()) != 0 {
		t.Errorf("alias: got %q, calls = %v", got, r.Calls())
	}
}
