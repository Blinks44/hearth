package pkgmgr

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestBrew(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout("/opt/homebrew\n", "brew", "--prefix")

	for _, fixture := range []struct {
		file string
		args []string
	}{
		{"brew-installed.json", []string{"brew", "info", "--json=v2", "--installed"}},
		{"brew-outdated.json", []string{"brew", "outdated", "--json=v2"}},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", fixture.file))
		if err != nil {
			t.Fatal(err)
		}
		f.Stdout(string(data), fixture.args...)
	}

	f.Stdout("custom/tools\n", "brew", "tap")
	probe := func(args ...string) ([]byte, error) {
		res, err := f.Run(context.Background(), run.Cmd{Args: append([]string{"brew"}, args...)})
		return res.Stdout, err
	}
	m := inventory.PackageManager{}
	brew(&m, probe)
	if len(m.Warnings) != 0 || !m.UpdatesKnown || len(m.Packages) != 5 {
		t.Fatalf("%+v", m)
	}
	if p := m.Packages[0]; p.Available != "24.0" || p.Path != filepath.FromSlash("/opt/homebrew/Cellar/node/22.1") || len(p.Dependencies) != 1 {
		t.Fatalf("%+v", p)
	}
	// A tap's formula may list no dependencies: its keg's receipt does.
	if p := m.Packages[2]; !slices.Equal(p.Dependencies, []string{"custom/tools/tool", "icu4c"}) {
		t.Errorf("editor's dependencies %q", p.Dependencies)
	}
	// node was asked for; tool came as a dependency; casks always are
	// asked for.
	if !m.ReasonsKnown || m.Packages[0].Dependency || !m.Packages[1].Dependency || m.Packages[3].Dependency {
		t.Fatalf("reasons: %+v", m.Packages)
	}
	// A tap's formula matches by its full name; a formula and a cask of one
	// name keep their own updates.
	want := []struct{ name, kind, available string }{
		{"custom/tools/tool", "formula", "1.1"},
		{"editor", "formula", ""},
		{"editor", "cask", "3.0"},
		{"app", "cask", ""},
	}
	for i, w := range want {
		if p := m.Packages[i+1]; p.Name != w.name || p.Kind != w.kind || p.Available != w.available {
			t.Fatalf("%d: %+v, want %+v", i+1, p, w)
		}
	}
	// A cask's app is counted with it; its command leads into the app.
	if a := m.Packages[3].Artifacts; len(a) != 1 || a[0] != "/Applications/Editor.app" {
		t.Fatalf("cask artifacts %q", a)
	}
	// A failed update probe keeps installed packages, with an unknown status.
	f.Stdout("invalid json", "brew", "outdated", "--json=v2")
	m = inventory.PackageManager{}
	brew(&m, probe)
	if m.UpdatesKnown || len(m.Packages) != 5 || len(m.Warnings) != 1 {
		t.Fatalf("%+v", m)
	}
}
