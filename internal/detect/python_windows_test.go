package detect

import (
	"path/filepath"
	"testing"
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
