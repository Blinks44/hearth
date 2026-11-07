package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// codeEditors are the editors made of VS Code: their ID, name, command
// and the folder in home that keeps their extensions.
var codeEditors = []struct{ id, name, command, dir string }{
	{"vscode", "VS Code", "code", ".vscode"},
	{"cursor", "Cursor", "cursor", ".cursor"},
	// Windsurf became Devin Desktop; an older install keeps its name.
	{"devin-desktop", "Devin Desktop", "devin-desktop", ".devin"},
	{"windsurf", "Windsurf", "windsurf", ".windsurf"},
}

// Editors finds VS Code, Cursor and the JetBrains IDEs: the app, its
// command in the terminal, its extensions or plugins, and a JetBrains
// IDE's caches. Files are read; no editor is run.
func Editors(ctx context.Context, _ run.Runner, term, _ sysenv.Env) (inventory.ToolState, error) {
	home := sources.ManagerDirs(term).Home
	st := &inventory.EditorsState{}
	for _, spec := range codeEditors {
		e := inventory.Editor{ID: spec.id, Name: spec.name, Command: spec.command}
		for _, app := range codeApps(term, home, spec.id) {
			if v := codeVersion(app); v != "" {
				e.App, e.Version = app, v
				break
			}
		}
		e.CommandPath, e.CommandReal = firstOnPath(spec.command, term)
		if home != "" {
			e.Extensions = filepath.Join(home, spec.dir, "extensions")
		}
		if !isDir(e.Extensions) {
			e.Extensions = ""
		}
		e.Installed, e.Superseded = codeExtensions(e.Extensions)
		if e.App != "" || e.CommandPath != "" || e.Extensions != "" {
			st.Editors = append(st.Editors, e)
		}
	}
	st.Editors = append(st.Editors, jetbrainsIDEs(term, home)...)
	return inventory.ToolState{Tool: "editors", Editors: st}, ctx.Err()
}

// codeVersion is the version in the app's package.json, the newest
// written when an update left two. Windsurf and Devin Desktop keep VS
// Code's there, and their own as windsurfVersion in product.json.
func codeVersion(app string) string {
	var version string
	var newest time.Time
	for _, dir := range codeResources(app) {
		path := filepath.Join(dir, "package.json")
		info, err := os.Stat(path)
		data, err2 := os.ReadFile(path)
		var pkg struct{ Version string }
		if err != nil || err2 != nil || json.Unmarshal(data, &pkg) != nil || pkg.Version == "" || info.ModTime().Before(newest) {
			continue
		}
		var product struct{ WindsurfVersion string }
		if data, err := os.ReadFile(filepath.Join(dir, "product.json")); err == nil && json.Unmarshal(data, &product) == nil && product.WindsurfVersion != "" {
			pkg.Version = product.WindsurfVersion
		}
		version, newest = pkg.Version, info.ModTime()
	}
	return version
}

// codeExtensions reads the extensions folder's extensions.json, which
// VS Code keeps of what is installed: the extensions' IDs, and the
// folders it does not list, older versions an update left.
func codeExtensions(dir string) (installed, superseded []string) {
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "extensions.json"))
	var list []struct {
		Identifier       struct{ ID string }
		RelativeLocation string
	}
	if err != nil || json.Unmarshal(data, &list) != nil {
		return nil, nil
	}
	kept := map[string]bool{}
	for _, ext := range list {
		installed = append(installed, strings.ToLower(ext.Identifier.ID))
		kept[ext.RelativeLocation] = true
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !kept[e.Name()] {
			superseded = append(superseded, filepath.Join(dir, e.Name()))
		}
	}
	return installed, superseded
}

// jetbrainsIDEs are the JetBrains IDEs, by their product-info.json; the
// Android Studio's is the mobile page's.
func jetbrainsIDEs(env sysenv.Env, home string) []inventory.Editor {
	var out []inventory.Editor
	scripts := toolboxScripts(env, home)
	for _, info := range jetbrainsInfos(env, home) {
		var p struct {
			Name, Version, ProductCode, DataDirectoryName string
			Launch                                        []struct{ LauncherPath string }
		}
		data, err := os.ReadFile(info)
		if err != nil || json.Unmarshal(data, &p) != nil || p.ProductCode == "" || p.ProductCode == "AI" || len(p.Launch) == 0 {
			continue
		}
		app := jetbrainsApp(info)
		if slices.ContainsFunc(out, func(e inventory.Editor) bool { return e.App == app }) {
			continue
		}
		command := jetbrainsCommand(app, p.Launch[0].LauncherPath)
		e := inventory.Editor{ID: p.ProductCode, Name: p.Name, Version: p.Version, App: app, Command: command, Config: p.DataDirectoryName,
			Toolbox: strings.Contains(app, "Toolbox") || scripts != "" && exists(filepath.Join(scripts, command+toolboxScriptExt))}
		e.CommandPath, e.CommandReal = firstOnPath(command, env)
		// Homebrew names the community editions' commands idea-ce, pycharm-ce.
		if e.CommandPath == "" && (p.ProductCode == "IC" || p.ProductCode == "PC") {
			if path, real := firstOnPath(command+"-ce", env); path != "" {
				e.Command, e.CommandPath, e.CommandReal = command+"-ce", path, real
			}
		}
		if p.DataDirectoryName != "" {
			plugins, caches := jetbrainsDirs(env, home, p.DataDirectoryName)
			if entries, err := os.ReadDir(plugins); err == nil {
				e.Extensions = plugins
				for _, entry := range entries {
					if !strings.HasPrefix(entry.Name(), ".") {
						e.Installed = append(e.Installed, entry.Name())
					}
				}
			}
			if isDir(caches) {
				e.Caches = caches
			}
		}
		out = append(out, e)
	}
	return out
}

// jetbrainsCommand is the command of the IDE at app with the launcher
// bin/idea, ../MacOS/idea or bin/idea64.exe: idea. A snap's command is
// the snap's name, as /snap/bin/intellij-idea-community.
func jetbrainsCommand(app, launcher string) string {
	if rest, ok := strings.CutPrefix(filepath.ToSlash(app), "/snap/"); ok {
		name, _, _ := strings.Cut(rest, "/")
		return name
	}
	base := filepath.Base(filepath.FromSlash(launcher))
	return strings.TrimSuffix(strings.TrimSuffix(base, filepath.Ext(base)), "64")
}

// firstOnPath is where the terminal finds command and the file that
// leads to; empty when it does not find it.
func firstOnPath(command string, env sysenv.Env) (path, real string) {
	paths := sysenv.LookPathAll(command, env)
	if len(paths) == 0 {
		return "", ""
	}
	real, err := sysenv.RealPath(paths[0])
	if err != nil {
		real = paths[0]
	}
	return paths[0], real
}
