package ops

import "github.com/Blinks44/hearth/internal/sysenv"

func colimaFirstStart() []string { return nil }

func colimaClientPathEdits(sysenv.Env, string, string) ([]Step, error) { return nil, ErrNoFix }
