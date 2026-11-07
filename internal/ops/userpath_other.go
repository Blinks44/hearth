//go:build !windows

package ops

import "errors"

var errNoRegistry = errors.New("the user's Path in the registry is only on Windows")

func applyUserPath(UserPathEdit) error   { return errNoRegistry }
func restoreUserPath(UserPathEdit) error { return errNoRegistry }
