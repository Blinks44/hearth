package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func pnpmHome(env sysenv.Env) string {
	if p := env.Get("PNPM_HOME"); p != "" {
		return p
	}
	home := sources.ManagerDirs(env).Home
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "pnpm")
}
