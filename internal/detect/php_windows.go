package detect

import (
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const phpBin = "php.exe"

func phpOSGlobs(env sysenv.Env, home string) []string {
	globs := []string{`C:\xampp\php\php.exe`}
	if home != "" {
		globs = append(globs, filepath.Join(home, ".config", "herd", "bin", "php*", "php.exe"), filepath.Join(home, "scoop", "apps", "php*", "*", "php.exe"))
	}
	if scoop := env.Get("SCOOP"); scoop != "" {
		globs = append(globs, filepath.Join(scoop, "apps", "php*", "*", "php.exe"))
	}
	return globs
}
func phpHerd(path, home string) bool {
	return home != "" && strings.HasPrefix(strings.ToLower(path), strings.ToLower(filepath.Join(home, ".config", "herd", "bin"))+string(filepath.Separator))
}
