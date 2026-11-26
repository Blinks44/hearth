package projects

import (
	"os"
	"path/filepath"
)

func isGitStub(path string) bool {
	if path != "/usr/bin/git" {
		return false
	}
	if _, err := filepath.EvalSymlinks("/var/db/xcode_select_link"); err == nil {
		return false
	}
	if info, err := os.Stat("/Library/Developer/CommandLineTools/usr/bin"); err == nil && info.IsDir() {
		return false
	}
	return true
}
