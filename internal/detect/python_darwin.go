package detect

import (
	"os"
	"path/filepath"
)

// xcodeSelectLink leads to the developer directory xcode-select chose,
// when the Command Line Tools or Xcode are installed; a variable, for
// tests.
var xcodeSelectLink = "/var/db/xcode_select_link"

// cltDir is where the Command Line Tools install, the developer
// directory when xcode-select chose none, as after Software Update
// installed them; a variable, for tests.
var cltDir = "/Library/Developer/CommandLineTools"

// developerDir is the developer directory xcode-select uses, as
// xcode-select -p finds it; false when there is none.
func developerDir() (string, bool) {
	if dir, err := filepath.EvalSymlinks(xcodeSelectLink); err == nil {
		return dir, true
	}
	if info, err := os.Stat(filepath.Join(cltDir, "usr", "bin")); err == nil && info.IsDir() {
		return cltDir, true
	}
	return "", false
}

// pythonStub reports macOS's /usr/bin/python3 when the Command Line Tools
// are not installed: it then offers to install them instead of running.
func pythonStub(path, _ string) bool {
	if path != "/usr/bin/python3" {
		return false
	}
	_, ok := developerDir()
	return !ok
}
