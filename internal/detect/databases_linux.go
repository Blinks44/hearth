package detect

import (
	"context"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func databaseGlobs(_ sysenv.Env, command string) []string {
	return []string{"/usr/lib/postgresql/*/bin/" + command, "/usr/pgsql-*/bin/" + command, "/usr/sbin/" + command, "/usr/bin/" + command, "/usr/local/bin/" + command,
		"/opt/mssql/bin/" + command, "/opt/mssql-tools18/bin/" + command, "/opt/mssql-tools/bin/" + command}
}

// sqlServerVersion is the version of the mssql-server package, Microsoft's
// only way to install SQL Server on Linux: dpkg's on Ubuntu, rpm's on RHEL.
func sqlServerVersion(ctx context.Context, r run.Runner, env sysenv.Env, _ string) string {
	for _, argv := range [][]string{{"dpkg-query", "-W", "-f=${Version}", "mssql-server"}, {"rpm", "-q", "--queryformat", "%{VERSION}", "mssql-server"}} {
		res, err := r.Run(ctx, run.Cmd{Args: argv, Env: env})
		if err == nil && res.ExitCode == 0 {
			return databaseVersionOf(string(res.Stdout))
		}
	}
	return ""
}

func databaseServices(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "systemctl", "list-units", "--all", "--type=service", "--no-legend", "--plain", "--no-pager")
	if !ok {
		d.Warnings = append(d.Warnings, "systemctl")
		return
	}
	// Debian's postgresql.service runs nothing: it only starts the clusters,
	// each a postgresql@<version>-<name>.service of its own.
	clusters := strings.Contains(string(data), "postgresql@")
	var names, statuses []string
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		kind := inventory.DatabaseKind(f[0])
		if kind == "" || f[0] == "postgresql.service" && clusters {
			continue
		}
		status := ""
		switch {
		case f[3] == "running":
			status = "running"
		case f[2] == "inactive":
			status = "stopped"
		case f[2] == "failed": // as mongod that exited at start
			status = "error"
		}
		names, statuses = append(names, f[0]), append(statuses, status)
	}
	autostart := databaseEnabled(ctx, r, env, names)
	for i, name := range names {
		databaseService(d, inventory.DatabaseKind(name), name, "system", statuses[i], autostart[i])
	}
}

// databaseEnabled says of each of the units names whether it starts with
// the system, as systemctl is-enabled answers for the unit that starts it:
// "on", "off", or "" for a unit not enabled or disabled, as a static one.
func databaseEnabled(ctx context.Context, r run.Runner, env sysenv.Env, names []string) []string {
	out := make([]string, len(names))
	exe := dockerCommand(env, "systemctl")
	if exe == "" || len(names) == 0 {
		return out
	}
	args := []string{exe, "is-enabled"}
	for _, name := range names {
		args = append(args, inventory.SystemdAutostartUnit(name))
	}
	// is-enabled exits 1 unless every unit is enabled: its lines answer.
	res, err := r.Run(ctx, run.Cmd{Args: args, Env: env})
	if err != nil || res.Truncated {
		return out
	}
	lines := strings.Fields(string(res.Stdout))
	if len(lines) != len(names) {
		return out
	}
	for i, state := range lines {
		switch state {
		case "enabled", "enabled-runtime":
			out[i] = "on"
		case "disabled":
			out[i] = "off"
		}
	}
	return out
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "ss", "-ltnH")
	if !ok {
		d.Warnings = append(d.Warnings, "ss")
		return
	}
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		i := strings.LastIndex(f[3], ":")
		if i < 0 {
			continue
		}
		d.Listeners = append(d.Listeners, inventory.DatabaseListener{Address: f[3][:i], Port: f[3][i+1:]})
	}
}
