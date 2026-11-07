package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// toolboxScriptExt is what Toolbox adds to its scripts' names.
const toolboxScriptExt = ""

// codeApps are where an editor of VS Code's is installed: in
// /Applications or the user's own.
func codeApps(_ sysenv.Env, home, id string) []string {
	name := map[string]string{"vscode": "Visual Studio Code.app", "cursor": "Cursor.app", "devin-desktop": "Devin.app", "windsurf": "Windsurf.app"}[id]
	return []string{filepath.Join(applications, name), filepath.Join(home, "Applications", name)}
}

// codeResources are the folders of the app's package.json.
func codeResources(app string) []string {
	return []string{filepath.Join(app, "Contents", "Resources", "app")}
}

// jetbrainsInfos are the product-info.json of the JetBrains IDEs: in
// /Applications, in ~/Applications, where Toolbox 2 puts them, and in
// older Toolbox's apps folder.
func jetbrainsInfos(_ sysenv.Env, home string) []string {
	var out []string
	for _, pattern := range []string{
		filepath.Join(applications, "*.app", "Contents", "Resources", "product-info.json"),
		filepath.Join(home, "Applications", "*.app", "Contents", "Resources", "product-info.json"),
		filepath.Join(home, "Library", "Application Support", "JetBrains", "Toolbox", "apps", "*", "ch-*", "*", "*.app", "Contents", "Resources", "product-info.json"),
	} {
		paths, _ := filepath.Glob(pattern)
		out = append(out, paths...)
	}
	return out
}

// jetbrainsApp is the IDE's app of its product-info.json.
func jetbrainsApp(info string) string { return filepath.Dir(filepath.Dir(filepath.Dir(info))) }

// jetbrainsDirs are the IDE's plugins and caches folders, by its
// settings folder's name.
func jetbrainsDirs(_ sysenv.Env, home, data string) (plugins, caches string) {
	return filepath.Join(home, "Library", "Application Support", "JetBrains", data, "plugins"), filepath.Join(home, "Library", "Caches", "JetBrains", data)
}

// toolboxScripts is the folder of Toolbox's command scripts.
func toolboxScripts(_ sysenv.Env, home string) string {
	return filepath.Join(home, "Library", "Application Support", "JetBrains", "Toolbox", "scripts")
}
