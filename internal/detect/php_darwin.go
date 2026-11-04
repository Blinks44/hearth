package detect

import (
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const phpBin = "bin/php"

func phpOSGlobs(env sysenv.Env, home string) []string {
	var globs []string
	for _, prefix := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local"} {
		if prefix != "" {
			globs = append(globs, filepath.Join(prefix, "Cellar", "php*", "*", phpBin))
		}
	}
	if home != "" {
		// Herd's php picks a version itself; only its versioned PHPs are copies.
		globs = append(globs, filepath.Join(home, "Library", "Application Support", "Herd", "bin", "php[0-9]*"))
	}
	return globs
}

func phpHerd(path, home string) bool {
	return home != "" && strings.HasPrefix(path, filepath.Join(home, "Library", "Application Support", "Herd", "bin")+string(filepath.Separator))
}
