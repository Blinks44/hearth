package projects

import (
	"os"
	"path/filepath"
	"slices"
	"time"
)

// SearchFolders are the folders under home where people usually keep
// code, as Hearth looks into them on its first start.
var SearchFolders = []string{"dev", "code", "projects", "src", "work", "Developer", "repos"}

// Suggestion is a project Suggest found, with when its files last changed.
type Suggestion struct {
	Project Project
	Changed time.Time
}

// Suggest finds the projects in the search folders under home that exist,
// newest first; folders hold one another's projects once. It reads only
// those folders, never the whole home directory.
func Suggest(home string) (folders []string, found []Suggestion) {
	for _, name := range SearchFolders {
		dir := filepath.Join(home, name)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		folders = append(folders, dir)
		_, nested := Find(dir)
		for _, p := range nested {
			if slices.ContainsFunc(found, func(s Suggestion) bool { return s.Project.Dir == p.Dir || within(p.Dir, s.Project.Dir) }) {
				continue
			}
			found = append(found, Suggestion{Project: p, Changed: changed(p)})
		}
	}
	slices.SortStableFunc(found, func(a, b Suggestion) int { return b.Changed.Compare(a.Changed) })
	return folders, found
}

// within reports whether dir is a folder under parent: the package of a
// monorepo Find already found the root of.
func within(dir, parent string) bool {
	rel, err := filepath.Rel(parent, dir)
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel[0] != '.'
}

// changed is when the project last changed, as its files and git's index
// say: a folder's own time changes only as entries come and go.
func changed(p Project) time.Time {
	var last time.Time
	for _, f := range append(slices.Clone(p.Files), filepath.Join(".git", "index")) {
		if fi, err := os.Stat(filepath.Join(p.Dir, f)); err == nil && fi.ModTime().After(last) {
			last = fi.ModTime()
		}
	}
	return last
}
