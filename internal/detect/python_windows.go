package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	pythonExe = "python"
	// pythonBin is where python.exe lies in an installation's directory.
	pythonBin = "python.exe"
)

// pythonOSGlobs are where python.org's installers put Pythons: the
// install manager's py install, and the full installer for the user or
// for everyone.
func pythonOSGlobs(env sysenv.Env) []string {
	var globs []string
	if dir := env.Get("LOCALAPPDATA"); dir != "" {
		globs = append(globs,
			filepath.Join(dir, "Python", "pythoncore-*", pythonBin),
			filepath.Join(dir, "Programs", "Python", "Python3*", pythonBin))
	}
	pf := env.Get("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return append(globs, filepath.Join(pf, "Python3*", pythonBin))
}

// pythonManagerAlias reports the install manager's python.exe in
// WindowsApps: an alias of its package, which runs its default Python.
func pythonManagerAlias(path string) bool {
	dir := filepath.Dir(path)
	if !strings.EqualFold(filepath.Base(dir), "WindowsApps") {
		return false
	}
	entries, _ := os.ReadDir(dir)
	return slices.ContainsFunc(entries, func(e os.DirEntry) bool {
		return strings.HasPrefix(e.Name(), "PythonSoftwareFoundation.PythonManager_")
	})
}

// pythonManagerTag is the tag the install manager runs python for:
// PYTHON_MANAGER_DEFAULT, else default_tag of its configuration, else 3.
// The platform part, as in 3.14-arm64, is dropped.
func pythonManagerTag(env sysenv.Env) string {
	tag := env.Get("PYTHON_MANAGER_DEFAULT")
	if tag == "" {
		var config struct {
			DefaultTag string `json:"default_tag"`
		}
		if appData := env.Get("APPDATA"); appData != "" {
			_ = json.Unmarshal([]byte(readFile(filepath.Join(appData, "Python", "pymanager.json"))), &config) // missing or broken: the default
		}
		tag = config.DefaultTag
	}
	if tag == "" {
		return "3"
	}
	tag, _, _ = strings.Cut(tag, "-")
	return tag
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
