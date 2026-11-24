package ops

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// ProcessPlan returns the plan that stops the process listening at l. ok
// is false for a process Hearth does not stop: Hearth itself, another
// user's, or one of the system's own programs.
func ProcessPlan(l inventory.DatabaseListener, detect Detect) (p Plan, ok bool) {
	if !l.Own || l.Path == "" || SystemProgram(l.Path) {
		return Plan{}, false
	}
	pid, err := strconv.ParseUint(l.PID, 10, 32)
	if err != nil || pid == uint64(os.Getpid()) {
		return Plan{}, false
	}
	name := l.Process
	p.Title = locales.ProcessStopTitle(name, l.Port)
	p.Steps = []Step{stopProcess(l.PID)}
	p.Why = processWhy()
	p.Changes = []string{locales.ProcessStopChange(l.Port)}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			// A process asked to quit may take a moment to.
			for range 10 {
				st, err := detect(ctx)
				if err != nil {
					return "", err
				}
				if st.Databases == nil || !slices.ContainsFunc(st.Databases.Listeners, func(o inventory.DatabaseListener) bool {
					return o.PID == l.PID && o.Port == l.Port
				}) {
					return locales.ProcessStopChange(l.Port), nil
				}
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(time.Second):
				}
			}
			return "", errors.New(locales.ProcessCheckBusy(name, l.Port))
		}
	}
	return p, true
}

// SystemProgram reports that path is a program of the system's own,
// which Hearth does not stop.
func SystemProgram(path string) bool {
	for _, dir := range systemDirs() {
		if len(path) > len(dir) && strings.EqualFold(path[:len(dir)], dir) {
			return true
		}
	}
	return false
}
