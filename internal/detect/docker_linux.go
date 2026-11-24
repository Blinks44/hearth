package detect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
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

// detectDockerService reads the systemd state of docker.service (and
// docker.socket when installed) so Hearth can display and manage native
// Docker Engine start, stop, and boot autostart on Linux.
func detectDockerService(ctx context.Context, r run.Runner, env sysenv.Env, engines []inventory.DockerEngine) *inventory.DockerServiceState {
	hasEngine := slices.ContainsFunc(engines, func(e inventory.DockerEngine) bool {
		return e.Name == "Docker Engine" && e.Path != ""
	})
	if !hasEngine {
		return nil
	}
	exe := dockerCommand(env, "systemctl")
	if exe == "" {
		return nil
	}
	res, err := r.Run(ctx, run.Cmd{
		Args:    []string{exe, "show", "--property=Id,LoadState,ActiveState,UnitFileState", "docker.service", "docker.socket"},
		Env:     env,
		Timeout: 3 * time.Second,
	})
	if err != nil || res.ExitCode != 0 || res.Truncated {
		return nil
	}
	return parseDockerServiceShow(string(res.Stdout))
}

func parseDockerServiceShow(output string) *inventory.DockerServiceState {
	type unitProps struct {
		load, active, fileState string
	}
	units := map[string]unitProps{}
	var id string
	var cur unitProps
	flush := func() {
		if id != "" {
			units[id] = cur
		}
		id, cur = "", unitProps{}
	}
	for line := range strings.Lines(output) {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "Id":
			id = v
		case "LoadState":
			cur.load = v
		case "ActiveState":
			cur.active = v
		case "UnitFileState":
			cur.fileState = v
		}
	}
	flush()

	svc, ok := units["docker.service"]
	if !ok || svc.load != "loaded" {
		return nil
	}
	out := &inventory.DockerServiceState{Unit: "docker.service"}
	switch svc.active {
	case "active":
		out.Status = "running"
	case "inactive", "deactivating":
		out.Status = "stopped"
	case "failed":
		out.Status = "error"
	}
	sock, hasSock := units["docker.socket"]
	hasSock = hasSock && sock.load == "loaded"
	if hasSock {
		out.SocketUnit = "docker.socket"
	}
	enabled := func(s string) bool { return s == "enabled" || s == "enabled-runtime" }
	disabled := func(s string) bool { return s == "disabled" }
	switch {
	case enabled(svc.fileState) || (hasSock && enabled(sock.fileState)):
		out.Autostart = "on"
	case disabled(svc.fileState) && (!hasSock || disabled(sock.fileState) || sock.fileState == "static"):
		out.Autostart = "off"
	}
	return out
}
