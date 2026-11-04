package ai

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// applications is where macOS keeps apps for every user; tests move it.
var applications = "/Applications"

// cursorApp is where Cursor's editor is, in the Applications of every
// user or of this one, and its version.
func cursorApp(_ sysenv.Env, home string) (path, version string) {
	return firstApp([]string{filepath.Join(applications, "Cursor.app"), join(home, "Applications", "Cursor.app")},
		func(app string) string { return filepath.Join(app, "Contents", "Resources", "app", "package.json") })
}
