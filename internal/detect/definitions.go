package detect

import (
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// withInstallable sets what the managers that install from build
// definitions can install, for plugin, asdf's name of the tool.
func withInstallable(managers []inventory.Manager, plugin string, term sysenv.Env, d sources.Dirs) []inventory.Manager {
	for i := range managers {
		managers[i].Installable = installable(managers[i].Name, plugin, term, d)
	}
	return managers
}

// installable is the versions a manager's build definitions name, as
// node-build's share/node-build/22.23.4: what it installs now, which lags
// behind the releases for days after one. nil for a manager without them,
// or when Hearth does not find them.
func installable(manager, plugin string, term sysenv.Env, d sources.Dirs) []string {
	var dirs []string
	switch manager {
	case "asdf":
		p := filepath.Join(d.Asdf, "plugins", plugin)
		dirs = []string{
			filepath.Join(p, ".node-build", "share", "node-build"),
			filepath.Join(p, ".ruby-build", "share", "ruby-build"),
			filepath.Join(p, "pyenv", "plugins", "python-build", "share", "python-build"),
		}
	case "nodenv":
		dirs = []string{besideProgram(term, "node-build", "..", "share", "node-build"), filepath.Join(d.Nodenv, "plugins", "node-build", "share", "node-build")}
	case "rbenv":
		dirs = []string{besideProgram(term, "ruby-build", "..", "share", "ruby-build"), filepath.Join(d.Rbenv, "plugins", "ruby-build", "share", "ruby-build")}
	case "pyenv":
		// pyenv keeps python-build among its plugins, from Homebrew too.
		dirs = []string{besideProgram(term, "pyenv", "..", "plugins", "python-build", "share", "python-build"), filepath.Join(d.Pyenv, "plugins", "python-build", "share", "python-build")}
	default:
		return nil
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var out []string
		for _, e := range entries {
			if !e.IsDir() {
				out = append(out, e.Name())
			}
		}
		return out
	}
	return nil
}

// besideProgram is the path rel leads to from the folder of program's real
// file, as found in term's PATH; "" when it is not there.
func besideProgram(term sysenv.Env, program string, rel ...string) string {
	paths := sysenv.LookPathAll(program, term)
	if len(paths) == 0 {
		return ""
	}
	real, err := sysenv.RealPath(paths[0])
	if err != nil {
		return ""
	}
	return filepath.Join(append([]string{filepath.Dir(real)}, rel...)...)
}
