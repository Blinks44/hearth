package run

import (
	"os"
	"path/filepath"
)

// macEditors are the editors Hearth opens files in when installed, by
// their apps' names. JetBrains IDEs come from the Editors page's scan.
var macEditors = []struct{ name, app string }{
	{"Zed", "Zed"}, {"VS Code", "Visual Studio Code"}, {"Cursor", "Cursor"}, {"Windsurf", "Windsurf"},
	{"Sublime Text", "Sublime Text"}, {"Nova", "Nova"}, {"BBEdit", "BBEdit"}, {"CotEditor", "CotEditor"},
	{"TextMate", "TextMate"}, {"Xcode", "Xcode"},
}

// EditorApps are the editors installed that Hearth can open a file in:
// each one's name and its app bundle.
func EditorApps() []App {
	home, _ := os.UserHomeDir()
	var apps []App
	for _, e := range macEditors {
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			path := filepath.Join(dir, e.app+".app")
			if _, err := os.Stat(path); err == nil {
				apps = append(apps, App{Name: e.name, Path: path})
				break
			}
		}
	}
	return apps
}
