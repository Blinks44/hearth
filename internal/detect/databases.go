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
	for _, spec := range []struct{ command, kind string }{{"postgres", "PostgreSQL"}, {"mysqld", "MySQL"}, {"mariadbd", "MariaDB"}, {"redis-server", "Redis"}, {"mongod", "MongoDB"}, {"sqlservr", "SQL Server"}} {
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
			kind := spec.kind
			var v string
			if kind == "SQL Server" {
				// sqlservr has no --version: running it starts the server.
				v = sqlServerVersion(ctx, r, term, in.RealPath)
			} else {
				v = versionOf(ctx, r, term, in.Path)
			}
			if strings.Contains(v, "MariaDB") {
				kind = "MariaDB"
			}
			// MariaDB's Windows installer puts a copy of mariadbd.exe
			// beside it as mysqld.exe, not a link.
			if slices.ContainsFunc(d.Servers, func(db inventory.Database) bool {
				return db.Kind == kind && filepath.Dir(db.Path) == filepath.Dir(in.RealPath)
			}) {
				continue
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
	for _, command := range []string{"psql", "mysql", "mariadb", "redis-cli", "mongosh", "sqlcmd", "sqlite3"} {
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
			cmd := run.Cmd{Args: []string{in.Path, "--version"}, Env: term}
			if command == "mongosh" {
				cmd.Timeout = 20 * time.Second // a Node program: its first start takes some 10 s
			}
			if res, err := r.Run(ctx, cmd); err == nil && res.ExitCode == 0 {
				in.Version = databaseVersionOf(string(res.Stdout))
			}
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
	var rows []struct{ Name, Status, File string }
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
		// A service started at login is a launchd agent of the user's,
		// or a daemon of the system's for one started as root; one run
		// for now only has the formula's own file.
		autostart := "off"
		if strings.Contains(row.File, "/LaunchAgents/") || strings.Contains(row.File, "/LaunchDaemons/") {
			autostart = "on"
		}
		databaseService(d, kind, row.Name, "brew", status, autostart)
	}
}

func databaseService(d *inventory.DatabaseState, kind, name, source, status, autostart string) {
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
		d.Servers[candidate].Autostart = autostart
		return
	}
	d.Servers = append(d.Servers, inventory.Database{Kind: kind, Name: name, Service: name, Source: source, Status: status, Autostart: autostart})
}

// kindNode counts a node container into its kind cluster.
func kindNode(d *inventory.DockerState, cluster, role, image string, running bool) {
	i := slices.IndexFunc(d.KindClusters, func(k inventory.KindCluster) bool { return k.Name == cluster })
	if i < 0 {
		d.KindClusters = append(d.KindClusters, inventory.KindCluster{Name: cluster})
		i = len(d.KindClusters) - 1
	}
	k := &d.KindClusters[i]
	k.Nodes++
	if running {
		k.Running++
	}
	// kindest/node:v1.31.2@sha256:…
	if _, tag, ok := strings.Cut(image, ":"); ok && role == "control-plane" {
		tag, _, _ = strings.Cut(tag, "@")
		if strings.HasPrefix(tag, "v") {
			k.Version = tag
		}
	}
}

