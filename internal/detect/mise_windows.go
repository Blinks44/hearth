//go:build windows

package detect

import (
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func findMiseBinary(term sysenv.Env, home string) string {
	if paths := sysenv.LookPathAll("mise", term); len(paths) > 0 {
		return paths[0]
	}
	var places []string
	if local := term.Get("LOCALAPPDATA"); local != "" {
		places = append(places,
			filepath.Join(local, "Microsoft", "WinGet", "Links", "mise.exe"),
			filepath.Join(local, "Microsoft", "WinGet", "Packages", "jdx.mise_Microsoft.Winget.Source_8wekyb3d8bbwe", "mise", "bin", "mise.exe"),
		)
	}
	if home == "" {
		home = term.Get("USERPROFILE")
	}
	if home != "" {
		places = append(places,
			filepath.Join(home, "scoop", "shims", "mise.exe"),
			filepath.Join(home, ".cargo", "bin", "mise.exe"),
		)
	}
	for _, p := range places {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return ""
}
