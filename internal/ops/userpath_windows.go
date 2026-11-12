package ops

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// applyUserPath sets the user's Path to e.After, when it still is
// e.Before, or adds e.Add to it, and tells the running apps, as Explorer,
// that it changed. It returns the value that was, which restoreUserPath
// puts back.
func applyUserPath(e UserPathEdit) (string, error) {
	value, err := sysenv.UserPath()
	if err != nil {
		return "", err
	}
	after := e.After
	if e.Add != "" {
		entries := strings.Split(strings.TrimRight(value, ";"), ";")
		if slices.ContainsFunc(entries, func(d string) bool { return sysenv.SamePath(sysenv.ExpandPath(d), sysenv.ExpandPath(e.Add)) }) {
			return value, nil
		}
		// First, ahead of WindowsApps, whose aliases, as python.exe that
		// opens the Store, would run in place of the entry's.
		after = strings.Join(slices.DeleteFunc(append([]string{e.Add}, entries...), func(d string) bool { return d == "" }), ";")
	} else if value != e.Before {
		return "", fmt.Errorf("%s: %w", UserPathName, ErrChanged)
	}
	return value, sysenv.SetUserPath(after)
}

// restoreUserPath puts back the user's Path as it was before applyUserPath.
func restoreUserPath(before string) error { return sysenv.SetUserPath(before) }
