package ops

import "time"

// systemService is the step that starts or stops the Windows service
// name as the administrator, through UAC. net waits for the service to
// get there; /y stops the services that depend on it too, which net
// would otherwise ask about.
func systemService(action ServiceAction, name string) (Step, bool) {
	args := []string{"start", name}
	if action == ServiceStop {
		args = []string{"stop", name, "/y"}
	}
	s := runAs("net.exe", args)
	s.Timeout = 2 * time.Minute
	return s, true
}
