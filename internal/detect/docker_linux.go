package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const dockerSuffix = ""
const dockerDefaultEndpoint = "unix:///var/run/docker.sock"

func dockerLocations(env sysenv.Env, home string) ([]inventory.DockerEngine, []string) {
	engines := []inventory.DockerEngine{
		{Name: "Docker Engine", Path: dockerCommand(env, "dockerd")},
		{Name: "Podman", Path: dockerCommand(env, "podman")},
		{Name: "Docker Desktop", Path: dockerApp("/opt/docker-desktop")},
		{Name: "Rancher Desktop", Path: dockerCommand(env, "rancher-desktop")},
	}
	var paths []string
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "bin", "docker"), filepath.Join(home, ".rd", "bin", "docker"))
	}
	return engines, paths
}
