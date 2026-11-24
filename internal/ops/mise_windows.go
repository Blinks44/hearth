package ops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// miseShims is where mise keeps its shims on Windows, as the user's Path
// writes it.
const miseShims = `%LOCALAPPDATA%\mise\shims`

// SetUpMise returns what sets mise up for the user of env, the terminal's
// environment: WinGet's package where mise is not found, then its shims
// on the user's Path, which new terminals and apps both see, as mise's
// documentation advises on Windows.
func SetUpMise(env sysenv.Env) (MiseSetup, error) {
	s := MiseSetup{Mise: findMiseWindows(env), envPath: env.Get("PATH")}
	if s.Mise == "" {
		s.Steps = []Step{{Argv: []string{"winget", "install", "--id", "jdx.mise", "--exact", "--source", "winget",
			"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}}}
		// WinGet unpacks mise into the package's folder and puts its bin
		// on the user's Path, which the terminal sees only once it ran.
		s.Mise = filepath.Join(env.Get("LOCALAPPDATA"), "Microsoft", "WinGet", "Packages",
			"jdx.mise_Microsoft.Winget.Source_8wekyb3d8bbwe", "mise", "bin", "mise.exe")
	}
	shims := sysenv.ExpandPath(miseShims)
	addEntry := miseShims
	if custom := sources.ManagerDirs(env).Mise; custom != "" && env.Get("MISE_DATA_DIR") != "" {
		shims = filepath.Join(custom, "shims")
		addEntry = shims
	}
	onPath := slices.ContainsFunc(strings.Split(env.Get("PATH"), ";"), func(d string) bool { return sysenv.SamePath(d, shims) })
	if !onPath {
		s.Steps = append(s.Steps, Step{UserPath: &UserPathEdit{Add: addEntry}})
	}
	return s, nil
}

func findMiseWindows(env sysenv.Env) string {
	if found := sysenv.LookPathAll("mise", env); len(found) > 0 {
		return found[0]
	}
	var places []string
	if local := env.Get("LOCALAPPDATA"); local != "" {
		places = append(places,
			filepath.Join(local, "Microsoft", "WinGet", "Links", "mise.exe"),
			filepath.Join(local, "Microsoft", "WinGet", "Packages", "jdx.mise_Microsoft.Winget.Source_8wekyb3d8bbwe", "mise", "bin", "mise.exe"),
		)
	}
	if home := env.Get("USERPROFILE"); home != "" {
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
