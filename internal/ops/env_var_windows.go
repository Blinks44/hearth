package ops

import (
	"fmt"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func setEnvVarSteps(name, value string, _ sysenv.Env) ([]Step, error) {
	cur, ok, err := sysenv.UserVar(name)
	if err != nil {
		return nil, fmt.Errorf("read user env %s: %w", name, err)
	}
	edit := &UserVarEdit{
		Name:    name,
		Value:   value,
		Before:  cur,
		Replace: ok,
		Public:  true,
	}
	return []Step{{UserVar: edit}}, nil
}
