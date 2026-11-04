package ai

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// cursorApp is where Cursor's editor is, as its installer for the user
// puts it, and its version.
func cursorApp(env sysenv.Env, _ string) (path, version string) {
	return firstApp([]string{join(env.Get("LOCALAPPDATA"), "Programs", "cursor", "Cursor.exe")},
		func(app string) string { return filepath.Join(filepath.Dir(app), "resources", "app", "package.json") })
}
