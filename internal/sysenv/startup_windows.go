package sysenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

// Startup measures how long PowerShell takes to start in a new terminal:
// PowerShell 7 when it is there, else Windows PowerShell; once with its
// profiles and once without, which Bare has. PowerShell has no trace with
// time, so Lines are the profiles' lines known to be slow, with none. nil
// when it did not start. It starts with term, a new terminal's
// environment.
func Startup(ctx context.Context, r run.Runner, term, _ Env) *inventory.ShellStartup {
	ps, name := powerShell(term)
	// $PROFILE's paths, in the order PowerShell runs them.
	bare, err := r.Run(ctx, run.Cmd{Args: []string{ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		"$PROFILE.AllUsersAllHosts; $PROFILE.AllUsersCurrentHost; $PROFILE.CurrentUserAllHosts; $PROFILE.CurrentUserCurrentHost"},
		Env: term, Timeout: psTimeout})
	if err != nil || bare.ExitCode != 0 {
		return nil
	}
	full, err := r.Run(ctx, run.Cmd{Args: []string{ps, "-NoLogo", "-NonInteractive", "-Command", "exit"}, Env: term, Timeout: psTimeout})
	if err != nil {
		return nil
	}
	s := &inventory.ShellStartup{Shell: name, Took: full.Duration, Bare: bare.Duration}
	var files []string
	for l := range strings.Lines(string(bare.Stdout)) {
		f := strings.TrimSpace(l)
		if info, err := os.Stat(f); err == nil && info.Mode().IsRegular() {
			files = append(files, f)
			s.Files = append(s.Files, inventory.StartupFile{File: f})
		}
	}
	s.Lines = culpritLines(files)
	return s
}

// psTimeout leaves room for slow profiles.
const psTimeout = 10 * time.Second

// powerShell is PowerShell 7 on the terminal's Path, else Windows
// PowerShell, and its name.
func powerShell(env Env) (path, name string) {
	if found := LookPathAll("pwsh", env); len(found) > 0 {
		return found[0], "pwsh"
	}
	root := env.Get("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "powershell"
}

// culpritLines are the lines of files that load a known culprit, with no
// time: for a shell Hearth cannot trace.
func culpritLines(files []string) []inventory.StartupLine {
	var out []inventory.StartupLine
	for _, f := range files {
		for i, l := range readLines(f) {
			if StartupCulprit(l) != "" {
				out = append(out, inventory.StartupLine{File: f, Line: i + 1, Text: trimLine(l)})
			}
		}
	}
	return out
}
