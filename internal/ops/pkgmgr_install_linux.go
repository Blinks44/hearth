package ops

import (
	"slices"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PackageManagerInstallPlan returns a plan to install a safe, supported
// package manager on Linux (Flatpak through the detected distro manager).
func PackageManagerInstallPlan(id string, managers []inventory.PackageManager, _ sysenv.Env) (Plan, bool) {
	if id != "flatpak" {
		return Plan{}, false
	}
	for _, m := range managers {
		if !slices.Contains(linuxManagers, m.ID) || m.Path == "" {
			continue
		}
		p, ok := PackagePlan(PackageInstall, m, []inventory.SystemPackage{{Name: "flatpak"}}, nil)
		if !ok {
			continue
		}
		p.Title = locales.PkgManagerInstallTitle("Flatpak")
		p.Why = locales.PkgManagerInstallWhy("Flatpak")
		return p, true
	}
	return Plan{}, false
}
