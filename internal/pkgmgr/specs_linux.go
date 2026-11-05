package pkgmgr

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func specs() []spec {
	return []spec{
		{id: "apt", name: "APT", command: "apt", installed: []string{"list", "--installed"}, updates: []string{"list", "--upgradable"}, format: "apt"},
		{id: "dnf", name: "DNF", command: "dnf", installed: []string{"--cacheonly", "list", "--installed"}, updates: []string{"--cacheonly", "check-update"}, format: "rpm", updatesExit: 100},
		{id: "pacman", name: "Pacman", command: "pacman", installed: []string{"-Q"}, updates: []string{"-Qu"}, format: "pairs", updatesExit: 1},
		{id: "zypper", name: "Zypper", command: "zypper", installed: []string{"--no-refresh", "--xmlout", "search", "--installed-only", "--details", "--type", "package"}, updates: []string{"--non-interactive", "--no-refresh", "--xmlout", "list-updates", "--type", "package"}, format: "zypper", updateFormat: "zypper-updates"},
		{id: "apk", name: "APK", command: "apk", installed: []string{"info", "-v"}, updates: []string{"--no-network", "version", "--limit", "<"}, format: "apk", updateFormat: "apk-updates"},
		{id: "snap", name: "Snap", command: "snap", installed: []string{"list"}, format: "snap"},
		{id: "flatpak", name: "Flatpak", command: "flatpak", installed: []string{"list", "--columns=ref,version,installation,origin,commit"}, format: "flatpak"},
		{id: "nix", name: "Nix", command: "nix-env", installed: []string{"--query", "--installed", "--json"}, format: "nix"},
		{id: "brew", name: "Homebrew", command: "brew"},
	}
}
func standardPaths(id string, _ sysenv.Env) []string {
	if id == "brew" {
		return []string{"/home/linuxbrew/.linuxbrew/bin/brew"}
	}
	return nil
}

func localManagers(_ sysenv.Env) []inventory.PackageManager { return nil }

func wingetPackages(_ *inventory.PackageManager) {}
