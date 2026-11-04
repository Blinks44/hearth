package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// userConfigDir is os.UserConfigDir for env: Library/Application Support
// in the home.
func userConfigDir(_ sysenv.Env, home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support")
}
