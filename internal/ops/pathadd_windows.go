package ops

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathAddPlan returns the plan that adds raw to the user's Path in the Windows
// registry.
func PathAddPlan(entries []inventory.PathEntry, raw string, env sysenv.Env, terminal Terminal) (Plan, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" || strings.ContainsAny(dir, ";\r\n") {
		return Plan{}, errors.New(locales.PathAddInvalid())
	}
	home := env.Get("USERPROFILE")
	if home != "" {
		switch {
		case dir == "~":
			dir = home
		case strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`):
			dir = filepath.Join(home, dir[2:])
		}
	}
	if !filepath.IsAbs(dir) {
		return Plan{}, errors.New(locales.PathAddInvalid())
	}
	dir = filepath.Clean(dir)
	notes, err := validatePathAdd(entries, dir)
	if err != nil {
		return Plan{}, err
	}
	steps := []Step{{UserPath: &UserPathEdit{Add: dir}}}
	return pathAddPlan(steps, locales.PathAddWhyRegistry(), dir, notes, terminal), nil
}
