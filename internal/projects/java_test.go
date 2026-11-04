package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestJavaProject(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ file, data, want string }{{".java-version", "17\n", "17"}, {".sdkmanrc", "# comment\njava=17.0.13-tem\n", "17.0.13-tem"}} {
		if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.data), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		var req Requirement
		for _, r := range p.Requirements {
			if r.File == tc.file {
				req = r
			}
		}
		if req.Tool != "java" || req.Want != tc.want {
			t.Fatalf("req %+v", req)
		}
	}
	for _, auto := range []bool{true, false} {
		snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "java", Installations: []inventory.Installation{
			{Tool: "java", Version: "21.0.5", OnPath: 1},
			{Tool: "java", Version: "17.0.13", Source: inventory.Source{Manager: "sdkman", Version: "17.0.13-tem"}},
		}, Managers: []inventory.Manager{{Name: "sdkman", SwitchOnCD: auto, Files: []string{".sdkmanrc"}}}}}}
		st := Resolve(Read(dir), snap, nil)[0]
		if len(st.Checks) != 2 || st.Checks[1].Installed == nil || st.Checks[1].Met != auto {
			t.Fatalf("auto %v: %+v", auto, st)
		}
	}
}
