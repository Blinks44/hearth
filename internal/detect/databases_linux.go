package detect

import (
	"context"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func databaseGlobs(_ sysenv.Env, command string) []string {
	return []string{"/usr/lib/postgresql/*/bin/" + command, "/usr/pgsql-*/bin/" + command, "/usr/sbin/" + command, "/usr/bin/" + command, "/usr/local/bin/" + command}
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
		if f[3] == "running" {
			status = "running"
		} else if f[2] == "inactive" {
			status = "stopped"
		}
		databaseService(d, kind, f[0], "system", status)
	}
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
