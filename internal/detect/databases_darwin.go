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
	for _, prefix := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local"} {
		if prefix != "" {
			out = append(out, filepath.Join(prefix, "Cellar", "*", "*", "bin", command))
		}
	}
	if command == "postgres" || command == "psql" {
		out = append(out, "/Applications/Postgres.app/Contents/Versions/*/bin/"+command)
	}
	return out
}

func databaseServices(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	databaseBrewServices(ctx, r, env, d)
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	exe := dockerCommand(env, "lsof")
	if exe == "" {
		d.Warnings = append(d.Warnings, "lsof")
		return
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn"}, Env: env})
	// lsof exits 1 when it finds no listener at all.
	if err != nil || res.Truncated || res.ExitCode != 0 && (res.ExitCode != 1 || len(res.Stdout) > 0) {
		d.Warnings = append(d.Warnings, "lsof")
		return
	}
	d.Listeners = databaseLsof(string(res.Stdout))
}

func databaseLsof(data string) []inventory.DatabaseListener {
	var out []inventory.DatabaseListener
	var pid, process string
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid = line[1:]
		case 'c':
			process = line[1:]
		case 'n':
			if i := strings.LastIndex(line, ":"); i > 0 {
				out = append(out, inventory.DatabaseListener{Address: line[1:i], Port: line[i+1:], Process: process, PID: pid})
			}
		}
	}
	return out
}
