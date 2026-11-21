package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func jsLaunchers(name string) []string { return []string{name + ".cmd", name + ".exe"} }

// userCacheDir is where Corepack caches under the home when no cache
// variable is set.
var userCacheDir = filepath.Join("AppData", "Local")

func pnpmHome(env sysenv.Env) string {
	if p := env.Get("PNPM_HOME"); p != "" {
		return p
	}
	if p := env.Get("LOCALAPPDATA"); p != "" {
		return filepath.Join(p, "pnpm")
	}
	return ""
}

// yarnGlobalDir is where yarn global add installs packages on Windows.
func yarnGlobalDir(env sysenv.Env) string {
	if p := env.Get("YARN_GLOBAL_FOLDER"); p != "" {
		return p
	}
	if p := env.Get("LOCALAPPDATA"); p != "" {
		return filepath.Join(p, "Yarn", "Data", "global")
	}
	if home := sources.ManagerDirs(env).Home; home != "" {
		return filepath.Join(home, "AppData", "Local", "Yarn", "Data", "global")
	}
	return ""
}
