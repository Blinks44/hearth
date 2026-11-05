//go:build !windows

package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// corepackShims are the files corepack enable puts beside node.
var corepackShims = []string{"pnpm", "yarn"}

// shellStartup returns the text of the shells' startup files in home.
func shellStartup(env sysenv.Env, home string) []string {
	texts := []string{}
	for _, p := range StartupFiles(env, home) {
		texts = append(texts, readFile(p))
	}
	return texts
}

// StartupFiles returns the paths of the shells' startup files in home,
// which change what the terminal finds: zsh's (in ZDOTDIR when set),
// bash's, sh's and fish's, whether they exist or not.
func StartupFiles(env sysenv.Env, home string) []string {
	if home == "" {
		return nil
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
	return append(paths, fish...)
}

// scoopShimTarget is the file a Scoop shim runs, only on Windows.
func scoopShimTarget(string) string { return "" }
