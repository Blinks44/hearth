package ops

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// ServiceAction is what to do with a database's service.
type ServiceAction int

const (
	ServiceStart ServiceAction = iota
	ServiceStop
)

// serviceName is what a service's name may hold: Homebrew's formulas,
// systemd's units and Windows' services, as MSSQL$SQLEXPRESS.
var serviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@$+-]*$`)

// ServicePlan returns the plan that does action to db's service through
// the manager that keeps it: Homebrew's services, or the system's. ok is
// false when db has no service Hearth changes.
func ServicePlan(action ServiceAction, db inventory.Database, detect Detect) (p Plan, ok bool) {
	name := db.Service
	if name == "" || db.Source == "docker" || !serviceName.MatchString(name) {
		return Plan{}, false
	}
	verb := [...]string{"start", "stop"}[action]
	var step Step
	switch db.Source {
	case "brew":
		step = Step{Argv: []string{"brew", "services", verb, name}, Timeout: 2 * time.Minute}
		p.Why = locales.ServiceWhyBrew()
		if action == ServiceStop {
			p.Notes = []Note{{Title: locales.ServiceBrewLogin(), Text: locales.ServiceBrewLoginText(), Tag: "brew services"}}
		}
	default:
		// Any other service is the system's, also of a server found as
		// an installer's copy, as EDB's PostgreSQL on Windows.
		if step, ok = systemService(action, name); !ok {
			return Plan{}, false
		}
		p.Why = locales.ServiceWhySystem()
		p.Notes = []Note{AdminNote()}
	}
	p.Steps = []Step{step}
	if action == ServiceStart {
		p.Title = locales.ServiceStartTitle(name)
		p.Changes = []string{locales.ServiceStartChange(name)}
	} else {
		p.Title = locales.ServiceStopTitle(name)
		p.Changes = []string{locales.ServiceStopChange(name)}
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			return checkService(action, db, st)
		}
	}
	return p, true
}

// checkService says whether st, the databases found after the plan ran,
// show action done to db's service.
func checkService(action ServiceAction, db inventory.Database, st inventory.ToolState) (string, error) {
	want := [...]string{"running", "stopped"}[action]
	if st.Databases != nil {
		for _, s := range st.Databases.Servers {
			if s.Service == db.Service && s.Source == db.Source && s.Status == want {
				if action == ServiceStart {
					return locales.ServiceCheckRunning(db.Service), nil
				}
				return locales.ServiceCheckStopped(db.Service), nil
			}
		}
	}
	if action == ServiceStart {
		return "", errors.New(locales.ServiceCheckNotRunning(db.Service))
	}
	return "", errors.New(locales.ServiceCheckNotStopped(db.Service))
}

// AutostartPlan returns the plan that makes db's service start with the
// system, or at the user's login for Homebrew's, when on, and stop doing
// so when not. It leaves the service running or stopped as it is, but
// for Homebrew's, which start as they are set to start. ok is false when
// db has no service Hearth changes.
func AutostartPlan(on bool, db inventory.Database, detect Detect) (p Plan, ok bool) {
	name := db.Service
	if name == "" || db.Source == "docker" || !serviceName.MatchString(name) {
		return Plan{}, false
	}
	switch {
	case db.Source == "brew" && on:
		// brew services start sets the login item and starts it now. One
		// running without a login item, as brew services run leaves it,
		// start leaves as it is, and restart runs it again the same way:
		// it stops first.
		start := Step{Argv: []string{"brew", "services", "start", name}, Timeout: 2 * time.Minute}
		if db.Status == "running" {
			p.Steps = []Step{{Argv: []string{"brew", "services", "stop", name}, Timeout: 2 * time.Minute}, start}
		} else {
			p.Steps = []Step{start}
			p.Changes = append(p.Changes, locales.ServiceStartChange(name))
		}
		p.Why = locales.ServiceWhyBrew()
	case db.Source == "brew":
		// stop takes it out of the login items; run starts it again
		// without one, for now.
		p.Steps = []Step{{Argv: []string{"brew", "services", "stop", name}, Timeout: 2 * time.Minute}}
		if db.Status == "running" {
			p.Steps = append(p.Steps, Step{Argv: []string{"brew", "services", "run", name}, Timeout: 2 * time.Minute})
		}
		p.Why = locales.ServiceWhyBrew()
	default:
		step, found := systemAutostart(on, name)
		if !found {
			return Plan{}, false
		}
		p.Steps = []Step{step}
		p.Why = locales.ServiceWhySystem()
		p.Notes = []Note{AdminNote()}
	}
	if on {
		p.Title = locales.AutostartOnTitle(name)
		p.Changes = append([]string{locales.AutostartOnChange(name)}, p.Changes...)
	} else {
		p.Title = locales.AutostartOffTitle(name)
		p.Changes = append([]string{locales.AutostartOffChange(name)}, p.Changes...)
	}
	if detect != nil {
		want := map[bool]string{true: "on", false: "off"}[on]
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Databases != nil && slices.ContainsFunc(st.Databases.Servers, func(s inventory.Database) bool {
				return s.Service == name && s.Source == db.Source && s.Autostart == want
			}) {
				return p.Changes[0], nil
			}
			return "", errors.New(locales.AutostartCheckFailed(name))
		}
	}
	return p, true
}
