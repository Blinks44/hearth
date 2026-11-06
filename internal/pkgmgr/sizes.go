package pkgmgr

import (
	"os"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// dpkgStatus is where dpkg records the installed packages, with their
// sizes.
const dpkgStatus = "/var/lib/dpkg/status"

// recordedSizes fills the sizes the manager records of its packages, so
// that they need no count: dpkg's Installed-Size, in KiB, and Pacman's
// %SIZE%, in bytes.
func recordedSizes(m *inventory.PackageManager, status, db string) {
	var sizes map[string]int64
	switch m.ID {
	case "apt":
		sizes = dpkgSizes(status)
	case "pacman":
		descs, _ := pacmanDescs(db)
		sizes = map[string]int64{}
		for name, desc := range descs {
			if n, err := strconv.ParseInt(desc["%SIZE%"], 10, 64); err == nil {
				sizes[name] = n
			}
		}
	}
	for i, p := range m.Packages {
		m.Packages[i].Size = sizes[p.Name]
	}
}

// dpkgSizes are the Installed-Size of the packages in dpkg's status file,
// in bytes, by name; a package of several architectures has the first's.
func dpkgSizes(path string) map[string]int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sizes := map[string]int64{}
	for block := range strings.SplitSeq(string(data), "\n\n") {
		var name string
		var size int64
		for line := range strings.SplitSeq(block, "\n") {
			key, value, _ := strings.Cut(line, ":")
			switch key {
			case "Package":
				name = strings.TrimSpace(value)
			case "Installed-Size":
				if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
					size = n * 1024
				}
			}
		}
		if _, ok := sizes[name]; !ok && name != "" && size > 0 {
			sizes[name] = size
		}
	}
	return sizes
}
