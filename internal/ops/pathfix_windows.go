package ops

import (
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// userPathSource is how the scan names the user's Path among the sources
// of a PATH entry.
const userPathSource = UserPathName

// PathFixable reports that e comes from the user's Path in the registry,
// which Hearth may take it off.
func PathFixable(e inventory.PathEntry, _ string) bool {
	return slices.ContainsFunc(e.Sources, func(s inventory.PathSource) bool { return s.File == userPathSource })
}

// PathPlan returns the plan that takes dirs off the user's Path in the
// registry: a repeat of a directory earlier in the computer's Path or the
// user's, or one that is not there; all of them when dirs is nil. The
// computer's Path stays as it is: changing it needs the administrator.
// entries are the terminal's PATH as the scan found it; terminal checks
// the result.
func PathPlan(entries []inventory.PathEntry, dirs []string, _ sysenv.Env, terminal Terminal) (Plan, error) {
	before, err := sysenv.UserPath()
	if err != nil {
		return Plan{}, err
	}
	system, err := sysenv.SystemPath()
	if err != nil {
		return Plan{}, err
	}
	want := wanted(entries, dirs)
	seen := slices.DeleteFunc(strings.Split(system, ";"), func(d string) bool { return d == "" })
	var kept []string
	var fixed []PathProblem
	for item := range strings.SplitSeq(before, ";") {
		dir := sysenv.ExpandPath(item)
		i := slices.IndexFunc(want, func(p PathProblem) bool { return sysenv.SamePath(p.Dir, dir) })
		repeat := slices.ContainsFunc(seen, func(d string) bool { return sysenv.SamePath(d, dir) })
		_, statErr := os.Stat(dir)
		missing := errors.Is(statErr, os.ErrNotExist) && !issues.OSDir(dir)
		if dir == "" || i < 0 || !repeat && !missing {
			kept = append(kept, item)
			if dir != "" {
				seen = append(seen, dir)
			}
			continue
		}
		if !slices.ContainsFunc(fixed, func(p PathProblem) bool { return sysenv.SamePath(p.Dir, dir) }) {
			fixed = append(fixed, want[i])
		}
	}
	if len(fixed) == 0 {
		return Plan{}, ErrNoFix
	}
	step := Step{UserPath: &UserPathEdit{Before: before, After: strings.Join(kept, ";")}}
	removed := len(strings.Split(before, ";")) - len(kept)
	return pathPlan([]Step{step}, locales.PathFixWhyRegistry(), locales.PathFixRemove(removed), entries, fixed, terminal), nil
}
