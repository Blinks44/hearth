package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// systemService is the step that starts or stops the systemd unit name,
// as the administrator. It does not enable or disable the unit: whether
// it starts with the system stays as it was.
func systemService(action ServiceAction, name string) ([]Step, bool) {
	verb := [...]string{"start", "stop", "restart"}[action]
	return []Step{{Argv: []string{"systemctl", verb, name}, Admin: true, Timeout: 2 * time.Minute}}, true
}

// systemAutostart is the step that enables or disables the unit that
// starts the service name with the system, as the administrator.
func systemAutostart(on bool, name string) (Step, bool) {
	verb := "disable"
	if on {
		verb = "enable"
	}
	return Step{Argv: []string{"systemctl", verb, inventory.SystemdAutostartUnit(name)}, Admin: true, Timeout: time.Minute}, true
}
