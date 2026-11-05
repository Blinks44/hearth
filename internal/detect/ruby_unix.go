//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const rubyExe = "ruby"

func rubyOSGlobs(env sysenv.Env) []string {
	patterns := []string{"/opt/rubies/*/bin/ruby"}
	for _, prefix := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"} {
		if prefix != "" {
			patterns = append(patterns, filepath.Join(prefix, "Cellar", "ruby*", "*", "bin", "ruby"))
		}
	}
	return patterns
}

// rubyInstallerRoot is RubyInstaller's folder, only on Windows.
func rubyInstallerRoot(string) string { return "" }
