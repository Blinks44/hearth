package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	gitExe = "git"
	ghExe  = "gh"
)

// gitStub reports a stand-in git; Linux has none.
func gitStub(string) bool { return false }

// ghConfigDir is where GitHub CLI keeps its settings.
func ghConfigDir(env sysenv.Env, home string) string {
	if dir := env.Get("GH_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(or(env.Get("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "gh")
}
