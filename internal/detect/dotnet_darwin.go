package detect

import (
	"github.com/Blinks44/hearth/internal/sysenv"
)

const dotnetExe = "dotnet"

func dotnetRoots(env sysenv.Env) []string {
	return []string{"/usr/local/share/dotnet", "/usr/local/share/dotnet/x64", "/opt/homebrew/share/dotnet", "/usr/local/opt/dotnet/libexec"}
}
