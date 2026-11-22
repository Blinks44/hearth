package run

import "strings"

// editorLauncher opens path with the app bundle, or its command without
// one.
func editorLauncher(app, command, path string) []string {
	trimmed := strings.TrimRight(app, "/")
	switch {
	case strings.HasSuffix(trimmed, ".app"):
		return []string{"/usr/bin/open", "-a", trimmed, path}
	case command != "":
		return []string{command, path}
	case trimmed != "":
		return []string{trimmed, path}
	}
	return nil
}
