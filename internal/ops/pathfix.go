package ops

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// ErrNoFix reports PATH problems Hearth cannot fix where they are
// declared: a line that adds other directories too, or one it cannot
// find.
var ErrNoFix = errors.New("nothing Hearth can fix where it is declared")

// Terminal finds the environment a new terminal gets, as a scan does: a
// PATH plan's check uses it.
type Terminal func(ctx context.Context) (sysenv.Env, error)

// PathProblem is a directory of the terminal's PATH Hearth can take off:
// Repeat when it comes earlier in PATH, else it is not there.
type PathProblem struct {
	Dir    string
	Repeat bool
}

// PathProblems are the entries of PATH, in order, that repeat an earlier
// one or name no directory, each once. A repeat the system adds, as
// macOS's path_helper does after brew shellenv, is harmless and the
// system's: it is left, as are the system's own missing directories.
func PathProblems(entries []inventory.PathEntry) []PathProblem {
	var out []PathProblem
	for i, e := range entries {
		var p PathProblem
		switch {
		case slices.ContainsFunc(out, func(p PathProblem) bool { return sysenv.SamePath(p.Dir, e.Dir) }):
			continue
		case !filepath.IsAbs(e.Dir):
			continue
		case slices.ContainsFunc(entries[:i], func(o inventory.PathEntry) bool { return sysenv.SamePath(o.Dir, e.Dir) }):
			if SystemRepeat(e) {
				continue
			}
			p = PathProblem{Dir: e.Dir, Repeat: true}
		case e.Missing && !issues.OSDir(e.Dir):
			p = PathProblem{Dir: e.Dir}
		default:
			continue
		}
		out = append(out, p)
	}
	return out
}

// SystemRepeat reports that the system declares e and adds it again:
// macOS's path_helper, from /etc/paths or /etc/paths.d, after a startup
// file put it first, as Homebrew's brew shellenv does, and Ubuntu's
// profile.d, for the /snap/bin of /etc/environment. The later entry is
// never used, harmless.
func SystemRepeat(e inventory.PathEntry) bool {
	return slices.ContainsFunc(e.Sources, func(s inventory.PathSource) bool {
		return s.File == "/etc/paths" || s.File == "/etc/environment" || strings.HasPrefix(s.File, "/etc/paths.d/")
	})
}

// wanted returns the problems of entries among dirs; all of them when
// dirs is nil.
func wanted(entries []inventory.PathEntry, dirs []string) []PathProblem {
	all := PathProblems(entries)
	if dirs == nil {
		return all
	}
	return slices.DeleteFunc(all, func(p PathProblem) bool {
		return !slices.ContainsFunc(dirs, func(d string) bool { return sysenv.SamePath(d, p.Dir) })
	})
}

// pathPlan returns the plan of steps fixing fixed, checked by a new
// terminal's PATH: every directory still found in the order it was,
// fixed repeats there once, missing ones gone.
func pathPlan(steps []Step, why, action string, entries []inventory.PathEntry, fixed []PathProblem, terminal Terminal) Plan {
	p := Plan{Title: locales.PathFixTitle(len(fixed)), Why: why, Action: action, Steps: steps, RestartTerminal: true}
	for _, f := range fixed {
		p.Changes = append(p.Changes, "− "+f.Dir)
	}
	if terminal == nil {
		return p
	}
	var old []string
	for _, e := range entries {
		old = append(old, e.Dir)
	}
	p.Verify = func(ctx context.Context) (string, error) {
		env, err := terminal(ctx)
		if err != nil {
			return "", err
		}
		return checkPath(old, filepath.SplitList(env.Get("PATH")), fixed)
	}
	return p
}

