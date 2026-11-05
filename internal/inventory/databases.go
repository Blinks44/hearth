package inventory

import "strings"

// Database is an installed server or an existing Docker container. Port
// bindings are observed listeners/published ports, never assumed defaults.
type Database struct {
	Kind, Name, Version, Path, Source, Status, Service, Project string
	Ports                                                       []string
}

// DatabaseState holds local servers, client-only commands and observed TCP listeners.
type DatabaseState struct {
	Servers   []Database
	Clients   []Installation
	Listeners []DatabaseListener
	Warnings  []string
}

// DatabaseListener is an observed TCP listener; no connection is made to it.
type DatabaseListener struct{ Address, Port, Process, PID string }

// DatabaseKind recognizes supported image and service names.
func DatabaseKind(name string) string {
	name = strings.ToLower(name)
	name = name[strings.LastIndex(name, "/")+1:]
	name, _, _ = strings.Cut(name, "@sha256:") // postgres@sha256:…
	name, _, _ = strings.Cut(name, ":")
	name = strings.TrimSuffix(name, ".service")
	for _, spec := range []struct{ prefix, kind string }{{"postgresql", "PostgreSQL"}, {"postgres", "PostgreSQL"}, {"mariadb", "MariaDB"}, {"mysql", "MySQL"}, {"redis", "Redis"}} {
		if name == spec.prefix || name == spec.prefix+"-server" {
			return spec.kind
		}
		rest, ok := strings.CutPrefix(name, spec.prefix)
		if ok && len(rest) > 0 {
			rest = strings.TrimLeft(rest, "@-")
			rest = strings.TrimPrefix(strings.TrimPrefix(rest, "x64-"), "x86-")
			if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
				return spec.kind
			}
		}
	}
	return ""
}

// DatabaseServers combines local installations with containers of the selected local engine.
func DatabaseServers(s *Snapshot) []Database {
	var out []Database
	if st, ok := s.Tool("databases"); ok && st.Databases != nil {
		out = append(out, st.Databases.Servers...)
	}
	if st, ok := s.Tool("docker"); ok && st.Docker != nil {
		out = append(out, st.Docker.Databases...)
	}
	return out
}
