package ai

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func scanLiveAgentProcesses(_ context.Context, _ run.Runner, _ sysenv.Env) []LiveAgentProcess {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	uid := os.Getuid()
	selfPID := strconv.Itoa(os.Getpid())
	var out []LiveAgentProcess
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if pid == selfPID || pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		proc := filepath.Join("/proc", pid)
		fi, err := os.Stat(proc)
		if err != nil {
			continue
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || int(st.Uid) != uid {
			continue
		}
		commRaw, err := os.ReadFile(filepath.Join(proc, "comm"))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(commRaw))
		var args []string
		if raw, err := os.ReadFile(filepath.Join(proc, "cmdline")); err == nil && len(raw) > 0 {
			args = strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		}
		agent := matchAgentProcess(comm, args)
		if agent == "" {
			continue
		}
		cwd, _ := os.Readlink(filepath.Join(proc, "cwd"))
		if cwd == "/" {
			cwd = ""
		}
		out = append(out, LiveAgentProcess{Agent: agent, PID: pid, Cwd: cwd})
	}
	return out
}