// dockerDatabases asks only for selected non-secret container fields: it
// finds the database containers, port-publishing containers, and kind's node containers.
func dockerDatabases(ctx context.Context, r run.Runner, env sysenv.Env, exe, endpoint string, d *inventory.DockerState) {
	format := `{"ID":{{json .ID}},"Name":{{json .Names}},"Image":{{json .Image}},"State":{{json .State}},"Ports":{{json .Ports}},"Project":{{json (.Label "com.docker.compose.project.working_dir")}},"Service":{{json (.Label "com.docker.compose.service")}},"Kind":{{json (.Label "io.x-k8s.kind.cluster")}},"Role":{{json (.Label "io.x-k8s.kind.role")}}}`
	res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "--host", endpoint, "ps", "--all", "--no-trunc", "--format", format}, Timeout: 3 * time.Second, Env: append(slices.Clone(env), "DOCKER_CONTEXT=")})
	if err != nil || res.ExitCode != 0 || res.Truncated {
		d.DatabaseErr = true
		return
	}
	for line := range strings.Lines(string(res.Stdout)) {
		var row struct{ ID, Name, Image, State, Ports, Project, Service, Kind, Role string }
		if json.Unmarshal([]byte(line), &row) != nil {
			d.DatabaseErr = true
			continue
		}
		ports := publishedTCPPorts(row.Ports)
		if row.State == "running" && len(ports) > 0 {
			d.PortContainers = append(d.PortContainers, inventory.DockerPortContainer{
				ID:     row.ID,
				Name:   row.Name,
				Image:  row.Image,
				Status: row.State,
				Ports:  ports,
			})
		}
		if row.Kind != "" {
			kindNode(d, row.Kind, row.Role, row.Image, row.State == "running")
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
		db := inventory.Database{
			Kind:        kind,
			Name:        row.Name,
			Version:     row.Image,
			Source:      "docker",
			Status:      status,
			Project:     row.Project,
			Service:     row.Service,
			ContainerID: row.ID,
			Ports:       ports,
		}
		if len(db.Ports) == 0 && databaseContainerID.MatchString(row.ID) {
			inspected, ok := databaseContainerPorts(ctx, r, env, exe, endpoint, row.ID)
			if !ok {
				d.DatabaseErr = true
			} else {
				db.Ports = inspected
			}
		}
		d.Databases = append(d.Databases, db)
	}
}

// PortsSnapshot is the result of a targeted Ports refresh without a full scan.
type PortsSnapshot struct {
	Listeners  []inventory.DatabaseListener
	Warnings   []string
	Containers []inventory.DockerPortContainer
	HasDocker  bool
}

// RefreshPorts reads active TCP listeners and, when a local Docker engine is
// active in snap, running containers publishing host TCP ports.
func RefreshPorts(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot) PortsSnapshot {
	d := &inventory.DatabaseState{}
	databaseListeners(ctx, r, env, d)
	out := PortsSnapshot{
		Listeners: d.Listeners,
		Warnings:  d.Warnings,
	}
	if snap == nil {
		return out
	}
	st, ok := snap.Tool("docker")
	if !ok || st.Docker == nil {
		return out
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 || st.Installations[active].Source.Shim {
		return out
	}
	var endpoint string
	for _, c := range st.Docker.Contexts {
		if c.Name == st.Docker.Selected && c.Local {
			endpoint = c.Endpoint
			break
		}
	}
	if endpoint == "" {
		return out
	}
	format := `{"ID":{{json .ID}},"Name":{{json .Names}},"Image":{{json .Image}},"State":{{json .State}},"Ports":{{json .Ports}}}`
	res, err := r.Run(ctx, run.Cmd{
		Args:    []string{st.Installations[active].Path, "--host", endpoint, "ps", "--no-trunc", "--format", format},
		Timeout: 3 * time.Second,
		Env:     append(slices.Clone(env), "DOCKER_CONTEXT="),
	})
	if err != nil || res.ExitCode != 0 || res.Truncated {
		return out
	}
	out.HasDocker = true
	for line := range strings.Lines(string(res.Stdout)) {
		var row struct{ ID, Name, Image, State, Ports string }
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		ports := publishedTCPPorts(row.Ports)
		if len(ports) == 0 {
			continue
		}
		out.Containers = append(out.Containers, inventory.DockerPortContainer{
			ID:     row.ID,
			Name:   row.Name,
			Image:  row.Image,
			Status: row.State,
			Ports:  ports,
		})
	}
	return out
}

func publishedTCPPorts(raw string) []string {
	var ports []string
	for _, m := range databasePublished.FindAllStringSubmatch(raw, -1) {
		if !slices.Contains(ports, m[1]) {
			ports = append(ports, m[1])
		}
	}
	return ports
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
