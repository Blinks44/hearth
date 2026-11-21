//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func jsLaunchers(name string) []string { return []string{name} }

// userCacheDir is where Corepack caches under the home when no cache
// variable is set.
const userCacheDir = ".cache"

// yarnGlobalDir is where yarn global add installs packages on Unix.
func yarnGlobalDir(env sysenv.Env) string {
	if p := env.Get("YARN_GLOBAL_FOLDER"); p != "" {
		return p
	}
	home := sources.ManagerDirs(env).Home
	if home == "" {
		return ""
	}
	return filepath.Join(or(env.Get("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "yarn", "global")
}
