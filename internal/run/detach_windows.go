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
