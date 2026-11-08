package ops

import (
	"path/filepath"
	"slices"
	"strings"

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
	var s MiseSetup
	if found := sysenv.LookPathAll("mise", env); len(found) > 0 {
		s.Mise = found[0]
	} else {
		s.Steps = []Step{{Argv: []string{"winget", "install", "--id", "jdx.mise", "--exact", "--source", "winget",
			"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}}}
		// WinGet unpacks mise into the package's folder and puts its bin
		// on the user's Path, which the terminal sees only once it ran.
		s.Mise = filepath.Join(env.Get("LOCALAPPDATA"), "Microsoft", "WinGet", "Packages",
			"jdx.mise_Microsoft.Winget.Source_8wekyb3d8bbwe", "mise", "bin", "mise.exe")
	}
	shims := sysenv.ExpandPath(miseShims)
	onPath := slices.ContainsFunc(strings.Split(env.Get("PATH"), ";"), func(d string) bool { return sysenv.SamePath(d, shims) })
	if !onPath {
		s.Steps = append(s.Steps, Step{UserPath: &UserPathEdit{Add: miseShims}})
	}
	return s, nil
}
