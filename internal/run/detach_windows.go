package run

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
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

// setCancelGroup terminates the process tree on Windows using taskkill /T /F.
func setCancelGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		exe := "taskkill.exe"
		if root := os.Getenv("SystemRoot"); root != "" {
			exe = filepath.Join(root, "System32", "taskkill.exe")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		kill := exec.CommandContext(ctx, exe, "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		detach(kill)
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}

func killGroupHard(int) {}
