package detect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

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
		{Name: "Kiri", Path: dockerCommand(env, "kiri")},
	}
	if path := dockerCommand(env, "colima"); path != "" {
		engines = append(engines, inventory.DockerEngine{Name: "Colima", Path: path})
	}
	// Kiri's package keeps its docker CLI out of PATH when another owns
	// /usr/bin/docker.
	paths := []string{"/usr/libexec/kiri/docker"}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "bin", "docker"), filepath.Join(home, ".rd", "bin", "docker"))
	}
	return engines, paths
}

// groupFile lists the system's groups; a variable, for tests.
var groupFile = "/etc/group"

// dockerJoined reports that user is in the docker group, as the group
// file has it, though the socket refused them: the session started
// before they joined, and a new sign-in gives access.
func dockerJoined(user string) bool {
	if user == "" {
		return false
	}
	data, err := os.ReadFile(groupFile)
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(data)) {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) == 4 && fields[0] == "docker" {
			return slices.Contains(strings.Split(fields[3], ","), user)
		}
	}
	return false
}
