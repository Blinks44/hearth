//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// corepackShims are the files corepack enable puts beside node.
var corepackShims = []string{"pnpm", "yarn"}

// shellStartup returns the text of the shells' startup files in home:
// zsh's (in ZDOTDIR when set), bash's, sh's and fish's.
func shellStartup(env sysenv.Env, home string) []string {
	if home == "" {
		return []string{}
	}
	zdot := env.Get("ZDOTDIR")
	if zdot == "" {
		zdot = home
	}
	paths := []string{
		filepath.Join(zdot, ".zshrc"), filepath.Join(zdot, ".zprofile"), filepath.Join(zdot, ".zshenv"),
		filepath.Join(home, ".bashrc"), filepath.Join(home, ".bash_profile"), filepath.Join(home, ".profile"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}
	fish, _ := filepath.Glob(filepath.Join(home, ".config", "fish", "conf.d", "*.fish")) // only a malformed pattern fails
	texts := []string{}
	for _, p := range append(paths, fish...) {
		texts = append(texts, readFile(p))
	}
	return texts
}
