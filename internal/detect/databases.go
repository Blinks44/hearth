package detect

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Databases finds local server binaries, clients, services and TCP listeners.
// It never starts servers, queries databases or reads connection credentials.
func Databases(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	return detectDatabases(ctx, r, term, gui, databaseGlobs)
}

func detectDatabases(ctx context.Context, r run.Runner, term, gui sysenv.Env, globs func(sysenv.Env, string) []string) (inventory.ToolState, error) {
	d := &inventory.DatabaseState{}
	for _, spec := range []struct{ command, kind string }{{"postgres", "PostgreSQL"}, {"mysqld", "MySQL"}, {"mariadbd", "MariaDB"}, {"redis-server", "Redis"}} {
		f := newFinder(spec.command, term)
		f.lookPath(spec.command, term, gui)
		for _, pattern := range globs(term, spec.command) {
			paths, _ := filepath.Glob(pattern)
			for _, path := range paths {
				f.add(path)
			}
		}
		for _, in := range f.ins {
			// MariaDB links mysqld to mariadbd: one server, found twice.
			if in.Source.Shim || slices.ContainsFunc(d.Servers, func(db inventory.Database) bool { return db.Path == in.RealPath }) {
				continue
			}
			v := versionOf(ctx, r, term, in.Path)
			kind := spec.kind
			if strings.Contains(v, "MariaDB") {
				kind = "MariaDB"
			}
			source := in.Source.Manager
			if source == "" && in.Source.Kind == inventory.KindInstaller {
				source = "installer" // as EDB's PostgreSQL in Program Files
			}
			if source == "" && in.Source.Kind == inventory.KindSystem {
				source = "system"
			}
			path := in.RealPath
			if filepath.Base(path) == "redis-check-rdb" { // Debian links redis-server to this same binary
				path = in.Path
			}
			d.Servers = append(d.Servers, inventory.Database{Kind: kind, Name: kind, Version: databaseVersionOf(v), Path: path, Source: source})
		}
	}
	for _, command := range []string{"psql", "mysql", "mariadb", "redis-cli", "sqlite3"} {
		f := newFinder(command, term)
		f.lookPath(command, term, gui)
		for _, pattern := range globs(term, command) {
			paths, _ := filepath.Glob(pattern)
			for _, path := range paths {
				f.add(path)
			}
		}
		for _, in := range f.ins {
			// MariaDB links mysql to mariadb as well.
			if in.Source.Shim || slices.ContainsFunc(d.Clients, func(c inventory.Installation) bool { return c.RealPath == in.RealPath }) {
				continue
			}
			// mysql --version exits without reading a database or connecting.
			in.Version = databaseVersionOf(versionOf(ctx, r, term, in.Path))
			d.Clients = append(d.Clients, in)
		}
	}
	databaseServices(ctx, r, term, d)
	databaseListeners(ctx, r, term, d)
	return inventory.ToolState{Tool: "databases", Databases: d}, ctx.Err()
}

var databaseVersion = regexp.MustCompile(`[0-9]+\.[0-9]+(?:\.[0-9]+)?`)

// databaseVersionOf finds the version in --version output. mysqld and
// mariadbd print their path first, as /Cellar/mysql@8.4/8.4.3/bin/mysqld
// Ver 8.4.3, so the version after "Ver" wins.
func databaseVersionOf(out string) string {
	if _, after, ok := strings.Cut(out, " Ver "); ok {
		out = after
	}
	return databaseVersion.FindString(out)
}

func databaseRun(ctx context.Context, r run.Runner, env sysenv.Env, command string, args ...string) ([]byte, bool) {
	exe := dockerCommand(env, command)
	if exe == "" {
		return nil, false
	}
	res, err := r.Run(ctx, run.Cmd{Args: append([]string{exe}, args...), Env: env})
	return res.Stdout, err == nil && res.ExitCode == 0 && !res.Truncated
}

