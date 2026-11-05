package pkgmgr

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"github.com/Blinks44/hearth/internal/inventory"
)

// wingetPortable reads only entries explicitly tagged by WinGet. The rest of
// ARP includes applications from many installers and is not proof of ownership.
func wingetPortable(m *inventory.PackageManager) {
	var entries []map[string]string
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			root, err := registry.OpenKey(hive, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, registry.ENUMERATE_SUB_KEYS|view)
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			if err != nil {
				m.Warnings = append(m.Warnings, fmt.Sprintf("read WinGet registrations: %v", err))
				continue
			}
			names, err := root.ReadSubKeyNames(-1)
			if err == nil {
				for _, name := range names {
					key, e := registry.OpenKey(root, name, registry.QUERY_VALUE|view)
					if e != nil {
						continue
					}
					values := map[string]string{}
					for _, field := range []string{"WinGetPackageIdentifier", "WinGetInstallerType", "DisplayVersion", "InstallLocation"} {
						values[field], _, _ = key.GetStringValue(field)
					}
					_ = key.Close()
					entries = append(entries, values)
				}
			} else {
				m.Warnings = append(m.Warnings, fmt.Sprintf("enumerate WinGet registrations: %v", err))
			}
			_ = root.Close()
		}
	}
	m.Packages = parseWingetRegistrations(entries)
}
