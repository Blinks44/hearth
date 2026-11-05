package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

const dotnetExe = "dotnet.exe"

func dotnetRoots(env sysenv.Env) []string {
	var roots []string
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if path := env.Get(key); path != "" {
			roots = append(roots, filepath.Join(path, "dotnet"))
		}
	}
	return roots
}
