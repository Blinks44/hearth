//go:build windows

package ops

import (
	"github.com/Blinks44/hearth/internal/sysenv"
)

func awsProfileSteps(profile string, _ sysenv.Env) ([]Step, []Note, error) {
	prev, ok, err := sysenv.UserVar("AWS_PROFILE")
	if err != nil || !ok {
		prev = ""
		ok = false
	}
	if profile == "" || profile == "default" {
		if !ok {
			return nil, nil, nil
		}
		return []Step{{UserVar: &UserVarEdit{
			Name:   "AWS_PROFILE",
			Before: prev,
			Delete: true,
			Public: true,
		}}}, nil, nil
	}
	if ok && prev == profile {
		return nil, nil, nil
	}
	return []Step{{UserVar: &UserVarEdit{
		Name:    "AWS_PROFILE",
		Value:   profile,
		Before:  prev,
		Replace: true,
		Public:  true,
	}}}, nil, nil
}
