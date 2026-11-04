package detect

import (
	"path/filepath"
	"testing"
)

// /usr/bin/python3 is a stub only while xcode-select points nowhere.
func TestPythonStubDarwin(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "xcode_select_link")
	orig := xcodeSelectLink
	xcodeSelectLink = link
	t.Cleanup(func() { xcodeSelectLink = orig })
	if !pythonStub("/usr/bin/python3", "/usr/bin/python3") {
		t.Error("without the Command Line Tools /usr/bin/python3 is not a stub")
	}
	symlink(t, dir, link)
	if pythonStub("/usr/bin/python3", "/usr/bin/python3") {
		t.Error("with the Command Line Tools /usr/bin/python3 is a stub")
	}
	if pythonStub("/opt/homebrew/bin/python3", "") {
		t.Error("Homebrew's python3 is a stub")
	}
}
