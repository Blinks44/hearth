package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	gitExe = "git.exe"
	ghExe  = "gh.exe"
)

// gitStub reports a stand-in git; Windows has none.
func gitStub(string) bool { return false }

// ghConfigDir is where GitHub CLI keeps its settings: %AppData%\GitHub CLI.
func ghConfigDir(env sysenv.Env, _ string) string {
	if dir := env.Get("GH_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(env.Get("APPDATA"), "GitHub CLI")
}
