package ops

import "time"

// systemService is the step that starts or stops the systemd unit name,
// as the administrator. It does not enable or disable the unit: whether
// it starts with the system stays as it was.
func systemService(action ServiceAction, name string) (Step, bool) {
	verb := [...]string{"start", "stop"}[action]
	return Step{Argv: []string{"systemctl", verb, name}, Admin: true, Timeout: 2 * time.Minute}, true
}
