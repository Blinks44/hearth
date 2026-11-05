package detect

import (
	"path/filepath"
	"strings"

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

// rubyInstallerRoot is the folder RubyInstaller put the ruby at path in,
// C:\Ruby34-x64 at a drive's root; "" for another ruby.
func rubyInstallerRoot(path string) string {
	root := filepath.Dir(filepath.Dir(path))
	if !strings.EqualFold(filepath.Base(filepath.Dir(path)), "bin") ||
		filepath.Dir(root) != filepath.VolumeName(root)+`\` ||
		!strings.HasPrefix(strings.ToLower(filepath.Base(root)), "ruby") {
		return ""
	}
	return root
}
