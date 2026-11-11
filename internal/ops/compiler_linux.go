package ops

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// compilerWait is zero: the package manager is done when it exits.
const compilerWait = 0

// compilerPackages are the packages of each distribution's manager with
// gcc, g++, make and the C library's headers.
var compilerPackages = []struct {
	manager string
	names   []string
}{
	{"apt", []string{"build-essential"}},
	{"dnf", []string{"gcc", "gcc-c++", "make"}},
	{"zypper", []string{"gcc", "gcc-c++", "make"}},
	{"pacman", []string{"base-devel"}},
	{"apk", []string{"build-base"}},
}

// compilerPlan installs the build packages through the first of the
// distribution's managers found.
func compilerPlan(managers []inventory.PackageManager) (Plan, bool) {
	for _, want := range compilerPackages {
		for _, m := range managers {
			if m.ID != want.manager {
				continue
			}
			pkgs := make([]inventory.SystemPackage, len(want.names))
			for i, name := range want.names {
				pkgs[i] = inventory.SystemPackage{Name: name}
			}
			p, ok := PackagePlan(PackageInstall, m, pkgs, nil)
			p.Why = locales.CompilerWhyLinux(m.Name)
			return p, ok
		}
	}
	return Plan{}, false
}
