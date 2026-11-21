package ops

import (
	"context"
	"errors"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func editorPathPlan(steps []Step, why, command, dir string, terminal Terminal) Plan {
	p := Plan{
		Title:           locales.EditorsAddPathTitle(command),
		Why:             why,
		Steps:           steps,
		Changes:         []string{locales.EditorsAddPathChange(dir)},
		RestartTerminal: true,
	}
	if terminal != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			fresh, err := terminal(ctx)
			if err != nil {
				return "", err
			}
			if paths := sysenv.LookPathAll(command, fresh); len(paths) > 0 {
				return locales.EditorsAddPathCheckOk(command, paths[0]), nil
			}
			return "", errors.New(locales.EditorsAddPathCheckFail(command))
		}
	}
	return p
}
