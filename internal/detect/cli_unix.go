//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// gcloudConfigDir is where gcloud keeps its configurations: ~/.config/gcloud
// on macOS too, whatever XDG_CONFIG_HOME says.
func gcloudConfigDir(_ sysenv.Env, home string) string {
	return filepath.Join(home, ".config", "gcloud")
}
