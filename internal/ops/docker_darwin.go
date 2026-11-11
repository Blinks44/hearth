package ops

import "time"

// engineStart are the steps that start e on macOS: an app opens, Colima
// and Podman start their virtual machine.
func engineStart(e Engine) []Step {
	switch e.Name {
	case "OrbStack", "Docker Desktop", "Rancher Desktop":
		return []Step{{Argv: []string{"/usr/bin/open", "-a", e.Path}, Timeout: time.Minute}}
	case "Colima":
		return []Step{{Argv: []string{e.Path, "start"}}}
	case "Podman":
		return []Step{{Argv: []string{e.Path, "machine", "start"}}}
	}
	return nil
}
