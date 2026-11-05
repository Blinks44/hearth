package detect

import (
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// corepackShims are the files corepack enable puts beside node.
var corepackShims = []string{"pnpm.cmd", "yarn.cmd"}

// shellStartup returns the text of PowerShell's profiles in home.
func shellStartup(env sysenv.Env, home string) []string {
	texts := []string{}
	for _, p := range StartupFiles(env, home) {
		texts = append(texts, readFile(p))
	}
	return texts
}

// StartupFiles returns the paths of PowerShell's profiles in home, both
// PowerShell 7's and Windows PowerShell's, whether they exist or not.
func StartupFiles(_ sysenv.Env, home string) []string {
	if home == "" {
		return nil
	}
	var paths []string
	for _, dir := range []string{"PowerShell", "WindowsPowerShell"} {
		paths = append(paths, filepath.Join(home, "Documents", dir, "Microsoft.PowerShell_profile.ps1"))
	}
	return paths
}

// scoopShimTarget is the file a Scoop shim runs: path = "..." in the
// .shim beside an .exe shim, or the first line's @rem of a .cmd one; ""
// when it names none.
func scoopShimTarget(path string) string {
	ext := filepath.Ext(path)
	if strings.EqualFold(ext, ".exe") {
		for line := range strings.Lines(readFile(strings.TrimSuffix(path, ext) + ".shim")) {
			if value, ok := strings.CutPrefix(strings.TrimSpace(line), "path ="); ok {
				return strings.Trim(strings.TrimSpace(value), `"`)
			}
		}
		return ""
	}
	if strings.EqualFold(ext, ".cmd") {
		first, _, _ := strings.Cut(readFile(path), "\n")
		if target, ok := strings.CutPrefix(strings.TrimSpace(first), "@rem "); ok && filepath.IsAbs(target) {
			return target
		}
	}
	return ""
}
