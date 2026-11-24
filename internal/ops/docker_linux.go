package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// engineStart are the steps that start e on Linux through systemd:
// Docker Engine's and Kiri's service is the system's, Docker Desktop's
// and Podman's socket the user's.
func engineStart(e Engine) []Step {
	switch e.Name {
	case "Colima":
		return []Step{{Argv: []string{e.Path, "start"}}}
	case "Docker Engine":
		return []Step{{Argv: []string{"systemctl", "start", "docker"}, Admin: true, Timeout: 2 * time.Minute}}
	case "Kiri":
		return []Step{{Argv: []string{"systemctl", "start", "kiri"}, Admin: true, Timeout: 2 * time.Minute}}
	case "Docker Desktop":
		return []Step{{Argv: []string{"systemctl", "--user", "start", "docker-desktop"}, Timeout: 2 * time.Minute}}
	case "Podman":
		return []Step{{Argv: []string{"systemctl", "--user", "start", "podman.socket"}, Timeout: time.Minute}}
	}
	return nil
}

// dockerServiceStep builds the systemd command for Docker Engine lifecycle and
// autostart actions. Stopping also stops docker.socket when present so the
// next probe does not immediately socket-activate docker.service again.
// Enabling or disabling never passes --now so running state stays unchanged.
func dockerServiceStep(action DockerServiceAction, svc inventory.DockerServiceState) (Step, bool) {
	switch action {
	case DockerServiceStart:
		return Step{Argv: []string{"systemctl", "start", svc.Unit}, Admin: true, Timeout: 2 * time.Minute}, true
	case DockerServiceStop:
		argv := []string{"systemctl", "stop"}
		if svc.SocketUnit != "" {
			argv = append(argv, svc.SocketUnit)
		}
		argv = append(argv, svc.Unit)
		return Step{Argv: argv, Admin: true, Timeout: 2 * time.Minute}, true
	case DockerServiceEnable, DockerServiceDisable:
		verb := "disable"
		if action == DockerServiceEnable {
			verb = "enable"
		}
		argv := []string{"systemctl", verb, svc.Unit}
		if svc.SocketUnit != "" {
			argv = append(argv, svc.SocketUnit)
		}
		return Step{Argv: argv, Admin: true, Timeout: time.Minute}, true
	}
	return Step{}, false
}
