//go:build !windows && !darwin

package ops

import "time"

func dockerDefaultSocketStep(socketPath string) (Step, []string, []Note) {
	return Step{
		Argv:    []string{"ln", "-sfn", socketPath, dockerDefaultSocket},
		Admin:   true,
		Timeout: 30 * time.Second,
	}, nil, nil
}
