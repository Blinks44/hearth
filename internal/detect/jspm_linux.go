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
	if p := env.Get("XDG_DATA_HOME"); p != "" {
		return filepath.Join(p, "pnpm")
	}
	return filepath.Join(home, ".local", "share", "pnpm")
}
