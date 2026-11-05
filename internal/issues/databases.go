package issues

import (
	"path/filepath"
	"slices"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

// DatabasePortConflict is a literal Compose host port already in use.
const DatabasePortConflict Kind = "database-port-conflict"

// DatabaseConflicts finds occupied Compose ports. A running container of the
// same service and project accounts for its own binding. It does not treat
// .env connections to running local databases as conflicts.
func DatabaseConflicts(s *inventory.Snapshot, ps []projects.Project) []Issue {
	st, ok := s.Tool("databases")
	if !ok || st.Databases == nil {
		return nil
	}
	var out []Issue
	for _, p := range ps {
		for _, ref := range p.Databases {
			if ref.File == ".env" {
				continue
			}
			for _, port := range ref.Ports {
				own := false
				for _, db := range inventory.DatabaseServers(s) {
					if db.Source == "docker" && db.Status == "running" && db.Project != "" && filepath.Clean(db.Project) == filepath.Clean(p.Dir) && db.Service == ref.Service && slices.Contains(db.Ports, port) {
						own = true
					}
				}
				if own {
					continue
				}
				occupied := slices.ContainsFunc(st.Databases.Listeners, func(l inventory.DatabaseListener) bool { return l.Port == port })
				if !occupied {
					for _, db := range inventory.DatabaseServers(s) {
						if db.Status == "running" && slices.Contains(db.Ports, port) {
							occupied = true
						}
					}
				}
				if occupied {
					out = append(out, Issue{Kind: DatabasePortConflict, Tool: "databases", Severity: Warning, Project: p.Dir, Dirs: []string{port}})
				}
			}
		}
	}
	return out
}
