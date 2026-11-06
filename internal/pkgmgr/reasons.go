package pkgmgr

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

const (
	// pacmanDB is where Pacman keeps what it installed and why.
	pacmanDB = "/var/lib/pacman/local"
	// aptStates is where APT marks the packages it installed for others.
	aptStates = "/var/lib/apt/extended_states"
)

// reasons marks the packages another pulled in, where the manager keeps
// it locally, and reports whether it does: APT in its states file, Pacman
// by package in its database. APT's list says "automatic" too, but not of
// a package with an update.
func reasons(m *inventory.PackageManager, states, db string) bool {
	switch m.ID {
	case "apt":
		deps, ok := aptAutomatic(states)
		for i, p := range m.Packages {
			m.Packages[i].Dependency = deps[p.Name]
		}
		return ok
	case "pacman":
		deps, ok := pacmanDependencies(db)
		for i, p := range m.Packages {
			m.Packages[i].Dependency = deps[p.Name]
		}
		return ok
	}
	return false
}

// pacmanDependencies are the packages in Pacman's database whose
// %REASON% is 1, installed as a dependency; false when it cannot be read.
func pacmanDependencies(db string) (map[string]bool, bool) {
	entries, err := os.ReadDir(db)
	if err != nil {
		return nil, false
	}
	deps := map[string]bool{}
	for _, e := range entries {
		f, err := os.Open(filepath.Join(db, e.Name(), "desc"))
		if err != nil {
			continue
		}
		var name, section string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			switch {
			case strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%"):
				section = line
			case line == "":
				section = ""
			case section == "%NAME%":
				name = line
			case section == "%REASON%" && line == "1":
				deps[name] = true
			}
		}
		_ = f.Close()
	}
	return deps, true
}

// aptAutomatic are the packages APT's states file marks Auto-Installed;
// false when it cannot be read.
func aptAutomatic(path string) (map[string]bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	deps := map[string]bool{}
	for _, block := range strings.Split(string(data), "\n\n") {
		var name string
		auto := false
		for _, line := range strings.Split(block, "\n") {
			key, value, _ := strings.Cut(line, ":")
			switch strings.TrimSpace(key) {
			case "Package":
				name = strings.TrimSpace(value)
			case "Auto-Installed":
				auto = strings.TrimSpace(value) == "1"
			}
		}
		if auto {
			deps[name] = true
		}
	}
	return deps, true
}
