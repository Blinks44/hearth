package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const rubyExe = "ruby.exe"

func rubyOSGlobs(env sysenv.Env) []string {
	patterns := []string{`C:\Ruby*\bin\ruby.exe`}
	if dir := env.Get("LOCALAPPDATA"); dir != "" {
		patterns = append(patterns, filepath.Join(dir, "Programs", "Ruby*", "bin", "ruby.exe"))
	}
	return patterns
}
