package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestRubyProject(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"literal", "ruby '3.2.6'", "3.2.6"},
		{"range", "ruby '>= 3.2', '< 3.4'", ">= 3.2, < 3.4"},
		{"interpolation", `ruby "#{File.read('.ruby-version')}"`, ""},
		{"dynamic", "ruby File.read('.ruby-version')", ""},
		{"comments", "# ruby '2.6.10'\nruby \"3.2.6\"", "3.2.6"},
		{"no declaration", "gem 'rails'", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range map[string]string{".ruby-version": "3.2.6\n", "Gemfile": tc.data} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := Read(dir)
			if len(p.Requirements) != 2 || p.Requirements[1].Want != tc.want {
				t.Fatalf("%+v", p)
			}
			snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "ruby", Installations: []inventory.Installation{
				{Tool: "ruby", Path: "/rbenv/shims/ruby", OnPath: 1, Source: inventory.Source{Manager: "rbenv", Shim: true}},
				{Tool: "ruby", Version: "3.3.6", Default: true, Source: inventory.Source{Manager: "rbenv"}},
				{Tool: "ruby", Version: "3.2.6", Source: inventory.Source{Manager: "rbenv"}},
			}, Managers: []inventory.Manager{{Name: "rbenv", SwitchOnCD: true, Files: []string{".ruby-version"}}}}}}
			statuses := Resolve(p, snap, nil)
			_, unmet := statuses[0].Unmet()
			if len(statuses) != 1 || statuses[0].Terminal == nil || statuses[0].Terminal.Version != "3.2.6" || unmet {
				t.Fatalf("%+v", statuses)
			}
		})
	}
}
