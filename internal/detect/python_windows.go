package detect

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	pythonExe = "python"
	// pythonBin is where python.exe lies in an installation's directory.
	pythonBin = "python.exe"
)

// pythonOSGlobs are where python.org's installer puts Pythons: for the
// user, or for everyone.
func pythonOSGlobs(env sysenv.Env) []string {
	var globs []string
	if dir := env.Get("LOCALAPPDATA"); dir != "" {
		globs = append(globs, filepath.Join(dir, "Programs", "Python", "Python3*", pythonBin))
	}
	pf := env.Get("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return append(globs, filepath.Join(pf, "Python3*", pythonBin))
}

// pythonStub reports Microsoft Store's stand-in python.exe in
// WindowsApps, which opens the Store: the alias of a Python the Store
// installed has its package's folder beside it.
func pythonStub(path, _ string) bool {
	dir := filepath.Dir(path)
	if !strings.EqualFold(filepath.Base(dir), "WindowsApps") {
		return false
	}
	entries, _ := os.ReadDir(dir) // unreadable: taken for the stub
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "PythonSoftwareFoundation.Python.") {
			return false
		}
	}
	return true
}
