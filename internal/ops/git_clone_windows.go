//go:build windows

package ops

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func gitFallbackPaths(env sysenv.Env) []string {
	var dirs []string
	if pf := env.Get("ProgramFiles"); pf != "" {
		dirs = append(dirs, filepath.Join(pf, "Git", "cmd"), filepath.Join(pf, "Git", "bin"))
	} else {
		dirs = append(dirs, `C:\Program Files\Git\cmd`, `C:\Program Files\Git\bin`)
	}
	if home := env.Get("USERPROFILE"); home != "" {
		dirs = append(dirs, filepath.Join(home, "scoop", "shims"))
	}
	return dirs
}
