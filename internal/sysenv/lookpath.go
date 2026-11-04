package sysenv

import "path/filepath"

// LookPathAll returns every executable called name in the PATH of env, in
// PATH order, like which -a: the first is the one that runs, the rest are
// shadowed. Paths are as found, symlinks not resolved. Empty and relative
// PATH entries are skipped: they mean the working directory, Hearth's and
// not a terminal's.
func LookPathAll(name string, env Env) []string {
	var found []string
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(env.Get("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		for _, file := range candidates(name, env) {
			if p := filepath.Join(dir, file); executable(p) {
				found = append(found, p)
			}
		}
	}
	return found
}
