//go:build !windows

package ops

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathRemovable reports that e has a PATH declaration inside Hearth's managed
// block in one of the user's shell startup files, which Hearth may remove.
func PathRemovable(e inventory.PathEntry, home string) bool {
	return slices.ContainsFunc(e.Sources, func(s inventory.PathSource) bool { return s.Hearth && !s.Hook && userLine(s, home) })
}

// PathRemovePlan returns the plan that removes dir from Hearth's managed block
// in the user's shell startup files. Lines outside Hearth's block are never
// touched.
func PathRemovePlan(entries []inventory.PathEntry, dir string, env sysenv.Env, terminal Terminal) (Plan, error) {
	home := env.Get("HOME")
	files := map[string]bool{}
	for _, e := range entries {
		if !sysenv.SamePath(e.Dir, dir) {
			continue
		}
		for _, s := range e.Sources {
			if s.Hearth && !s.Hook && userLine(s, home) {
				files[s.File] = true
			}
		}
	}
	if len(files) == 0 {
		return Plan{}, ErrNoFix
	}
	var steps []Step
	for _, file := range slices.Sorted(maps.Keys(files)) {
		data, err := os.ReadFile(file)
		if err != nil {
			return Plan{}, fmt.Errorf("read %s: %w", file, err)
		}
		before := string(data)
		all := splitLines(before)
		start, end := blockLines(all)
		if start < 0 {
			continue
		}
		var keptBody []string
		removed := false
		for _, line := range all[start+1 : end] {
			if !strings.Contains(line, "$(") && !strings.Contains(line, "`") && !strings.Contains(line, "| source") {
				added := sysenv.LineDirs(file, line, env)
				if len(added) == 1 && sysenv.SamePath(added[0], dir) {
					removed = true
					continue
				}
			}
			keptBody = append(keptBody, line)
		}
		if !removed {
			continue
		}
		after := Block(before, strings.Join(keptBody, "\n"))
		steps = append(steps, Step{Edit: &FileEdit{Path: file, Before: before, After: after}})
	}
	if len(steps) == 0 {
		return Plan{}, ErrNoFix
	}
	return pathRemovePlan(steps, locales.PathRemoveWhy(), dir, terminal), nil
}
