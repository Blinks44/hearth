package ops

import (
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/locales"
)

// stopProcess ends the process: taskkill without /f only asks windows to
// close, and a console program such as node has none.
func stopProcess(pid string) Step {
	return Step{Argv: []string{"taskkill.exe", "/pid", pid, "/f"}, Timeout: 30 * time.Second}
}

func processWhy() string { return locales.ProcessWhyWindows() }

func systemDirs() []string {
	root, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		root = `C:\Windows`
	}
	return []string{filepath.Clean(root) + `\`}
}
