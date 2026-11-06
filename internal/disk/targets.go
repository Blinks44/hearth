package disk

import (
	"slices"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Targets are the paths to count for snap, each once: the folders of the
// copies of tools and agents, the packages whose manager records no size,
// and caches. The system's copies are left out, and an installer's
// without a manager: their root is a folder they share, as /usr or
// /usr/local, not their own.
func Targets(snap *inventory.Snapshot, caches []inventory.Cache) []string {
	var out []string
	add := func(path string) {
		if path != "" && !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	copies := func(ins []inventory.Installation) {
		for _, in := range ins {
			shared := in.Source.Kind == inventory.KindSystem || in.Source.Kind == inventory.KindInstaller && in.Source.Manager == ""
			if !in.Source.Shim && !shared {
				add(in.Source.Root)
			}
		}
	}
	if snap != nil {
		for _, t := range snap.Tools {
			copies(t.Installations)
			for _, m := range t.PackageManagers {
				for _, p := range m.Packages {
					if p.Size == 0 {
						add(p.Path)
						for _, a := range p.Artifacts {
							add(a)
						}
					}
				}
			}
		}
		for _, a := range snap.Agents {
			copies(a.Installations)
		}
	}
	for _, c := range caches {
		add(c.Path)
	}
	return out
}
