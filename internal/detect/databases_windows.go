package detect

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func databaseGlobs(env sysenv.Env, command string) []string {
	var out []string
	for _, root := range []string{env.Get("ProgramFiles"), env.Get("ProgramFiles(x86)")} {
		if root != "" {
			for _, pattern := range []string{"PostgreSQL/*/bin", "MySQL/*/bin", "MariaDB*/bin", "Redis"} {
				out = append(out, filepath.Join(root, pattern, command+".exe"))
			}
		}
	}
	return out
}

func databaseServices(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "sc.exe", "queryex", "type=", "service", "state=", "all")
	if !ok {
		d.Warnings = append(d.Warnings, "sc.exe")
		return
	}
	var name string
	for line := range strings.Lines(string(data)) {
		// sc.exe pads its keys: "        STATE              : 4  RUNNING".
		key, value, _ := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if key == "SERVICE_NAME" {
			name = strings.TrimSpace(value)
		}
		if key == "STATE" && inventory.DatabaseKind(name) != "" {
			status := ""
			if strings.Contains(value, "RUNNING") {
				status = "running"
			} else if strings.Contains(value, "STOPPED") {
				status = "stopped"
			}
			databaseService(d, inventory.DatabaseKind(name), name, "system", status)
		}
	}
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "netstat.exe", "-ano", "-p", "tcp")
	if !ok {
		d.Warnings = append(d.Warnings, "netstat")
		return
	}
	for line := range strings.Lines(string(data)) {
		// The state is translated on localized Windows; a listener is the
		// only TCP row without a foreign port.
		f := strings.Fields(line)
		if len(f) != 5 || !strings.HasSuffix(f[2], ":0") {
			continue
		}
		i := strings.LastIndex(f[1], ":")
		if i < 0 {
			continue
		}
		d.Listeners = append(d.Listeners, inventory.DatabaseListener{Address: f[1][:i], Port: f[1][i+1:], PID: f[4]})
	}
}
