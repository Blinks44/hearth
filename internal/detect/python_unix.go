//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	pythonExe = "python3"
	// pythonBin is where python3 lies in an installation's directory.
	pythonBin = "bin/python3"
)

// pythonOSGlobs are where Homebrew keeps Pythons other formulas need,
// often off PATH, and where python.org's installer puts them on macOS.
func pythonOSGlobs(env sysenv.Env) []string {
	globs := []string{"/Library/Frameworks/Python.framework/Versions/*/bin/python3"}
	for _, prefix := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"} {
		if prefix != "" {
			globs = append(globs, filepath.Join(prefix, "Cellar", "python@*", "*", "bin", "python3"))
		}
	}
	return globs
}
