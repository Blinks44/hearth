package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// The Store's python.exe is a stub until a Store Python's package is
// beside it.
func TestPythonStubWindows(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "Microsoft", "WindowsApps")
	path := filepath.Join(apps, "python.exe")
	writeFile(t, path, "")
	if !pythonStub(path, path) {
		t.Error("the Store's alias is not a stub")
	}
	writeFile(t, filepath.Join(apps, "PythonSoftwareFoundation.Python.3.13_qbz5n2kfra8p0", "python.exe"), "")
	if pythonStub(path, path) {
		t.Error("a Store Python's alias is a stub")
	}
}

// The install manager's python.exe in WindowsApps is its alias, not the
// Store's stub.
func TestPythonManagerAlias(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "Microsoft", "WindowsApps")
	path := filepath.Join(apps, "python.exe")
	writeFile(t, path, "")
	if pythonManagerAlias(path) {
		t.Error("the Store's stub is the manager's alias")
	}
	writeFile(t, filepath.Join(apps, "PythonSoftwareFoundation.PythonManager_qbz5n2kfra8p0", "python.exe"), "")
	if !pythonManagerAlias(path) {
		t.Error("the manager's alias is not found")
	}
}

func TestPythonManagerTag(t *testing.T) {
	appData := t.TempDir()
	env := sysenv.Env{"APPDATA=" + appData}
	if got := pythonManagerTag(env); got != "3" {
		t.Errorf("no configuration: got %q", got)
	}
	writeFile(t, filepath.Join(appData, "Python", "pymanager.json"), `{"default_tag": "3.13-arm64"}`)
	if got := pythonManagerTag(env); got != "3.13" {
		t.Errorf("configured: got %q", got)
	}
	if got := pythonManagerTag(append(env, "PYTHON_MANAGER_DEFAULT=3.12")); got != "3.12" {
		t.Errorf("variable: got %q", got)
	}
}
