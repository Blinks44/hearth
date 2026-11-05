package pkgmgr

import (
	"os"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func specs() []spec {
	return []spec{{id: "brew", name: "Homebrew", command: "brew"}, {id: "nix", name: "Nix", command: "nix-env", installed: []string{"--query", "--installed", "--json"}, format: "nix"}}
}
func standardPaths(id string, _ sysenv.Env) []string {
	var paths []string
	if id == "brew" {
		for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if _, err := os.Stat(p); err == nil {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

func localManagers(_ sysenv.Env) []inventory.PackageManager { return nil }
