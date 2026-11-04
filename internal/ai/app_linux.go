package ai

import "github.com/Blinks44/hearth/internal/sysenv"

// cursorApp finds no editor on Linux: Cursor comes as an AppImage the user
// keeps anywhere. Its agent's command is found on PATH.
func cursorApp(sysenv.Env, string) (path, version string) { return "", "" }
