package run

import "strings"

// editorLauncher opens path with the app bundle, or its command without
// one.
func editorLauncher(app, command, path string) []string {
	switch {
	case strings.HasSuffix(app, ".app"):
		return []string{"/usr/bin/open", "-a", app, path}
	case command != "":
		return []string{command, path}
	}
	return nil
}
