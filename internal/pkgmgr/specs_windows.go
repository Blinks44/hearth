package pkgmgr

import (
	"encoding/json"
	"os"
	"path/filepath"

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

// Scoop exposes installed manifests locally; executing its PowerShell shim
// would require a shell. Reading the manifests also works without PATH setup.
func localManagers(env sysenv.Env) []inventory.PackageManager {
	var out []inventory.PackageManager
	roots := []string{env.Get("SCOOP"), filepath.Join(env.Get("USERPROFILE"), "scoop"), env.Get("SCOOP_GLOBAL"), filepath.Join(env.Get("ProgramData"), "scoop")}
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) || seen[root] {
			continue
		}
		seen[root] = true
		apps := filepath.Join(root, "apps")
		dirs, err := os.ReadDir(apps)
		if err != nil {
			continue
		}
		m := inventory.PackageManager{ID: "scoop", Name: "Scoop", Prefix: root, Path: filepath.Join(root, "shims", "scoop.ps1")}
		for _, dir := range dirs {
			if !dir.IsDir() || dir.Name() == "scoop" {
				continue
			}
			current := filepath.Join(apps, dir.Name(), "current")
			data, err := os.ReadFile(filepath.Join(current, "manifest.json"))
			if err != nil {
				m.Warnings = append(m.Warnings, dir.Name()+": manifest unavailable")
				continue
			}
			var manifest struct{ Version string }
			if err := json.Unmarshal(data, &manifest); err != nil {
				m.Warnings = append(m.Warnings, dir.Name()+": invalid manifest")
				continue
			}
			m.Packages = append(m.Packages, inventory.SystemPackage{Name: dir.Name(), Version: manifest.Version, Path: current, Kind: "app"})
		}
		out = append(out, m)
	}
	return out
}
