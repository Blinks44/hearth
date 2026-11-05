package detect

import (
	"github.com/Blinks44/hearth/internal/sysenv"
)

const dotnetExe = "dotnet"

func dotnetRoots(env sysenv.Env) []string {
	return []string{"/usr/share/dotnet", "/usr/lib/dotnet", "/usr/lib64/dotnet", "/opt/dotnet"}
}
