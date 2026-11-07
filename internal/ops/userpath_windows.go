package ops

import (
	"fmt"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// applyUserPath sets the user's Path to e.After, when it still is
// e.Before, and tells the running apps, as Explorer, that it changed.
func applyUserPath(e UserPathEdit) error {
	value, err := sysenv.UserPath()
	if err != nil {
		return err
	}
	if value != e.Before {
		return fmt.Errorf("%s: %w", UserPathName, ErrChanged)
	}
	return sysenv.SetUserPath(e.After)
}

// restoreUserPath puts back the user's Path as it was before applyUserPath.
func restoreUserPath(e UserPathEdit) error { return sysenv.SetUserPath(e.Before) }
