package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// The versions a manager installs are the names of its build definitions:
// rbenv's ruby-build among its plugins, asdf's node-build in the plugin;
// a manager without them, or with none found, says nothing.
func TestInstallable(t *testing.T) {
	root := t.TempDir()
	d := sources.Dirs{Rbenv: filepath.Join(root, "rbenv"), Asdf: filepath.Join(root, "asdf"), Pyenv: filepath.Join(root, "pyenv")}
	write := func(dir string, names ...string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(filepath.Join(d.Rbenv, "plugins", "ruby-build", "share", "ruby-build"), "3.4.11", "3.4.12")
	write(filepath.Join(d.Asdf, "plugins", "nodejs", ".node-build", "share", "node-build"), "22.23.4")
	term := sysenv.Env{"PATH=" + filepath.Join(root, "bin")}
	for _, tt := range []struct {
		manager, plugin string
		want            []string
	}{
		{"rbenv", "ruby", []string{"3.4.11", "3.4.12"}},
		{"asdf", "nodejs", []string{"22.23.4"}},
		{"pyenv", "python", nil},
		{"fnm", "nodejs", nil},
	} {
		if got := installable(tt.manager, tt.plugin, term, d); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.manager, got, tt.want)
		}
	}
}
