package ops

import (
	"errors"
	"path/filepath"
	"slices"
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
	edit := &UserPathEdit{Add: dir}
	if before, err := sysenv.UserPath(); err == nil {
		entries := strings.Split(strings.TrimRight(before, ";"), ";")
		if !slices.ContainsFunc(entries, func(d string) bool { return sysenv.SamePath(sysenv.ExpandPath(d), sysenv.ExpandPath(dir)) }) {
			edit.Before = before
			edit.After = strings.Join(slices.DeleteFunc(append([]string{dir}, entries...), func(d string) bool { return d == "" }), ";")
		}
	}
	steps := []Step{{UserPath: edit}}
	return pathAddPlan(steps, locales.PathAddWhyRegistry(), dir, notes, terminal), nil
}
