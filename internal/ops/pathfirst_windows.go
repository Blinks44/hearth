package ops

import (
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathFirstable reports that Hearth may put the i-th entry of the
// terminal's PATH first in the user's Path in the registry: it comes from
// there, not the computer's Path, and after another entry of the user's.
func PathFirstable(entries []inventory.PathEntry, i int, home string) bool {
	user := func(e inventory.PathEntry) bool { return PathFixable(e, home) }
	machine := slices.ContainsFunc(entries[i].Sources, func(s inventory.PathSource) bool { return s.File != userPathSource })
	return firstable(entries, i) && user(entries[i]) && !machine && slices.ContainsFunc(entries[:i], user)
}

// PathFirstPlan returns the plan that puts dir first in the user's Path in
// the registry. The computer's Path stays ahead of it in a terminal's
// PATH: changing it needs the administrator. entries are the terminal's
// PATH as the scan found it; terminal checks the result.
func PathFirstPlan(entries []inventory.PathEntry, dir string, _ sysenv.Env, terminal Terminal) (Plan, error) {
	i := firstIndex(entries, dir)
	if i < 0 || !PathFirstable(entries, i, "") {
		return Plan{}, ErrNoFix
	}
	before, err := sysenv.UserPath()
	if err != nil {
		return Plan{}, err
	}
	system, err := sysenv.SystemPath()
	if err != nil {
		return Plan{}, err
	}
	items := strings.Split(before, ";")
	j := slices.IndexFunc(items, func(d string) bool { return sysenv.SamePath(sysenv.ExpandPath(d), dir) })
	if j < 0 {
		return Plan{}, ErrNoFix
	}
	after := strings.Join(append([]string{items[j]}, slices.Delete(slices.Clone(items), j, j+1)...), ";")
	var ahead []string
	for _, d := range strings.Split(system, ";") {
		ahead = append(ahead, sysenv.ExpandPath(d))
	}
	at := len(firsts(ahead, func(d string) bool { return sysenv.SamePath(d, dir) }))
	step := Step{UserPath: &UserPathEdit{Before: before, After: after}}
	return pathFirstPlan(step, locales.PathFirstWhyRegistry(), entries, dir, at, terminal), nil
}
