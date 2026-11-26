package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// engineStart are the steps that start e on macOS: an app opens, Colima
// and Podman start their virtual machine.
func engineStart(e Engine) []Step {
	switch e.Name {
	case "OrbStack", "Docker Desktop", "Rancher Desktop", "Kiri":
		return []Step{{Argv: []string{"/usr/bin/open", "-a", e.Path}, Timeout: time.Minute}}
	case "Colima":
		return []Step{{Argv: []string{e.Path, "start"}, Disclaim: true}}
	case "Podman":
		return []Step{{Argv: []string{e.Path, "machine", "start"}, Disclaim: true}}
	}
	return nil
}

func dockerServiceStep(DockerServiceAction, inventory.DockerServiceState) (Step, bool) {
	return Step{}, false
}
