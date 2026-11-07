package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// helmConfigDir is where helm keeps its settings: XDG_CONFIG_HOME/helm when
// set, else ~/Library/Preferences/helm.
func helmConfigDir(env sysenv.Env, home string) string {
	if dir := env.Get("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "helm")
	}
	return filepath.Join(home, "Library", "Preferences", "helm")
}
