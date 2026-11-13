package detect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// sqlServerVersion is empty: SQL Server runs on macOS only in containers.
func sqlServerVersion(context.Context, run.Runner, sysenv.Env, string) string { return "" }

func databaseServices(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	databaseBrewServices(ctx, r, env, d)
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	exe := dockerCommand(env, "lsof")
	if exe == "" {
		d.Warnings = append(d.Warnings, "lsof")
		return
	}
	// +c0 keeps the whole command name, which lsof cuts at 9 characters.
	res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "+c0", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcun"}, Env: env})
	// lsof exits 1 when it finds no listener at all.
	if err != nil || res.Truncated || res.ExitCode != 0 && (res.ExitCode != 1 || len(res.Stdout) > 0) {
		d.Warnings = append(d.Warnings, "lsof")
		return
	}
	d.Listeners = databaseLsof(string(res.Stdout), strconv.Itoa(os.Getuid()))
	var pids []string
	for _, l := range d.Listeners {
		if l.PID != "" && !slices.Contains(pids, l.PID) {
			pids = append(pids, l.PID)
		}
	}
	if len(pids) == 0 {
		return
	}
	// The program and the working folder; lsof exits 1 for a process
	// gone since, and lists the others.
	res, err = r.Run(ctx, run.Cmd{Args: []string{exe, "-a", "-p", strings.Join(pids, ","), "-d", "cwd,txt", "-Fpfn"}, Env: env})
	if err != nil || res.Truncated {
		return
	}
	files := listenerFiles(string(res.Stdout))
	for i, l := range d.Listeners {
		if f, ok := files[l.PID]; ok {
			d.Listeners[i].Path = f.path
			if f.cwd != "" && f.cwd != "/" {
				d.Listeners[i].Dirs = []string{f.cwd}
			}
		}
	}
}

// databaseLsof reads lsof's listeners; uid is this user's.
func databaseLsof(data, uid string) []inventory.DatabaseListener {
	var out []inventory.DatabaseListener
	var pid, process string
	own := false
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, process, own = line[1:], "", false
		case 'c':
			process = line[1:]
		case 'u':
			own = line[1:] == uid
		case 'n':
			if i := strings.LastIndex(line, ":"); i > 0 {
				out = append(out, inventory.DatabaseListener{Address: line[1:i], Port: line[i+1:], Process: process, PID: pid, Own: own})
			}
		}
	}
	return out
}

// listenerFiles reads lsof's working folder and program of each process:
// its first txt file is the program, those after it libraries.
func listenerFiles(data string) map[string]struct{ path, cwd string } {
	out := map[string]struct{ path, cwd string }{}
	var pid, fd string
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, fd = line[1:], ""
		case 'f':
			fd = line[1:]
		case 'n':
			f := out[pid]
			switch {
			case fd == "cwd":
				f.cwd = line[1:]
			case fd == "txt" && f.path == "":
				f.path = line[1:]
			}
			out[pid] = f
		}
	}
	return out
}
