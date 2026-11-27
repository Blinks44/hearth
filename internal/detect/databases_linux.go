package detect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

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
	// -p names the process only for this user's: others' need root.
	data, ok := databaseRun(ctx, r, env, "ss", "-ltnpH")
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
		l := inventory.DatabaseListener{Address: f[3][:i], Port: f[3][i+1:]}
		if m := ssUser.FindStringSubmatch(line); m != nil {
			l.Process, l.PID = m[1], m[2]
			proc := filepath.Join(procDir, l.PID)
			l.Path, _ = os.Readlink(filepath.Join(proc, "exe"))
			if cwd, err := os.Readlink(filepath.Join(proc, "cwd")); err == nil && cwd != "/" {
				l.Dirs = []string{cwd}
			}
			if fi, err := os.Stat(proc); err == nil {
				st, ok := fi.Sys().(*syscall.Stat_t)
				l.Own = ok && int(st.Uid) == os.Getuid()
			}
			readProcMeta(proc, &l)
		}
		d.Listeners = append(d.Listeners, l)
	}
}

func readProcMeta(proc string, l *inventory.DatabaseListener) {
	if raw, err := os.ReadFile(filepath.Join(proc, "cmdline")); err == nil && len(raw) > 0 {
		parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		l.Args = inventory.RedactArgs(parts)
	}
	if ppid, pgid := readProcStatPPIDPGID(filepath.Join(proc, "stat")); ppid != "" || pgid != "" {
		l.PPID = ppid
		l.PGID = pgid
		cur := ppid
		for range 12 {
			if cur == "" || cur == "0" || cur == "1" || cur == l.PID {
				break
			}
			seen := false
			for _, prev := range l.Ancestors {
				if prev == cur {
					seen = true
					break
				}
			}
			if seen {
				break
			}
			l.Ancestors = append(l.Ancestors, cur)
			next, _ := readProcStatPPIDPGID(filepath.Join(procDir, cur, "stat"))
			if next == "" || next == cur {
				break
			}
			cur = next
		}
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "statm")); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) > 1 {
			if rssPages, err := strconv.ParseInt(fields[1], 10, 64); err == nil && rssPages > 0 {
				l.MemoryBytes = rssPages * int64(os.Getpagesize())
			}
		}
	}
}

func readProcStatPPIDPGID(statPath string) (ppid, pgid string) {
	raw, err := os.ReadFile(statPath)
	if err != nil {
		return "", ""
	}
	idx := strings.LastIndex(string(raw), ")")
	if idx < 0 || idx+2 >= len(raw) {
		return "", ""
	}
	fields := strings.Fields(string(raw[idx+2:]))
	if len(fields) > 1 {
		ppid = fields[1]
	}
	if len(fields) > 2 {
		pgid = fields[2]
	}
	return ppid, pgid
}

// ssUser is the first process of ss' users:(("node",pid=4312,fd=21)).
var ssUser = regexp.MustCompile(`users:\(\("((?:[^"\\]|\\.)*)",pid=([0-9]+),`)

// procDir is where Linux shows the processes; tests set their own.
var procDir = "/proc"
