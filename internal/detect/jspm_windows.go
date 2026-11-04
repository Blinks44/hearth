package detect

import (
	"path/filepath"

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
