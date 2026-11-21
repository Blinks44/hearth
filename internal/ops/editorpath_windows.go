package ops

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// EditorPathPlan returns the plan that puts the directory of e.CommandSource
// on the user's Path in the Windows registry.
func EditorPathPlan(e inventory.Editor, _ sysenv.Env, terminal Terminal) (Plan, error) {
	if e.Command == "" || e.CommandSource == "" || !filepath.IsAbs(e.CommandSource) {
		return Plan{}, ErrNoFix
	}
	dir := filepath.Dir(e.CommandSource)
	steps := []Step{{UserPath: &UserPathEdit{Add: dir}}}
	return editorPathPlan(steps, locales.EditorsAddPathWhyRegistry(dir, e.Command), e.Command, dir, terminal), nil
}
