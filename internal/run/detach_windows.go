package run

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps a console program from opening a console window
// when Hearth, a GUI app, starts it.
const createNoWindow = 0x08000000

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// detachGUI leaves window creation flags untouched so a launched GUI editor
// or terminal opens visibly instead of hidden.
func detachGUI(*exec.Cmd) {}
