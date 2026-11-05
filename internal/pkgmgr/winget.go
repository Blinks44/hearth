package pkgmgr

import (
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

func parseWingetRegistrations(entries []map[string]string) []inventory.SystemPackage {
	var out []inventory.SystemPackage
	seen := map[string]bool{}
	for _, values := range entries {
		id, kind := values["WinGetPackageIdentifier"], values["WinGetInstallerType"]
		if id == "" || kind != "" && !strings.EqualFold(kind, "portable") {
			continue
		}
		version, path := values["DisplayVersion"], values["InstallLocation"]
		identity := strings.ToLower(id + "\x00" + path + "\x00" + version)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		out = append(out, inventory.SystemPackage{Name: id, Version: version, Path: path, Kind: "portable"})
	}
	return out
}
