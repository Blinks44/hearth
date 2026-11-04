package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// corepackShims are the files corepack enable puts beside node.
var corepackShims = []string{"pnpm.cmd", "yarn.cmd"}

// shellStartup returns the text of PowerShell's profiles in home, both
// PowerShell 7's and Windows PowerShell's.
func shellStartup(_ sysenv.Env, home string) []string {
	texts := []string{}
	if home == "" {
		return texts
	}
	for _, dir := range []string{"PowerShell", "WindowsPowerShell"} {
		texts = append(texts, readFile(filepath.Join(home, "Documents", dir, "Microsoft.PowerShell_profile.ps1")))
	}
	return texts
}
