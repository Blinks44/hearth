package ops

import "time"

// startScript starts the program in HEARTH_PROGRAM and does not wait for
// it: a desktop app keeps running. The script is fixed: the path comes in
// the environment.
const startScript = `Start-Process -FilePath $env:HEARTH_PROGRAM`

// engineStart are the steps that start e on Windows: an app starts, the
// Podman command starts its virtual machine.
func engineStart(e Engine) []Step {
	switch e.Name {
	case "Docker Desktop", "Rancher Desktop", "Kiri":
		return []Step{{
			Argv:    []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", startScript},
			Env:     []string{"HEARTH_PROGRAM=" + e.Path},
			Runs:    []string{e.Path},
			Timeout: time.Minute,
		}}
	case "Podman":
		return []Step{{Argv: []string{e.Path, "machine", "start"}}}
	}
	return nil
}
