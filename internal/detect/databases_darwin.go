package detect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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
	enrichListenersPS(ctx, r, env, pids, d.Listeners)
}

type psMeta struct {
	ppid   string
	pgid   string
	cpu    float64
	mem    int64
	uptime time.Duration
	args   []string
}

func enrichListenersPS(ctx context.Context, r run.Runner, env sysenv.Env, pids []string, listeners []inventory.DatabaseListener) {
	psExe := dockerCommand(env, "ps")
	if psExe == "" {
		psExe = "/bin/ps"
	}
	res, err := r.Run(ctx, run.Cmd{
		Args: []string{psExe, "-ww", "-o", "pid=,ppid=,pgid=,pcpu=,rss=,etime=,args=", "-p", strings.Join(pids, ",")},
		Env:  env,
	})
	if err != nil || res.Truncated || len(res.Stdout) == 0 {
		return
	}
	meta := parseDarwinPS(string(res.Stdout))
	now := time.Now()
	for i, l := range listeners {
		m, ok := meta[l.PID]
		if !ok {
			continue
		}
		listeners[i].PPID = m.ppid
		listeners[i].PGID = m.pgid
		listeners[i].CPUPercent = m.cpu
		listeners[i].MemoryBytes = m.mem
		if m.uptime > 0 {
			listeners[i].StartedAt = now.Add(-m.uptime)
		}
		if len(m.args) > 0 {
			listeners[i].Args = inventory.RedactArgs(m.args)
		}
	}
	enrichListenerAncestors(ctx, r, env, psExe, listeners)
}

func enrichListenerAncestors(ctx context.Context, r run.Runner, env sysenv.Env, psExe string, listeners []inventory.DatabaseListener) {
	res, err := r.Run(ctx, run.Cmd{
		Args: []string{psExe, "-eo", "pid=,ppid="},
		Env:  env,
	})
	if err != nil || res.Truncated || len(res.Stdout) == 0 {
		return
	}
	parents := parseDarwinParentMap(string(res.Stdout))
	for i, l := range listeners {
		if l.PID == "" {
			continue
		}
		listeners[i].Ancestors = walkAncestorPIDs(l.PID, parents, 12)
	}
}

func parseDarwinParentMap(data string) map[string]string {
	out := map[string]string{}
	for line := range strings.Lines(data) {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		out[f[0]] = f[1]
	}
	return out
}

func walkAncestorPIDs(pid string, parents map[string]string, limit int) []string {
	var out []string
	cur := pid
	for range limit {
		ppid := parents[cur]
		if ppid == "" || ppid == "0" || ppid == "1" || ppid == cur || slices.Contains(out, ppid) {
			break
		}
		out = append(out, ppid)
		cur = ppid
	}
	return out
}

func parseDarwinPS(data string) map[string]psMeta {
	out := map[string]psMeta{}
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 6 fixed whitespace-delimited columns: pid, ppid, pgid, pcpu, rss (KiB), etime, then args.
		fields := splitNFields(line, 7)
		if len(fields) < 6 {
			continue
		}
		pid := fields[0]
		cpu, _ := strconv.ParseFloat(strings.ReplaceAll(fields[3], ",", "."), 64)
		rssKB, _ := strconv.ParseInt(fields[4], 10, 64)
		var args []string
		if len(fields) >= 7 {
			args = strings.Fields(fields[6])
		}
		out[pid] = psMeta{
			ppid:   fields[1],
			pgid:   fields[2],
			cpu:    cpu,
			mem:    rssKB * 1024,
			uptime: parseElapsedTime(fields[5]),
			args:   args,
		}
	}
	return out
}

func splitNFields(s string, n int) []string {
	var out []string
	for len(out) < n-1 {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return out
		}
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	s = strings.TrimLeft(s, " \t")
	if s != "" {
		out = append(out, s)
	}
	return out
}

// parseElapsedTime parses ps etime format: [[dd-]hh:]mm:ss.
func parseElapsedTime(s string) time.Duration {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		days, _ = strconv.Atoi(d)
		s = rest
	}
	parts := strings.Split(s, ":")
	var h, m, sec int
	switch len(parts) {
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, _ = strconv.Atoi(parts[1])
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, _ = strconv.Atoi(parts[2])
	default:
		return 0
	}
	return time.Duration(days)*24*time.Hour + time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
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
