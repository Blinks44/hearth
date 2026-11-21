//go:build !windows

package ops

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathFixable reports that e comes from a plain PATH line in a startup file of
// the user's (not a tool-manager eval/source hook), where Hearth may comment
// out the line that adds it.
func PathFixable(e inventory.PathEntry, home string) bool {
	return slices.ContainsFunc(e.Sources, func(s inventory.PathSource) bool { return !s.Hook && userLine(s, home) })
}

// PathEntryFixable matches PathPlan's preconditions for a single entry:
// a repeat on Unix needs at least two known declarations so Hearth knows
// which line to keep and which to comment out.
func PathEntryFixable(e inventory.PathEntry, home string, repeat bool) bool {
	if repeat && len(e.Sources) < 2 {
		return false
	}
	return PathFixable(e, home)
}

func userLine(s inventory.PathSource, home string) bool {
	return s.Line > 0 && home != "" && strings.HasPrefix(s.File, home+string(filepath.Separator))
}

// PathPlan returns the plan that takes dirs off entries, the terminal's
// PATH as the scan found it, by commenting out the lines of the user's
// startup files that add them; all the problems when dirs is nil. A repeat
// keeps the line that puts it where commands are found in it. Only a line
// that adds the directory alone is touched: one that adds others too
// stays, and so does the problem. env is the terminal's, which the lines
// are read with; terminal checks the result.
func PathPlan(entries []inventory.PathEntry, dirs []string, env sysenv.Env, terminal Terminal) (Plan, error) {
	home := env.Get("HOME")
	texts := map[string]string{}
	edited := map[string]string{}
	var fixed []PathProblem
	lines := 0
	for _, p := range wanted(entries, dirs) {
		i := slices.IndexFunc(entries, func(e inventory.PathEntry) bool { return sysenv.SamePath(e.Dir, p.Dir) })
		sources := entries[i].Sources
		why := "no such folder"
		if p.Repeat {
			if len(sources) < 2 {
				continue // a repeat Hearth cannot tell the origin of
			}
			keep, err := kept(sources, texts)
			if err != nil {
				return Plan{}, err
			}
			sources, why = slices.Delete(slices.Clone(sources), keep, keep+1), "repeats an earlier PATH entry"
		}
		var commented []inventory.PathSource
		for _, s := range sources {
			if !userLine(s, home) {
				continue
			}
			line, err := sourceLine(s, texts)
			if err != nil {
				return Plan{}, err
			}
			// The file must still have the line, adding p.Dir alone (never a
			// shell eval/source hook that also configures a tool manager).
			if strings.Contains(line, "$(") || strings.Contains(line, "`") || strings.Contains(line, "| source") {
				continue
			}
			added := sysenv.LineDirs(s.File, line, env)
			if len(added) != 1 || !sysenv.SamePath(added[0], p.Dir) {
				continue
			}
			commented = append(commented, s)
		}
		if len(commented) == 0 {
			continue
		}
		for _, s := range commented {
			before, ok := edited[s.File]
			if !ok {
				before = texts[s.File]
			}
			edited[s.File] = CommentOut(before, []int{s.Line}, why)
			lines++
		}
		fixed = append(fixed, p)
	}
	if len(fixed) == 0 {
		return Plan{}, ErrNoFix
	}
	var steps []Step
	for _, file := range slices.Sorted(maps.Keys(edited)) {
		steps = append(steps, Step{Edit: &FileEdit{Path: file, Before: texts[file], After: edited[file]}})
	}
	return pathPlan(steps, locales.PathFixWhy(), locales.PathFixComment(lines), entries, fixed, terminal), nil
}

// kept is the index of the source of a repeated directory that puts it
// where commands are found in it, in the order the shell reads them: the
// last that puts it before the rest of PATH, else the first that puts it
// after.
func kept(sources []inventory.PathSource, texts map[string]string) (int, error) {
	keep := 0
	for i, s := range sources {
		line, err := sourceLine(s, texts)
		if err != nil {
			return 0, err
		}
		if prepends(line) {
			keep = i
		}
	}
	return keep, nil
}

// prepends reports that a line puts its directories before the rest of
// PATH, as PATH="dir:$PATH" and brew shellenv do, not after, as
// PATH="$PATH:dir" and zsh's path+=(dir) do.
func prepends(line string) bool {
	if strings.Contains(line, "path+=") || strings.Contains(line, "PATH+=") {
		return false
	}
	i := strings.Index(line, "$PATH")
	if j := strings.Index(line, "${PATH}"); i < 0 || j >= 0 && j < i {
		i = j
	}
	return i <= 0 || line[i-1] == ':'
}

// sourceLine is the line of its file s names, reading the file into texts
// the first time; "" when the file is shorter now.
func sourceLine(s inventory.PathSource, texts map[string]string) (string, error) {
	text, ok := texts[s.File]
	if !ok {
		data, err := os.ReadFile(s.File)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", s.File, err)
		}
		text = string(data)
		texts[s.File] = text
	}
	all := splitLines(text)
	if s.Line < 1 || s.Line > len(all) {
		return "", nil
	}
	return all[s.Line-1], nil
}
