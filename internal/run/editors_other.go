//go:build !darwin

package run

import "os/exec"

// editorCommands are the editors Hearth opens files in when their command
// is on the PATH: the name shown and the commands to look for.
var editorCommands = []struct {
	name     string
	commands []string
}{
	{"Zed", []string{"zed", "zeditor"}},
	{"VS Code", []string{"code"}},
	{"Cursor", []string{"cursor"}},
	{"Windsurf", []string{"windsurf"}},
	{"Sublime Text", []string{"subl"}},
	{"Notepad++", []string{"notepad++"}},
	{"Kate", []string{"kate"}},
	{"GNOME Text Editor", []string{"gnome-text-editor"}},
	{"gedit", []string{"gedit"}},
}

// EditorApps are the editors whose command is on the PATH that Hearth can
// open a file in: each one's name and its command.
func EditorApps() []App {
	var apps []App
	for _, e := range editorCommands {
		for _, cmd := range e.commands {
			if p, err := exec.LookPath(cmd); err == nil {
				apps = append(apps, App{Name: e.name, Path: p})
				break
			}
		}
	}
	return apps
}
