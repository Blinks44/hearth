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
		{id: "zypper", name: "Zypper", command: "zypper", installed: []string{"--no-refresh", "--xmlout", "search", "--installed-only", "--type", "package"}, format: "zypper"},
		{id: "apk", name: "APK", command: "apk", installed: []string{"info", "-v"}, format: "apk"},
		{id: "snap", name: "Snap", command: "snap", installed: []string{"list"}, format: "snap"},
		{id: "flatpak", name: "Flatpak", command: "flatpak", installed: []string{"list", "--columns=application,version,installation"}, format: "tsv"},
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
