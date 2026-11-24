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

// A formula with several uncleaned kegs in the Cellar keeps every keg in
// Packages, but reports a single update on the linked keg (or the last
// installed keg when unlinked, as for keg-only formulae).
func TestBrewMultipleKegs(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout("/opt/homebrew\n", "brew", "--prefix")
	f.Stdout(`{
		"formulae": [
			{"name":"simdjson","full_name":"simdjson","linked_keg":"5.2.0","installed":[{"version":"5.0.5"},{"version":"5.1.1"},{"version":"5.2.0"}]},
			{"name":"switched","full_name":"switched","linked_keg":"1.1","installed":[{"version":"1.0"},{"version":"1.1"},{"version":"1.2"}]},
			{"name":"node@24","full_name":"node@24","linked_keg":null,"installed":[{"version":"24.22.3_1","installed_on_request":true},{"version":"24.22.3_2","installed_on_request":true}]}
		],
		"casks": []
	}`, "brew", "info", "--json=v2", "--installed")
	f.Stdout(`{
		"formulae": [
			{"name":"simdjson","installed_versions":["5.0.5","5.1.1","5.2.0"],"current_version":"5.3.0"},
			{"name":"switched","installed_versions":["1.0","1.1","1.2"],"current_version":"2.0"},
			{"name":"node@24","installed_versions":["24.22.3_1","24.22.3_2"],"current_version":"24.22.4"}
		],
		"casks": []
	}`, "brew", "outdated", "--json=v2")
	f.Stdout("", "brew", "tap")
	probe := func(args ...string) ([]byte, error) {
		res, err := f.Run(context.Background(), run.Cmd{Args: append([]string{"brew"}, args...)})
		return res.Stdout, err
	}
	m := inventory.PackageManager{}
	brew(&m, probe)
	if len(m.Warnings) != 0 || !m.UpdatesKnown {
		t.Fatalf("%+v", m)
	}
	want := []struct{ name, version, available string }{
		{"simdjson", "5.0.5", ""},
		{"simdjson", "5.1.1", ""},
		{"simdjson", "5.2.0", "5.3.0"},
		{"switched", "1.0", ""},
		{"switched", "1.1", "2.0"},
		{"switched", "1.2", ""},
		{"node@24", "24.22.3_1", ""},
		{"node@24", "24.22.3_2", "24.22.4"},
	}
	if len(m.Packages) != len(want) {
		t.Fatalf("got %d packages: %+v", len(m.Packages), m.Packages)
	}
	for i, w := range want {
		if p := m.Packages[i]; p.Name != w.name || p.Version != w.version || p.Available != w.available {
			t.Errorf("%d: got %s %s (available %q), want %s %s (available %q)", i, p.Name, p.Version, p.Available, w.name, w.version, w.available)
		}
	}
}
