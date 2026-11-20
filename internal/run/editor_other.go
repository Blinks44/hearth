//go:build !darwin

package run

// editorLauncher opens path with the editor's command: its app folder
// holds no launcher to call on Linux and Windows.
func editorLauncher(_, command, path string) []string {
	if command == "" {
		return nil
	}
	return []string{command, path}
}
