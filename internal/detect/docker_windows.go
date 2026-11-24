package detect

import (
	"context"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const dockerSuffix = ".exe"
const dockerDefaultEndpoint = "npipe:////./pipe/docker_engine"

func dockerLocations(env sysenv.Env, home string) ([]inventory.DockerEngine, []string) {
	var desktop, podman, rancher, kiri string
	if pf := env.Get("ProgramFiles"); pf != "" {
		desktop = dockerApp(filepath.Join(pf, "Docker", "Docker", "Docker Desktop.exe"))
		podman = dockerApp(filepath.Join(pf, "RedHat", "Podman Desktop", "Podman Desktop.exe"))
		if podman == "" {
			podman = dockerApp(filepath.Join(pf, "Podman Desktop", "Podman Desktop.exe"))
		}
	}
	if local := env.Get("LOCALAPPDATA"); local != "" {
		rancher = dockerApp(filepath.Join(local, "Programs", "Rancher Desktop", "Rancher Desktop.exe"))
		kiri = dockerApp(filepath.Join(local, "Kiri", "kiri-win.exe"))
		if podman == "" {
			podman = dockerApp(filepath.Join(local, "Programs", "podman-desktop", "Podman Desktop.exe"))
		}
	}
	engines := []inventory.DockerEngine{{Name: "Docker Desktop", Path: desktop}, {Name: "Podman", Path: dockerCommand(env, "podman")}, {Name: "Podman Desktop", Path: podman}, {Name: "Rancher Desktop", Path: rancher}, {Name: "Kiri", Path: kiri}}
	var paths []string
	if desktop != "" {
		paths = append(paths, filepath.Join(filepath.Dir(desktop), "resources", "bin", "docker.exe"))
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "bin", "docker.exe"), filepath.Join(home, ".rd", "bin", "docker.exe"))
	}
	if kiri != "" {
		paths = append(paths, filepath.Join(filepath.Dir(kiri), "bin", "docker.exe"))
	}
	return engines, paths
}

// dockerJoined: the docker group is Linux's.
func dockerJoined(string) bool { return false }

func detectDockerService(context.Context, run.Runner, sysenv.Env, []inventory.DockerEngine) *inventory.DockerServiceState {
	return nil
}