// checkPath says whether new, a terminal's PATH after a plan, is old with
// the problems fixed: commands are found in the same directories, in the
// same order, repeats come once, missing directories are gone.
func checkPath(old, new []string, fixed []PathProblem) (string, error) {
	gone := func(dir string) bool {
		return slices.ContainsFunc(fixed, func(f PathProblem) bool { return !f.Repeat && sysenv.SamePath(f.Dir, dir) })
	}
	if !slices.EqualFunc(firsts(old, gone), firsts(new, gone), sysenv.SamePath) {
		return "", errors.New(locales.PathCheckOrder())
	}
	for _, f := range fixed {
		n := 0
		for _, d := range new {
			if sysenv.SamePath(d, f.Dir) {
				n++
			}
		}
		if f.Repeat && n != 1 || !f.Repeat && n != 0 {
			return "", errors.New(locales.PathCheckStill(f.Dir))
		}
	}
	return locales.PathChecked(len(new)), nil
}

// firsts are the directories of path in order, each where it comes
// first, without those skip leaves out.
func firsts(path []string, skip func(string) bool) []string {
	var out []string
	for _, d := range path {
		if d == "" || skip(d) || slices.ContainsFunc(out, func(o string) bool { return sysenv.SamePath(o, d) }) {
			continue
		}
		out = append(out, filepath.Clean(d))
	}
	return out
}

// firstable reports that the i-th entry of the terminal's PATH is a
// directory commands could be found in before those of the entries ahead
// of it: there, named whole, and not a repeat of one of them.
func firstable(entries []inventory.PathEntry, i int) bool {
	e := entries[i]
	return i > 0 && filepath.IsAbs(e.Dir) && !e.Missing &&
		!slices.ContainsFunc(entries[:i], func(o inventory.PathEntry) bool { return sysenv.SamePath(o.Dir, e.Dir) })
}

// pathFirstPlan returns the plan of steps putting dir first in PATH, after
// the at directories that stay ahead of it, checked by a new terminal's
// PATH: dir there, the other directories in the order they were.
func pathFirstPlan(steps []Step, why string, entries []inventory.PathEntry, dir string, at int, terminal Terminal) Plan {
	p := Plan{Title: locales.PathFirstTitle(dir), Why: why, Action: locales.PathFirstAction(), Steps: steps,
		Changes: []string{"↑ " + dir}, RestartTerminal: true}
	if terminal == nil {
		return p
	}
	var old []string
	for _, e := range entries {
		old = append(old, e.Dir)
	}
	p.Verify = func(ctx context.Context) (string, error) {
		env, err := terminal(ctx)
		if err != nil {
			return "", err
		}
		return checkFirst(old, filepath.SplitList(env.Get("PATH")), dir, at)
	}
	return p
}

// checkFirst says whether new, a terminal's PATH after a plan, finds
// commands in dir after the at directories ahead of it and in the others
// in the order old did.
func checkFirst(old, new []string, dir string, at int) (string, error) {
	want := firsts(old, func(d string) bool { return sysenv.SamePath(d, dir) })
	at = min(at, len(want))
	want = slices.Insert(want, at, filepath.Clean(dir))
	got := firsts(new, func(string) bool { return false })
	if i := slices.IndexFunc(got, func(d string) bool { return sysenv.SamePath(d, dir) }); i > at {
		// Lines run after Hearth's put others before it, as a version
		// manager's hook does.
		ahead := got[at:i]
		if len(ahead) > 3 {
			ahead = append(ahead[:3:3], "…")
		}
		return "", errors.New(locales.PathFirstAhead(dir, strings.Join(ahead, ", ")))
	}
	if !slices.EqualFunc(want, got, sysenv.SamePath) {
		return "", errors.New(locales.PathCheckOrder())
	}
	return locales.PathFirstChecked(dir), nil
}

// firstIndex is the index of dir's first entry in entries, -1 when none.
func firstIndex(entries []inventory.PathEntry, dir string) int {
	return slices.IndexFunc(entries, func(e inventory.PathEntry) bool { return sysenv.SamePath(e.Dir, dir) })
}
