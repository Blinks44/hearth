package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Toolbox 2 unpacks an IDE right in apps; its plugins are in its data
// folder itself.
func TestEditorsJetBrainsLinux(t *testing.T) {
	home := root(t)
	data := filepath.Join(home, ".local", "share", "JetBrains")
	app := filepath.Join(data, "Toolbox", "apps", "goland")
	writeFile(t, filepath.Join(app, "product-info.json"), `{"name":"GoLand","version":"2025.1","productCode":"GO","dataDirectoryName":"GoLand2025.1","launch":[{"launcherPath":"bin/goland.sh"}]}`)
	writeFile(t, filepath.Join(data, "GoLand2025.1", "protobuf", "x"), "")
	scripts := filepath.Join(data, "Toolbox", "scripts")
	fakeExe(t, filepath.Join(scripts, "goland"))
	env := sysenv.Env{"HOME=" + home, "PATH=" + scripts}
	st, _ := Editors(t.Context(), &run.Fake{}, env, env)
	var ide *struct{}
	for _, e := range st.Editors.Editors {
		if e.ID != "GO" {
			continue
		}
		ide = &struct{}{}
		if e.App != app || e.Command != "goland" || !e.Toolbox || e.CommandPath != filepath.Join(scripts, "goland") || len(e.Installed) != 1 || e.Caches != "" {
			t.Fatalf("%+v", e)
		}
	}
	if ide == nil {
		t.Fatalf("%+v", st.Editors)
	}
}
