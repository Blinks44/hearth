package ops

import (
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathRemovable reports that e comes from the user's Path in the Windows
// registry, which Hearth may remove.
func PathRemovable(e inventory.PathEntry, home string) bool {
	return PathFixable(e, home)
}

// PathRemovePlan returns the plan that removes dir from the user's Path in the
// Windows registry.
func PathRemovePlan(_ []inventory.PathEntry, dir string, _ sysenv.Env, terminal Terminal) (Plan, error) {
	before, err := sysenv.UserPath()
	if err != nil {
		return Plan{}, err
	}
	want := sysenv.ExpandPath(dir)
	var kept []string
	removed := false
	for item := range strings.SplitSeq(before, ";") {
		if item == "" {
			continue
		}
		if sysenv.SamePath(sysenv.ExpandPath(item), want) {
			removed = true
			continue
		}
		kept = append(kept, item)
	}
	if !removed {
		return Plan{}, ErrNoFix
	}
	step := Step{UserPath: &UserPathEdit{Before: before, After: strings.Join(kept, ";")}}
	return pathRemovePlan([]Step{step}, locales.PathRemoveWhyRegistry(), dir, terminal), nil
}
