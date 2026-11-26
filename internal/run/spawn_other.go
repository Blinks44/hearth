//go:build !darwin

package run

import "context"

func runPlatform(context.Context, Cmd, int) (Result, bool, error) {
	return Result{}, false, nil
}
