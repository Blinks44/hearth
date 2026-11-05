package detect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// miseFiles are the files mise reads a folder's versions from.
var miseFiles = []string{"mise.toml", ".tool-versions"}

// miseManager tells how mise is set up: its version, and how it is
// hooked in: mise activate in the shell's startup, which only the
// terminal sees, or its shims on the terminal's PATH (term) and the
// apps' (gui). Either way it picks the version by the folder's files.
func miseManager(ctx context.Context, r run.Runner, term, gui sysenv.Env, d sources.Dirs) inventory.Manager {
	m := inventory.Manager{Name: "mise", Files: miseFiles}
	if d.Mise != "" {
		shims := filepath.Join(d.Mise, "shims")
		m.Shims, m.ShimsInGUI = onPath(term, shims), onPath(gui, shims)
	}
	// mise activate exports MISE_SHELL in the terminal's environment; the
	// startup files are read too, for an activation that sets it later.
	m.Activated = term.Get("MISE_SHELL") != "" || startupRuns(term, d.Home, "mise activate")
	m.SwitchOnCD = m.Activated || m.Shims
	if path := miseConfig(d, term); path != "" {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			m.ConfigPath = path
		}
	}
	if paths := sysenv.LookPathAll("mise", term); len(paths) > 0 {
		m.Version = miseVersion(ctx, r, term, paths[0])
	}
	return m
}

// withMise puts how mise is set up in place of a plain mise entry among
// managers.
func withMise(ctx context.Context, r run.Runner, managers []inventory.Manager, term, gui sysenv.Env, d sources.Dirs) []inventory.Manager {
	for i, m := range managers {
		if m.Name == "mise" {
			managers[i] = miseManager(ctx, r, term, gui, d)
		}
	}
	return managers
}

// miseVersionRe finds mise's version, a date like 2025.1.6, in its
// path or its --version output, "2025.1.6 macos-arm64 (2025-01-20)".
var miseVersionRe = regexp.MustCompile(`\b20\d\d\.\d+\.\d+\b`)

// miseVersion is the version of the mise at path: from Homebrew's or
// mise's own directory when the path names it, else from mise --version.
func miseVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	if v := miseVersionRe.FindString(real); v != "" {
		return v
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "--version"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return miseVersionRe.FindString(string(res.Stdout))
}

// onPath reports whether dir is on env's PATH.
func onPath(env sysenv.Env, dir string) bool {
	for _, p := range filepath.SplitList(env.Get("PATH")) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// startupRuns reports whether a shell startup file in home runs command:
// a line, not commented out, that has it.
func startupRuns(env sysenv.Env, home, command string) bool {
	for _, text := range shellStartup(env, home) {
		for line := range strings.Lines(text) {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "#") && strings.Contains(line, command) {
				return true
			}
		}
	}
	return false
}

// miseDefault is the version of tool mise's global config picks, among
// the installed ones; "" when it names none.
func miseDefault(d sources.Dirs, env sysenv.Env, tool string, installed []string) string {
	config := miseConfig(d, env)
	if config == "" {
		return ""
	}
	return pickVersion(miseTool(config, tool), installed)
}