func databaseBrewServices(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	if dockerCommand(env, "brew") == "" {
		return
	}
	data, ok := databaseRun(ctx, r, append(slices.Clone(env), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1"), "brew", "services", "list", "--json")
	var rows []struct{ Name, Status string }
	if !ok || json.Unmarshal(data, &rows) != nil {
		d.Warnings = append(d.Warnings, "brew services")
		return
	}
	for _, row := range rows {
		kind := inventory.DatabaseKind(row.Name)
		if kind == "" {
			continue
		}
		status := ""
		switch row.Status {
		case "started":
			status = "running"
		case "none", "stopped":
			status = "stopped"
		case "error":
			status = "error"
		}
		databaseService(d, kind, row.Name, "brew", status)
	}
}

func databaseService(d *inventory.DatabaseState, kind, name, source, status string) {
	candidate := -1
	for i, s := range d.Servers {
		if s.Service != "" || s.Kind != kind || (s.Source != source && source != "system") || (source == "brew" && !strings.Contains(filepath.ToSlash(s.Path), "/"+name+"/")) {
			continue
		}
		if candidate >= 0 {
			candidate = -1
			break
		}
		candidate = i
	}
	if candidate >= 0 {
		d.Servers[candidate].Service = name
		d.Servers[candidate].Status = status
		return
	}
	d.Servers = append(d.Servers, inventory.Database{Kind: kind, Name: name, Service: name, Source: source, Status: status})
}

// dockerDatabases asks only for selected non-secret container fields.
func dockerDatabases(ctx context.Context, r run.Runner, env sysenv.Env, exe, endpoint string, d *inventory.DockerState) {
	format := `{"ID":{{json .ID}},"Name":{{json .Names}},"Image":{{json .Image}},"State":{{json .State}},"Ports":{{json .Ports}},"Project":{{json (.Label "com.docker.compose.project.working_dir")}},"Service":{{json (.Label "com.docker.compose.service")}}}`
	res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "--host", endpoint, "ps", "--all", "--format", format}, Timeout: 3 * time.Second, Env: append(slices.Clone(env), "DOCKER_CONTEXT=")})
	if err != nil || res.ExitCode != 0 || res.Truncated {
		d.DatabaseErr = true
		return
	}
	for line := range strings.Lines(string(res.Stdout)) {
		var row struct{ ID, Name, Image, State, Ports, Project, Service string }
		if json.Unmarshal([]byte(line), &row) != nil {
			d.DatabaseErr = true
			continue
		}
		kind := inventory.DatabaseKind(row.Image)
		if kind == "" {
			continue
		}
		status := "unknown" // paused, restarting, dead
		switch row.State {
		case "running":
			status = "running"
		case "created", "exited":
			status = "stopped"
		}
		db := inventory.Database{Kind: kind, Name: row.Name, Version: row.Image, Source: "docker", Status: status, Project: row.Project, Service: row.Service}
		for _, m := range databasePublished.FindAllStringSubmatch(row.Ports, -1) {
			if !slices.Contains(db.Ports, m[1]) {
				db.Ports = append(db.Ports, m[1])
			}
		}
		if len(db.Ports) == 0 && databaseContainerID.MatchString(row.ID) {
			ports, ok := databaseContainerPorts(ctx, r, env, exe, endpoint, row.ID)
			if !ok {
				d.DatabaseErr = true
			} else {
				db.Ports = ports
			}
		}
		d.Databases = append(d.Databases, db)
	}
}

var databasePublished = regexp.MustCompile(`:([0-9]+)->[0-9]+/tcp`)

var databaseContainerID = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

func databaseContainerPorts(ctx context.Context, r run.Runner, env sysenv.Env, exe, endpoint, id string) ([]string, bool) {
	res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "--host", endpoint, "inspect", "--format", "{{json .HostConfig.PortBindings}}", id}, Timeout: 3 * time.Second, Env: append(slices.Clone(env), "DOCKER_CONTEXT=")})
	var bindings map[string][]struct{ HostPort string }
	if err != nil || res.ExitCode != 0 || res.Truncated || json.Unmarshal(res.Stdout, &bindings) != nil {
		return nil, false
	}
	var out []string
	for target, rows := range bindings {
		if !strings.HasSuffix(target, "/tcp") {
			continue
		}
		for _, row := range rows {
			if row.HostPort != "" && row.HostPort != "0" && !slices.Contains(out, row.HostPort) {
				out = append(out, row.HostPort)
			}
		}
	}
	slices.Sort(out)
	return out, true
}
