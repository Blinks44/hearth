package detect

import "path/filepath"

// xcodeSelectLink leads to the developer directory xcode-select chose,
// when the Command Line Tools or Xcode are installed; a variable, for
// tests.
var xcodeSelectLink = "/var/db/xcode_select_link"

// pythonStub reports macOS's /usr/bin/python3 when the Command Line Tools
// are not installed: it then offers to install them instead of running.
func pythonStub(path, _ string) bool {
	if path != "/usr/bin/python3" {
		return false
	}
	_, err := filepath.EvalSymlinks(xcodeSelectLink)
	return err != nil
}
