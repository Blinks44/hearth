package ops

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func pathRemovePlan(steps []Step, why, dir string, terminal Terminal) Plan {
	p := Plan{
		Title:           locales.PathRemovePlanTitle(dir),
		Why:             why,
		Action:          locales.PathRemove(),
		Steps:           steps,
		Changes:         []string{"− " + dir},
		RestartTerminal: true,
	}
	if terminal != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			fresh, err := terminal(ctx)
			if err != nil {
				return "", err
			}
			if !slices.ContainsFunc(filepath.SplitList(fresh.Get("PATH")), func(d string) bool { return sysenv.SamePath(d, dir) }) {
				return locales.PathRemoveCheckOk(dir), nil
			}
			return "", errors.New(locales.PathRemoveCheckFail(dir))
		}
	}
	return p
}
