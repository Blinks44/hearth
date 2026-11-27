//go:build !windows

package run

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a session of its own, without a controlling
// terminal. Hearth started from a terminal in the background would
// otherwise stop: the interactive shell that reads the terminal's
// environment asks for the terminal, and the system stops its whole
// process group, Hearth with it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// detachGUI starts a GUI terminal or editor in its own session so it outlives
// Hearth's command context and does not attach to Hearth's controlling TTY.
func detachGUI(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// setCancelGroup configures cmd.Cancel to signal the child's entire process
// group (created by detach's Setsid: true) with SIGTERM so child workers
// (such as Vite/Next/Air) exit cleanly rather than staying orphaned.
func setCancelGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			return cmd.Process.Kill()
		}
		return nil
	}
}

func killGroupHard(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
