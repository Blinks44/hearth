package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// goExe is go's executable in GOROOT\bin.
const goExe = "go.exe"

// goInstallDirs are where Go's official installer puts it; a variable,
// for tests.
var goInstallDirs = func(env sysenv.Env) []string {
	pf := env.Get("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return []string{filepath.Join(pf, "Go")}
}

// userConfigDir is os.UserConfigDir for env: AppData.
func userConfigDir(env sysenv.Env, _ string) string { return env.Get("APPDATA") }
