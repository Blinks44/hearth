package detect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// nodeManagers tells how the version managers that keep installations in
// ins are set up: whether they switch versions as the terminal enters a
// folder, and by which files.
func nodeManagers(ins []inventory.Installation, env sysenv.Env, home string) []inventory.Manager {
	var managers []inventory.Manager
	var rc []string // the shell's startup files, read once when needed
	hooked := func(marks ...string) bool {
		if rc == nil {
			rc = shellStartup(env, home)
		}
		for _, text := range rc {
			for line := range strings.Lines(text) {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "#") {
					continue
				}
				for _, m := range marks {
					if strings.Contains(line, m) {
						return true
					}
				}
			}
		}
		return false
	}
	for _, in := range ins {
		name := in.Source.Manager
		if in.Source.Kind != inventory.KindVersionManager || slices.ContainsFunc(managers, func(m inventory.Manager) bool { return m.Name == name }) {
			continue
		}
		m := inventory.Manager{Name: name}
		switch name {
		case "fnm":
			// fnm env --use-on-cd in the shell's startup adds the hook.
			m.SwitchOnCD, m.Files = hooked("--use-on-cd"), []string{".node-version", ".nvmrc"}
		case "nvm":
			// nvm has no hook of its own; its README's snippets define one.
			m.SwitchOnCD, m.Files = hooked("load-nvmrc", "cdnvm"), []string{".nvmrc"}
		case "volta":
			// Volta's shims read the project's package.json on every run.
			m.SwitchOnCD, m.Files = true, []string{"package.json"}
		case "mise":
			m.SwitchOnCD, m.Files = true, miseFiles
		case "asdf":
			m.SwitchOnCD, m.Files = true, []string{".tool-versions"}
		case "nodenv":
			m.SwitchOnCD, m.Files = true, []string{".node-version"}
		case "n":
			// n keeps versions in <prefix>/n/versions/node/<version> and
			// copies the one in use into the prefix.
			prefix := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(in.Source.Root))))
			m.Admin = !writable(prefix)
		}
		if !m.SwitchOnCD {
			m.Files = nil
		}
		managers = append(managers, m)
	}
	return managers
}

// corepackEnabled reports whether corepack enable put its pnpm or yarn in
// one of dirs: beside node, or beside npm -g's commands for a Corepack
// installed with npm, as Node 25 and later need.
func corepackEnabled(dirs ...string) bool {
	for _, dir := range dirs {
		for _, name := range corepackShims {
			p := filepath.Join(dir, name)
			if real, err := sysenv.RealPath(p); err == nil && strings.Contains(real, "corepack") {
				return true
			}
			if strings.Contains(readHead(p), "corepack") {
				return true
			}
		}
	}
	return false
}

// readHead is the start of a file, enough for a script's first lines; ""
// when it cannot be read.
func readHead(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }() // read only
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

// commands reads the commands of a package.json's bin: a path, named
// after the package without its scope, or commands by name.
func commands(name string, bin any) []string {
	switch b := bin.(type) {
	case string:
		_, short, ok := strings.Cut(name, "/")
		if !ok {
			short = name
		}
		return []string{short}
	case map[string]any:
		var names []string
		for k := range b {
			names = append(names, k)
		}
		slices.Sort(names)
		return names
	}
	return nil
}
