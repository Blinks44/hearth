package detect

import (
	"path/filepath"
	"testing"
)

// /usr/bin/python3 is a stub only while there is no developer
// directory: none chosen with xcode-select, nor the Command Line Tools in
// their place, as Software Update installs them.
func TestPythonStubDarwin(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "xcode_select_link")
	origLink, origCLT := xcodeSelectLink, cltDir
	xcodeSelectLink, cltDir = link, filepath.Join(dir, "CommandLineTools")
	t.Cleanup(func() { xcodeSelectLink, cltDir = origLink, origCLT })
	if !pythonStub("/usr/bin/python3", "/usr/bin/python3") {
		t.Error("without the Command Line Tools /usr/bin/python3 is not a stub")
	}
	writeFile(t, filepath.Join(cltDir, "usr", "bin", "python3"), "")
	if pythonStub("/usr/bin/python3", "/usr/bin/python3") {
		t.Error("with the Command Line Tools and no link /usr/bin/python3 is a stub")
	}
	symlink(t, dir, link)
	if pythonStub("/usr/bin/python3", "/usr/bin/python3") {
		t.Error("with the Command Line Tools /usr/bin/python3 is a stub")
	}
	if pythonStub("/opt/homebrew/bin/python3", "") {
		t.Error("Homebrew's python3 is a stub")
	}
}
