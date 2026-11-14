package ops

import (
	"github.com/Blinks44/hearth/internal/sysenv"
)

// userVars are the steps that set vars as the user's environment
// variables in the registry, with where they go: apps and terminals
// started after get them.
func userVars(vars []secretVar, _ sysenv.Env) ([]Step, string, error) {
	var steps []Step
	for _, v := range vars {
		steps = append(steps, Step{UserVar: &UserVarEdit{Name: v.Name, Value: v.Value}})
	}
	return steps, UserVarsName, nil
}
