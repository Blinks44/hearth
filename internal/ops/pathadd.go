package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// validatePathAdd checks that dir is not already in entries and is not an
// existing regular file. A directory that does not exist yet is allowed and
// returns an informational note for the confirmation dialog.
func validatePathAdd(entries []inventory.PathEntry, dir string) ([]Note, error) {
	if slices.ContainsFunc(entries, func(e inventory.PathEntry) bool { return sysenv.SamePath(e.Dir, dir) }) {
		return nil, errors.New(locales.PathAddExists(dir))
	}
	info, err := os.Stat(dir)
	switch {
	case err == nil && !info.IsDir():
		return nil, errors.New(locales.PathAddNotDir(dir))
	case errors.Is(err, os.ErrNotExist):
		return []Note{{Title: locales.PathAddMissingTitle(), Text: locales.PathAddMissingText(dir)}}, nil
	}
	return nil, nil
}

func pathAddPlan(steps []Step, why, dir string, notes []Note, terminal Terminal) Plan {
	p := Plan{
		Title:           locales.PathAddPlanTitle(dir),
		Why:             why,
		Action:          locales.PathAddAction(),
		Steps:           steps,
		Notes:           notes,
		Changes:         []string{"+ " + dir},
		RestartTerminal: true,
	}
	if terminal != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			fresh, err := terminal(ctx)
			if err != nil {
				return "", err
			}
			if slices.ContainsFunc(filepath.SplitList(fresh.Get("PATH")), func(d string) bool { return sysenv.SamePath(d, dir) }) {
				return locales.PathAddCheckOk(dir), nil
			}
			return "", errors.New(locales.PathAddCheckFail(dir))
		}
	}
	return p
}
