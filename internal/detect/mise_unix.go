//go:build !windows

package detect

import (
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// miseSystemPrefixes are standard Homebrew prefixes checked when mise is not
// yet on the terminal's PATH; tests override this slice.
var miseSystemPrefixes = []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}

func findMiseBinary(term sysenv.Env, home string) string {
	if paths := sysenv.LookPathAll("mise", term); len(paths) > 0 {
		return paths[0]
	}
	var places []string
	if home != "" {
		places = append(places,
			filepath.Join(home, ".local", "bin", "mise"),
			filepath.Join(home, ".cargo", "bin", "mise"),
			filepath.Join(home, ".linuxbrew", "bin", "mise"),
		)
	}
	for _, prefix := range miseSystemPrefixes {
		places = append(places, filepath.Join(prefix, "bin", "mise"))
	}
	for _, p := range places {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return ""
}
