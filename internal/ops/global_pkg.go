package ops

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// GlobalPkgAction is what to do with a globally installed language package.
type GlobalPkgAction int

const (
	GlobalPkgUpdate GlobalPkgAction = iota
	GlobalPkgUninstall
)

var (
	globalPkgNamePattern  = regexp.MustCompile(`^(@[A-Za-z0-9._-]+/)?[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
	globalGoModulePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
)

func validGlobalPkgName(s string) bool {
	return globalPkgNamePattern.MatchString(s) &&
		!strings.Contains(s, "..") &&
		!strings.HasPrefix(s, "/") &&
		!strings.HasSuffix(s, "/")
}

func validGoModulePath(s string) bool {
	return globalGoModulePattern.MatchString(s) &&
		!strings.Contains(s, "..") &&
		!strings.HasPrefix(s, "/") &&
		!strings.HasSuffix(s, "/")
}

// GlobalPackagePlan returns a plan that updates or uninstalls pkg through its
// owning manager. ok is false when pkg has no supported manager or invalid
// package identifier.
func GlobalPackagePlan(action GlobalPkgAction, pkg inventory.Package, detect Detect) (Plan, bool) {
	if !validGlobalPkgName(pkg.Name) {
		return Plan{}, false
	}
	argv := globalPackageCommand(action, pkg)
	if argv == nil {
		return Plan{}, false
	}
	p := Plan{
		Steps: []Step{{Argv: argv}},
	}
	switch action {
	case GlobalPkgUpdate:
		p.Title = locales.GlobalPkgUpdateTitle(pkg.Name)
		p.Changes = []string{locales.GlobalPkgUpdateChange(pkg.Name, pkg.Manager)}
	case GlobalPkgUninstall:
		p.Title = locales.GlobalPkgUninstallTitle(pkg.Name)
		p.Changes = []string{locales.GlobalPkgUninstallChange(pkg.Name, pkg.Manager)}
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if action == GlobalPkgUninstall && toolHasPackage(st, pkg.Name, pkg.Manager) {
				return "", errors.New(pkg.Name + " is still installed")
			}
			if action == GlobalPkgUpdate {
				return locales.GlobalPkgUpdateChange(pkg.Name, pkg.Manager), nil
			}
			return locales.GlobalPkgUninstallChange(pkg.Name, pkg.Manager), nil
		}
	}
	return p, true
}

func globalPackageCommand(action GlobalPkgAction, pkg inventory.Package) []string {
	name := pkg.Name
	switch pkg.Manager {
	case "npm":
		if action == GlobalPkgUpdate {
			return []string{"npm", "install", "-g", name + "@latest"}
		}
		return []string{"npm", "uninstall", "-g", name}
	case "pnpm":
		if action == GlobalPkgUpdate {
			return []string{"pnpm", "add", "-g", name + "@latest"}
		}
		return []string{"pnpm", "remove", "-g", name}
	case "yarn":
		if action == GlobalPkgUpdate {
			return []string{"yarn", "global", "upgrade", name}
		}
		return []string{"yarn", "global", "remove", name}
	case "bun":
		if action == GlobalPkgUpdate {
			return []string{"bun", "add", "-g", name + "@latest"}
		}
		return []string{"bun", "remove", "-g", name}
	case "uv":
		if action == GlobalPkgUpdate {
			return []string{"uv", "tool", "upgrade", name}
		}
		return []string{"uv", "tool", "uninstall", name}
	case "pipx":
		if action == GlobalPkgUpdate {
			return []string{"pipx", "upgrade", name}
		}
		return []string{"pipx", "uninstall", name}
	case "cargo":
		if action == GlobalPkgUpdate {
			return []string{"cargo", "install", name}
		}
		return []string{"cargo", "uninstall", name}
	case "gem":
		if action == GlobalPkgUpdate {
			return []string{"gem", "update", name}
		}
		return []string{"gem", "uninstall", "-a", "-x", name}
	case "dotnet":
		if action == GlobalPkgUpdate {
			return []string{"dotnet", "tool", "update", "-g", name}
		}
		return []string{"dotnet", "tool", "uninstall", "-g", name}
	case "composer":
		if action == GlobalPkgUpdate {
			return []string{"composer", "global", "require", name}
		}
		return []string{"composer", "global", "remove", name}
	case "go":
		if action == GlobalPkgUpdate && validGoModulePath(pkg.Module) {
			return []string{"go", "install", pkg.Module + "@latest"}
		}
		return nil
	default:
		return nil
	}
}

func toolHasPackage(st inventory.ToolState, name, manager string) bool {
	if idx := inventory.TerminalCopy(st.Installations); idx >= 0 {
		for _, p := range st.Installations[idx].Packages {
			if p.Name == name && (p.Manager == "" || manager == "" || p.Manager == manager) {
				return true
			}
		}
		return false
	}
	for _, in := range st.Installations {
		for _, p := range in.Packages {
			if p.Name == name && (p.Manager == "" || manager == "" || p.Manager == manager) {
				return true
			}
		}
	}
	return false
}
