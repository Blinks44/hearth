package ops

import "time"

// systemService is the step that starts or stops the Windows service
// name as the administrator, through UAC. net waits for the service to
// get there; /y stops the services that depend on it too, which net
// would otherwise ask about.
func systemService(action ServiceAction, name string) ([]Step, bool) {
	if action == ServiceRestart {
		s := runAs("powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", "Restart-Service -Name '" + name + "' -Force"})
		s.Timeout = 2 * time.Minute
		return []Step{s}, true
	}
	args := []string{"start", name}
	if action == ServiceStop {
		args = []string{"stop", name, "/y"}
	}
	s := runAs("net.exe", args)
	s.Timeout = 2 * time.Minute
	return []Step{s}, true
}

// systemAutostart is the step that sets the Windows service name to
// start with the system, or when asked, as the administrator through UAC.
// sc takes each option's value as an argument of its own after "start=".
func systemAutostart(on bool, name string) (Step, bool) {
	start := "demand"
	if on {
		start = "auto"
	}
	s := runAs("sc.exe", []string{"config", name, "start=", start})
	s.Timeout = time.Minute
	return s, true
}
