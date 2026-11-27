package projects

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SearchFolders are the folders under home where people usually keep
// code, as Hearth looks into them on its first start.
var SearchFolders = []string{"dev", "code", "projects", "src", "work", "Developer", "repos"}

// StaleAfter is how long unchanged a project is likely a try that was
// left: one found is offered switched off, and its installs to clean up.
const StaleAfter = 180 * 24 * time.Hour

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
			found = append(found, Suggestion{Project: p, Changed: p.Changed})
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

// changed is when the project last changed, as its files and enclosing git
// repository (including monorepo roots and git worktrees) say: a folder's own
// time changes only as entries come and go.
func changed(p Project) time.Time {
	var last time.Time
	consider := func(path string) {
		if fi, err := os.Stat(path); err == nil && fi.ModTime().After(last) {
			last = fi.ModTime()
		}
	}
	for _, f := range p.Files {
		if strings.ContainsAny(f, "*?[") {
			matches, err := filepath.Glob(filepath.Join(p.Dir, f))
			if err != nil {
				continue
			}
			for _, m := range matches {
				consider(m)
			}
			continue
		}
		consider(filepath.Join(p.Dir, f))
	}
	if gitTime := gitChangedTime(p.Dir); gitTime.After(last) {
		last = gitTime
	}
	return last
}

// gitChangedTime walks up from dir (up to 8 levels) to find the nearest .git
// directory or worktree/submodule .git file and returns the newest mtime among
// its index, HEAD, and logs/HEAD files.
func gitChangedTime(dir string) time.Time {
	cur := filepath.Clean(dir)
	for range 8 {
		gitPath := filepath.Join(cur, ".git")
		fi, err := os.Stat(gitPath)
		if err == nil {
			gitDir := gitPath
			if !fi.IsDir() {
				gitDir = resolveGitDirFile(gitPath)
			}
			if gitDir == "" {
				return time.Time{}
			}
			return newestGitMeta(gitDir)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return time.Time{}
}

func resolveGitDirFile(gitFile string) string {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	line = strings.TrimSpace(line)
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return ""
	}
	target := strings.TrimSpace(line[len(prefix):])
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(gitFile), target)
	}
	return filepath.Clean(target)
}

func newestGitMeta(gitDir string) time.Time {
	var last time.Time
	for _, rel := range []string{"index", "HEAD", filepath.Join("logs", "HEAD")} {
		if fi, err := os.Stat(filepath.Join(gitDir, rel)); err == nil && fi.ModTime().After(last) {
			last = fi.ModTime()
		}
	}
	return last
}
