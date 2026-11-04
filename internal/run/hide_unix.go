//go:build !windows

package run

import "os/exec"

// hideWindow does nothing outside Windows: no console opens there.
func hideWindow(*exec.Cmd) {}
