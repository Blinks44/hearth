package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const dockerSuffix = ""
const dockerDefaultEndpoint = "unix:///var/run/docker.sock"

func dockerLocations(env sysenv.Env, home string) ([]inventory.DockerEngine, []string) {
	app := func(name string) string {
		if home != "" {
			return dockerApp(filepath.Join(home, "Applications", name+".app"), "/Applications/"+name+".app")
		}
		return dockerApp("/Applications/" + name + ".app")
	}
	desktop, orb, podman, rancher := app("Docker"), app("OrbStack"), app("Podman Desktop"), app("Rancher Desktop")
	engines := []inventory.DockerEngine{{Name: "OrbStack", Path: orb}, {Name: "Colima", Path: dockerCommand(env, "colima")}, {Name: "Docker Desktop", Path: desktop}, {Name: "Podman", Path: dockerCommand(env, "podman")}, {Name: "Podman Desktop", Path: podman}, {Name: "Rancher Desktop", Path: rancher}}
	var paths []string
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "bin", "docker"), filepath.Join(home, ".orbstack", "bin", "docker"), filepath.Join(home, ".rd", "bin", "docker"))
	}
	if desktop != "" {
		paths = append(paths, filepath.Join(desktop, "Contents", "Resources", "bin", "docker"))
	}
	return engines, paths
}

// dockerJoined: the docker group is Linux's.
func dockerJoined(string) bool { return false }
