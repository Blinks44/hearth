package inventory

// EditorsState is the code editors and IDEs found, for "editors" only.
type EditorsState struct {
	Editors []Editor `json:",omitempty"`
}

// Editor is one editor or IDE: VS Code, Cursor or a JetBrains IDE.
type Editor struct {
	// ID is "vscode", "cursor", or a JetBrains IDE's product code, as "IU".
	ID, Name, Version string
	// App is the editor's app or install folder; empty when only its
	// extensions are left.
	App string `json:",omitempty"`
	// Toolbox reports that JetBrains Toolbox installed it.
	Toolbox bool `json:",omitempty"`
	// Command is the editor's command, as "code" or "idea"; CommandPath
	// where the terminal finds it, empty when not on its PATH;
	// CommandReal the file it leads to, as the app's own bin/code.
	Command     string
	CommandPath string `json:",omitempty"`
	CommandReal string `json:",omitempty"`
	// Extensions is the folder of the extensions or plugins; Installed
	// are their IDs, as "golang.go", in VS Code and Cursor, or the
	// plugins' folder names in a JetBrains IDE.
	Extensions string   `json:",omitempty"`
	Installed  []string `json:",omitempty"`
	// Superseded are the folders of extensions' older versions an update
	// left behind.
	Superseded []string `json:",omitempty"`
	// Config is a JetBrains IDE's settings folder name, as
	// "IntelliJIdea2024.3"; Caches its caches and indexes, made again
	// by the IDE.
	Config string `json:",omitempty"`
	Caches string `json:",omitempty"`
}
