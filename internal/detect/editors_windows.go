package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// toolboxScriptExt is what Toolbox adds to its scripts' names.
const toolboxScriptExt = ".cmd"

// codeApps are where an editor of VS Code's is installed: for the user
// alone, or for all.
func codeApps(env sysenv.Env, _, id string) []string {
	name := map[string]string{"vscode": "Microsoft VS Code", "cursor": "cursor", "devin-desktop": "Devin", "windsurf": "Windsurf"}[id]
	var out []string
	if local := env.Get("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "Programs", name))
	}
	return append(out, filepath.Join(or(env.Get("ProgramFiles"), `C:\Program Files`), name))
}

// codeResources are the folders of the app's package.json: VS Code
// keeps its files in a folder named by its commit, as e646868227, to
// update in the background; the old one goes once the new one runs.
func codeResources(app string) []string {
	versioned, _ := filepath.Glob(filepath.Join(app, "*", "resources", "app"))
	return append([]string{filepath.Join(app, "resources", "app")}, versioned...)
}

// codeCommandSource is the editor's CLI launcher inside its install folder.
func codeCommandSource(app, command string) string {
	for _, ext := range []string{".cmd", ".exe"} {
		if p := filepath.Join(app, "bin", command+ext); exists(p) {
			return p
		}
	}
	return ""
}

// jetbrainsInfos are the product-info.json of the JetBrains IDEs: where
// Toolbox 2 puts them, older Toolbox's apps folder, and their
// installers' Program Files\JetBrains.
func jetbrainsInfos(env sysenv.Env, _ string) []string {
	var patterns []string
	if local := env.Get("LOCALAPPDATA"); local != "" {
		patterns = append(patterns, filepath.Join(local, "Programs", "*", "product-info.json"),
			filepath.Join(local, "JetBrains", "Toolbox", "apps", "*", "ch-*", "*", "product-info.json"))
	}
	patterns = append(patterns, filepath.Join(or(env.Get("ProgramFiles"), `C:\Program Files`), "JetBrains", "*", "product-info.json"))
	var out []string
	for _, pattern := range patterns {
		paths, _ := filepath.Glob(pattern)
		out = append(out, paths...)
	}
	return out
}

// jetbrainsApp is the IDE's folder of its product-info.json.
func jetbrainsApp(info string) string { return filepath.Dir(info) }

// jetbrainsDirs are the IDE's plugins and caches folders, by its
// settings folder's name: settings roam in %APPDATA%, caches stay in
// %LOCALAPPDATA%.
func jetbrainsDirs(env sysenv.Env, _, data string) (plugins, caches string) {
	if roaming := env.Get("APPDATA"); roaming != "" {
		plugins = filepath.Join(roaming, "JetBrains", data, "plugins")
	}
	if local := env.Get("LOCALAPPDATA"); local != "" {
		caches = filepath.Join(local, "JetBrains", data)
	}
	return plugins, caches
}

// toolboxScripts is the folder of Toolbox's command scripts.
func toolboxScripts(env sysenv.Env, _ string) string {
	if local := env.Get("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "JetBrains", "Toolbox", "scripts")
	}
	return ""
}
