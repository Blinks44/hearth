package pkgmgr

import (
	"context"
	"fmt"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Reread lists m's packages again, as Detect does, uncached: an
// operation's check uses it. WinGet's packages come from its own list,
// which Detect leaves to the network query.
func Reread(ctx context.Context, r run.Runner, env sysenv.Env, m inventory.PackageManager) (inventory.PackageManager, error) {
	switch m.ID {
	case "winget":
		packages, err := queryNetwork(ctx, r, env, m)
		if len(packages) == 0 && err != nil {
			return m, err
		}
		m.Packages = packages
		return m, nil
	case "scoop":
		for _, found := range localManagers(env) {
			if found.Path == m.Path {
				return found, nil
			}
		}
		return m, fmt.Errorf("scoop is gone from %s", m.Prefix)
	}
	for _, s := range specs() {
		if s.id == m.ID {
			found := detectManager(ctx, r, env, s, m.Path)
			if len(found.Packages) == 0 && len(found.Warnings) > 0 {
				return found, fmt.Errorf("list packages: %s", found.Warnings[0])
			}
			return found, nil
		}
	}
	return m, fmt.Errorf("unknown package manager %s", m.ID)
}
