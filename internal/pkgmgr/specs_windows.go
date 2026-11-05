package pkgmgr

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func specs() []spec {
	return []spec{
		{id: "winget", name: "WinGet", command: "winget"},
		{id: "choco", name: "Chocolatey", command: "choco", installed: []string{"list", "--limit-output"}, format: "choco"},
	}
}
func standardPaths(_ string, _ sysenv.Env) []string { return nil }

func localManagers(env sysenv.Env) []inventory.PackageManager { return scoopManagers(env) }

func wingetPackages(m *inventory.PackageManager) {
	m.Unlisted = true
	wingetPortable(m)
}
