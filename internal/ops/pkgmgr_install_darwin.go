package ops

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PackageManagerInstallPlan returns a plan to install a safe, supported
// package manager on macOS (Homebrew via its signed official .pkg).
func PackageManagerInstallPlan(id string, _ []inventory.PackageManager, env sysenv.Env) (Plan, bool) {
	if id != "brew" {
		return Plan{}, false
	}
	_, steps, notes := homebrew(env)
	if len(steps) == 0 {
		return Plan{}, false
	}
	return Plan{
		Title:           locales.PkgManagerInstallTitle("Homebrew"),
		Why:             locales.PkgManagerInstallWhy("Homebrew"),
		Changes:         []string{locales.PkgManagerInstallChange("Homebrew", "macOS")},
		Steps:           steps,
		Notes:           notes,
		RestartTerminal: true,
	}, true
}
