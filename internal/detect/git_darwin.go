package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	gitExe = "git"
	ghExe  = "gh"
)

// gitStub reports macOS's /usr/bin/git when the Command Line Tools are not
// installed: it then offers to install them instead of running.
func gitStub(path string) bool {
	if path != "/usr/bin/git" {
		return false
	}
	_, err := filepath.EvalSymlinks(xcodeSelectLink)
	return err != nil
}

// ghConfigDir is where GitHub CLI keeps its settings.
func ghConfigDir(env sysenv.Env, home string) string {
	if dir := env.Get("GH_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(or(env.Get("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "gh")
}
