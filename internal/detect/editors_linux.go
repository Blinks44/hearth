package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// toolboxScriptExt is what Toolbox adds to its scripts' names.
const toolboxScriptExt = ""

// codeApps are where an editor of VS Code's is installed: its package's
// folder, or its snap.
func codeApps(_ sysenv.Env, _, id string) []string {
	switch id {
	case "cursor":
		return []string{"/usr/share/cursor", "/opt/Cursor"}
	case "devin-desktop", "windsurf":
		return []string{"/usr/share/" + id}
	}
	return []string{"/usr/share/code", "/snap/code/current/usr/share/code", "/opt/visual-studio-code"}
}

// codeResources are the folders of the app's package.json.
func codeResources(app string) []string { return []string{filepath.Join(app, "resources", "app")} }

// jetbrainsInfos are the product-info.json of the JetBrains IDEs: where
// Toolbox puts them, now and before its version 2, unpacked in /opt, and
// their snaps.
func jetbrainsInfos(env sysenv.Env, home string) []string {
	toolbox := filepath.Join(xdgData(env, home), "JetBrains", "Toolbox", "apps")
	var out []string
	for _, pattern := range []string{
		filepath.Join(toolbox, "*", "product-info.json"),
		filepath.Join(toolbox, "*", "ch-*", "*", "product-info.json"),
		"/opt/*/product-info.json",
		"/snap/*/current/product-info.json",
	} {
		paths, _ := filepath.Glob(pattern)
		out = append(out, paths...)
	}
	return out
}

// jetbrainsApp is the IDE's folder of its product-info.json.
func jetbrainsApp(info string) string { return filepath.Dir(info) }

// jetbrainsDirs are the IDE's plugins and caches folders, by its
// settings folder's name: plugins are right in its data folder.
func jetbrainsDirs(env sysenv.Env, home, data string) (plugins, caches string) {
	return filepath.Join(xdgData(env, home), "JetBrains", data), filepath.Join(or(env.Get("XDG_CACHE_HOME"), filepath.Join(home, ".cache")), "JetBrains", data)
}

// toolboxScripts is the folder of Toolbox's command scripts.
func toolboxScripts(env sysenv.Env, home string) string {
	return filepath.Join(xdgData(env, home), "JetBrains", "Toolbox", "scripts")
}

func xdgData(env sysenv.Env, home string) string {
	return or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
}
