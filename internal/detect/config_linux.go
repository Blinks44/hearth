package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// userConfigDir is os.UserConfigDir for env: XDG_CONFIG_HOME, by default
// .config in the home.
func userConfigDir(env sysenv.Env, home string) string {
	if dir := env.Get("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}
