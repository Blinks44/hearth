package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// A JetBrains IDE's settings roam in %APPDATA%, its caches stay in
// %LOCALAPPDATA%; its launcher is bin\idea64.exe, its command idea.
func TestEditorsWindows(t *testing.T) {
	dir := root(t)
	local, roaming := filepath.Join(dir, "Local"), filepath.Join(dir, "Roaming")
	// VS Code 1.10x keeps its files in a folder named by its commit.
	writeFile(t, filepath.Join(local, "Programs", "Microsoft VS Code", "e646868227", "resources", "app", "package.json"), `{"version":"1.106.2"}`)
	app := filepath.Join(local, "Programs", "IntelliJ IDEA Ultimate")
	writeFile(t, filepath.Join(app, "product-info.json"), `{"name":"IntelliJ IDEA","version":"2024.3.1","productCode":"IU","dataDirectoryName":"IntelliJIdea2024.3","launch":[{"launcherPath":"bin/idea64.exe"}]}`)
	writeFile(t, filepath.Join(roaming, "JetBrains", "IntelliJIdea2024.3", "plugins", "go", "x"), "")
	writeFile(t, filepath.Join(local, "JetBrains", "IntelliJIdea2024.3", "caches", "x"), "")
	scripts := filepath.Join(local, "JetBrains", "Toolbox", "scripts")
	writeFile(t, filepath.Join(scripts, "idea.cmd"), "")
	env := sysenv.Env{"USERPROFILE=" + dir, "LOCALAPPDATA=" + local, "APPDATA=" + roaming, "ProgramFiles=" + filepath.Join(dir, "pf"), "PATH=" + scripts, "PATHEXT=.COM;.EXE;.BAT;.CMD"}
	st, err := Editors(t.Context(), &run.Fake{}, env, env)
	if err != nil {
		t.Fatal(err)
	}
	eds := st.Editors.Editors
	if len(eds) != 2 || eds[0].Version != "1.106.2" {
		t.Fatalf("%+v", eds)
	}
	ide := eds[1]
	if ide.App != app || ide.Command != "idea" || !ide.Toolbox || ide.CommandPath == "" || len(ide.Installed) != 1 || ide.Caches != filepath.Join(local, "JetBrains", "IntelliJIdea2024.3") {
		t.Fatalf("%+v", ide)
	}
}
