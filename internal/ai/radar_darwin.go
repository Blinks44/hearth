package ai

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func scanLiveAgentProcesses(ctx context.Context, r run.Runner, env sysenv.Env) []LiveAgentProcess {
	if r == nil {
		return nil
	}
	res, err := r.Run(ctx, run.Cmd{
		Args: []string{"/bin/ps", "-ww", "-u", strconv.Itoa(os.Getuid()), "-o", "pid=,args="},
		Env:  env,
	})
	if err != nil || res.Truncated || len(res.Stdout) == 0 {
		return nil
	}
	selfPID := strconv.Itoa(os.Getpid())
	var found []LiveAgentProcess
	var pids []string
	for line := range strings.Lines(string(res.Stdout)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, argsStr, ok := cutField(line)
		if !ok || pid == selfPID {
			continue
		}
		args := strings.Fields(argsStr)
		if len(args) == 0 {
			continue
		}
		agent := matchAgentProcess(args[0], args)
		if agent == "" {
			continue
		}
		found = append(found, LiveAgentProcess{Agent: agent, PID: pid})
		if !slices.Contains(pids, pid) {
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return nil
	}
	lsofExe := "/usr/sbin/lsof"
	if paths := sysenv.LookPathAll("lsof", env); len(paths) > 0 {
		lsofExe = paths[0]
	}
	lres, err := r.Run(ctx, run.Cmd{
		Args: []string{lsofExe, "-a", "-p", strings.Join(pids, ","), "-d", "cwd", "-Fpn"},
		Env:  env,
	})
	if err == nil && !lres.Truncated {
		cwds := parseLsofCWDs(string(lres.Stdout))
		for i := range found {
			if cwd := cwds[found[i].PID]; cwd != "" && cwd != "/" {
				found[i].Cwd = cwd
			}
		}
	}
	return found
}

func cutField(s string) (field, rest string, ok bool) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", "", false
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, "", true
	}
	return s[:i], strings.TrimLeft(s[i:], " \t"), true
}

func parseLsofCWDs(data string) map[string]string {
	out := map[string]string{}
	pid := ""
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid = line[1:]
		case 'n':
			if pid != "" {
				out[pid] = line[1:]
			}
		}
	}
	return out
}
