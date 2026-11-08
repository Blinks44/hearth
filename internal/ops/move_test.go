package ops

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestMovePlan(t *testing.T) {
	tests := []struct {
		tool, version string
		want          []string // nil: no plan
		venv          bool
	}{
		{"python", "3.12.8", []string{"mise", "use", "-g", "python@3.12.8"}, true},
		{"ruby", "3.3.6", []string{"mise", "use", "-g", "ruby@3.3.6"}, false},
		{"go", "1.24.1", []string{"mise", "use", "-g", "go@1.24.1"}, false},
		{"java", "21", []string{"mise", "use", "-g", "java@temurin-21"}, false},
		{"php", "8.4.3", nil, false},
		{"dotnet", "9.0.100", nil, false},
		{"python", "", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.tool+tc.version, func(t *testing.T) {
			p, ok := MovePlan(VersionTarget{Tool: tc.tool, Title: tc.tool, Version: tc.version}, "brew", nil)
			if tc.want == nil {
				if ok {
					t.Fatalf("plan %+v", p.Steps)
				}
				return
			}
			if !ok || len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tc.want) {
				t.Fatalf("ok=%v steps %+v", ok, p.Steps)
			}
			if venv := len(p.Notes) == 2; venv != tc.venv {
				t.Errorf("notes %+v", p.Notes)
			}
		})
	}
}

// The check finds mise's Temurin of the line, by its own name, the
// default.
func TestMovePlanVerify(t *testing.T) {
	detect := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Installations: []inventory.Installation{{Version: "21.0.13", Default: true,
			Source: inventory.Source{Manager: "mise", Version: "temurin-21.0.13+9.0.LTS"}}}}, nil
	}
	p, _ := MovePlan(VersionTarget{Tool: "java", Title: "Java", Version: "21", Label: "21.0.5"}, "Adoptium", detect)
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPythonOrgVersion(t *testing.T) {
	for path, want := range map[string]string{
		"/Library/Frameworks/Python.framework/Versions/3.13/bin/python3":    "3.13",
		"/Library/Frameworks/Python.framework/Versions/Current/bin/python3": "",
		"/opt/homebrew/bin/python3":                                         "",
	} {
		if got := PythonOrgVersion(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}

// The plan deletes the version's folder, its app folder and the links to
// it, and leaves another version's.
func TestPythonOrgRemove(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("python.org's layout is macOS's")
	}
	root := t.TempDir()
	pythonOrgVersions, pythonOrgApps, pythonOrgBin = filepath.Join(root, "Versions"), filepath.Join(root, "Applications"), filepath.Join(root, "bin")
	t.Cleanup(func() {
		pythonOrgVersions, pythonOrgApps, pythonOrgBin = "/Library/Frameworks/Python.framework/Versions", "/Applications", "/usr/local/bin"
	})
	for _, dir := range []string{"Versions/3.13/bin", "Versions/3.12/bin", "Applications/Python 3.13", "bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{
		"python3":    "../Versions/3.13/bin/python3",
		"pip3.13":    filepath.Join(root, "Versions/3.13/bin/pip3.13"),
		"python3.12": "../Versions/3.12/bin/python3.12",
	} {
		if err := os.Symlink(target, filepath.Join(root, "bin", name)); err != nil {
			t.Fatal(err)
		}
	}
	p, err := PythonOrgRemove("3.13", "3.13.1", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"rm", "-rf", filepath.Join(root, "Versions/3.13"), filepath.Join(root, "Applications/Python 3.13"),
		filepath.Join(root, "bin/pip3.13"), filepath.Join(root, "bin/python3")}
	if len(p.Steps) != 1 || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, want) {
		t.Fatalf("steps %+v", p.Steps)
	}
	if len(p.Changes) != 4 {
		t.Errorf("changes %q", p.Changes)
	}
	if len(p.Notes) == 0 || p.Notes[0].Text != strings.Join(want[2:], "\n") {
		t.Errorf("notes %+v", p.Notes)
	}
	if _, err := PythonOrgRemove("3.11", "3.11.9", nil); err == nil {
		t.Error("a version not there has a plan")
	}

	// The last version goes with the framework, its Current link aside.
	if err := os.RemoveAll(filepath.Join(root, "Versions/3.13")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("3.12", filepath.Join(root, "Versions/Current")); err != nil {
		t.Fatal(err)
	}
	p, err = PythonOrgRemove("3.12", "3.12.10", nil)
	if err != nil {
		t.Fatal(err)
	}
	if argv := p.Steps[0].Argv; argv[2] != root {
		t.Errorf("argv %q", argv)
	}
}
