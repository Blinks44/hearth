package pkgmgr

import (
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// AvailableManager describes a safe, well-supported package manager offered
// for the current OS in the Package Managers "Available" tab.
type AvailableManager struct {
	ID          string
	Name        string
	Recommended bool
	Installable bool
	WebsiteURL  string
	TermsURL    string
}

// UnusedDependencies returns installed dependencies that no other installed
// package currently needs. For managers that report explicit orphan flags
// (APT, Pacman) those are returned; for Homebrew, dependencies with zero
// installed dependents are returned.
func UnusedDependencies(m inventory.PackageManager) []inventory.SystemPackage {
	var orphans []inventory.SystemPackage
	for _, pkg := range m.Packages {
		if pkg.Orphan {
			orphans = append(orphans, pkg)
		}
	}
	if len(orphans) > 0 {
		slices.SortFunc(orphans, func(a, b inventory.SystemPackage) int { return strings.Compare(a.Name, b.Name) })
		return orphans
	}
	if m.ID != "brew" || !m.ReasonsKnown {
		return nil
	}
	needed := make(map[string]bool, len(m.Packages))
	for _, pkg := range m.Packages {
		for _, dep := range pkg.Dependencies {
			needed[dep] = true
		}
	}
	for _, pkg := range m.Packages {
		if pkg.Dependency && !needed[pkg.Name] {
			orphans = append(orphans, pkg)
		}
	}
	slices.SortFunc(orphans, func(a, b inventory.SystemPackage) int { return strings.Compare(a.Name, b.Name) })
	return orphans
}
