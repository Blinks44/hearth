package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// helmConfigDir is where helm keeps its settings: XDG_CONFIG_HOME/helm, by
// default ~/.config/helm.
func helmConfigDir(env sysenv.Env, home string) string {
	return filepath.Join(userConfigDir(env, home), "helm")
}
