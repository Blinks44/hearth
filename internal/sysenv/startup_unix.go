//go:build !windows

package sysenv

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

// Startup measures how long the user's login shell takes to start, as in
// a new terminal, its startup files and all; and when that is slow, traces
// a second start of zsh or bash to find the lines that take the time. nil
// when the shell did not start. The shell starts with gui, the environment
// a terminal app gets, as Terminal's does.
func Startup(ctx context.Context, r run.Runner, _, gui Env) *inventory.ShellStartup {
	return startup(ctx, r, gui, rand.Text())
}

func startup(ctx context.Context, r run.Runner, gui Env, marker string) *inventory.ShellStartup {
	sh := loginShell(ctx, r, gui)
	base := loginEnv(gui, sh)
	res, err := r.Run(ctx, run.Cmd{Args: append(shellArgs(sh), "exit"), Env: base, Timeout: shellTimeout})
	if err != nil {
		return nil
	}
	s := &inventory.ShellStartup{Shell: filepath.Base(sh), Took: res.Duration}
	if s.Took < StartupTraceFrom {
		return s
	}
	var args []string
	var ps4 string
	switch s.Shell {
	case "zsh":
		// %x and %I are the file and line of the code that runs, and
		// funcfiletrace has a frame for each sourced file, function and eval.
		args = []string{sh, "-o", "promptsubst", "-x", "-i", "-l", "-c", "exit"}
		ps4 = "+" + marker + "|%D{%s.%6.}|%x|%I|${#funcfiletrace}|"
	case "bash":
		// EPOCHREALTIME is bash 5's: the trace of macOS's bash 3.2 has no
		// time and gives no lines.
		args = []string{sh, "-x", "-i", "-l", "-c", "exit"}
		ps4 = "+" + marker + "|${EPOCHREALTIME}|${BASH_SOURCE[0]}|${LINENO}|${#BASH_SOURCE[@]}|"
	default:
		return s
	}
	res, err = r.Run(ctx, run.Cmd{Args: args, Env: base.with("PS4", ps4), Timeout: shellTimeout, MaxOutput: maxTraceBytes})
	if err != nil || res.Truncated {
		return s
	}
	files, lines, traced := attribute(parseTrace(res.Stderr, marker), startupFiles(s.Shell, gui))
	s.Files, s.Lines = slowest(files, lines, traced, s.Took)
	return s
}

// startupFiles reports the files shell reads as it starts, those it runs
// the lines of at the top level.
func startupFiles(shell string, env Env) func(string) bool {
	home := env.Get("HOME")
	var files []string
	switch shell {
	case "zsh":
		zd := env.Get("ZDOTDIR")
		if zd == "" {
			zd = home
		}
		files = []string{"/etc/zshenv", "/etc/zprofile", "/etc/zshrc", "/etc/zlogin"}
		for _, n := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
			files = append(files, filepath.Join(zd, n))
		}
	case "bash":
		files = []string{"/etc/profile", "/etc/bash.bashrc", "/etc/bashrc"}
		for _, n := range []string{".bash_profile", ".bash_login", ".profile", ".bashrc"} {
			files = append(files, filepath.Join(home, n))
		}
	}
	return func(file string) bool {
		return slices.Contains(files, file) || shell == "bash" && strings.HasPrefix(file, "/etc/profile.d/")
	}
}
