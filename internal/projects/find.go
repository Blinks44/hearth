package projects

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// findDepth is how deep under a folder Find looks for projects, as
	// apps/web or packages/ui/src.
	findDepth = 3
	// findLimit is how many folders Find looks into at most, so that a
	// folder as big as the home directory takes no long.
	findLimit = 2000
)

// skipped are folders that hold dependencies or build output, not
// projects of their own.
var skipped = map[string]bool{
	"node_modules": true, "vendor": true, "target": true, "dist": true,
	"build": true, "out": true, "venv": true, "__pycache__": true, "testdata": true,
}

// Find reads the project in dir and the projects nested in it, as the
// packages of a monorepo: the folders under it that have project files
// of their own, by path.
func Find(dir string) (root Project, nested []Project) {
	root = Read(dir)
	level := []string{dir}
	seen := 0
	for depth := 0; depth < findDepth && len(level) > 0 && seen < findLimit; depth++ {
		var next []string
		for _, d := range level {
			entries, err := os.ReadDir(d)
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if !e.IsDir() || strings.HasPrefix(name, ".") || skipped[name] || seen >= findLimit {
					continue
				}
				seen++
				sub := filepath.Join(d, name)
				if p := Read(sub); len(p.Files) > 0 {
					nested = append(nested, p)
				}
				next = append(next, sub)
			}
		}
		level = next
	}
	return root, nested
}

// Versions are the requirements of p that name a version of p's own, as
// .nvmrc's, not inherited ones or files that only tell a tool is needed.
func (p Project) Versions() []Requirement {
	var out []Requirement
	for _, r := range p.Requirements {
		if r.Want != "" && !r.Inherited() {
			out = append(out, r)
		}
	}
	return out
}
