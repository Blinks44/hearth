//go:build windows

package ops

// DockerDefaultSocketPlan is Unix-only; Windows engines use named pipes.
func DockerDefaultSocketPlan(contextName, endpoint string, detect Detect) (Plan, bool) {
	return Plan{}, false
}
