package ops

import "time"

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
