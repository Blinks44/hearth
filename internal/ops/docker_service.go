package ops

import (
	"context"
	"errors"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// DockerServiceAction is a lifecycle or autostart change for the native Linux
// Docker Engine systemd service.
type DockerServiceAction string

const (
	DockerServiceStart   DockerServiceAction = "start"
	DockerServiceStop    DockerServiceAction = "stop"
	DockerServiceEnable  DockerServiceAction = "enable"
	DockerServiceDisable DockerServiceAction = "disable"
)

// DockerServicePlan returns a confirmed plan that starts, stops, or toggles
// boot autostart for the native Linux Docker Engine systemd service.
// Toggling autostart never starts or stops the running service.
func DockerServicePlan(action DockerServiceAction, svc inventory.DockerServiceState, detect Detect) (Plan, bool) {
	if svc.Unit == "" || !serviceName.MatchString(svc.Unit) {
		return Plan{}, false
	}
	if svc.SocketUnit != "" && !serviceName.MatchString(svc.SocketUnit) {
		return Plan{}, false
	}
	step, ok := dockerServiceStep(action, svc)
	if !ok {
		return Plan{}, false
	}
	const name = "Docker Engine"
	p := Plan{
		Why:   locales.ServiceWhySystem(),
		Steps: []Step{step},
		Notes: []Note{AdminNote()},
	}
	switch action {
	case DockerServiceStart:
		p.Title = locales.ServiceStartTitle(name)
		p.Changes = []string{locales.ServiceStartChange(svc.Unit)}
	case DockerServiceStop:
		p.Title = locales.ServiceStopTitle(name)
		p.Changes = []string{locales.ServiceStopChange(svc.Unit)}
	case DockerServiceEnable:
		p.Title = locales.AutostartOnTitle(name)
		p.Changes = []string{locales.AutostartOnChange(name)}
	case DockerServiceDisable:
		p.Title = locales.AutostartOffTitle(name)
		p.Changes = []string{locales.AutostartOffChange(name)}
	default:
		return Plan{}, false
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			var got inventory.DockerServiceState
			if st.Docker != nil && st.Docker.Service != nil {
				got = *st.Docker.Service
			}
			switch action {
			case DockerServiceStart:
				if got.Status == "running" {
					return locales.ServiceCheckRunning(name), nil
				}
				return "", errors.New(locales.ServiceCheckNotRunning(name))
			case DockerServiceStop:
				if got.Status == "stopped" {
					return locales.ServiceCheckStopped(name), nil
				}
				return "", errors.New(locales.ServiceCheckNotStopped(name))
			case DockerServiceEnable:
				if got.Autostart == "on" {
					return locales.AutostartOnChange(name), nil
				}
				return "", errors.New(locales.AutostartCheckFailed(name))
			case DockerServiceDisable:
				if got.Autostart == "off" {
					return locales.AutostartOffChange(name), nil
				}
				return "", errors.New(locales.AutostartCheckFailed(name))
			}
			return "", nil
		}
	}
	return p, true
}
