package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// gcloudConfigDir is where gcloud keeps its configurations:
// %AppData%\gcloud.
func gcloudConfigDir(env sysenv.Env, _ string) string {
	return filepath.Join(env.Get("APPDATA"), "gcloud")
}

// helmConfigDir is where helm keeps its settings: %AppData%\helm.
func helmConfigDir(env sysenv.Env, home string) string {
	return filepath.Join(userConfigDir(env, home), "helm")
}
