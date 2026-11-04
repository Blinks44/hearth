package sources

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ownerQueries ask a Linux package database which package owns a file.
// The first whose program exists answers.
var ownerQueries = []struct {
	manager string
	argv    func(path string) []string
	parse   func(out string) string
}{
	{"apt", func(p string) []string { return []string{"dpkg-query", "-S", p} }, parseDpkg},
	{"rpm", func(p string) []string { return []string{"rpm", "-qf", "--queryformat", "%{NAME}\n", p} }, firstLine},
	{"pacman", func(p string) []string { return []string{"pacman", "-Qqo", p} }, firstLine},
}

// Owner asks the OS package manager which package installed the file at
// path, a system one by Classify. It returns the manager ("apt", "rpm" or
// "pacman") and the package, or empty strings when no package owns the
// file or no package manager is found.
func Owner(ctx context.Context, r run.Runner, env sysenv.Env, path string) (manager, pkg string, err error) {
	for _, q := range ownerQueries {
		res, err := r.Run(ctx, run.Cmd{Args: q.argv(path), Env: env})
		if errors.Is(err, exec.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", fmt.Errorf("owner of %s: %w", path, err)
		}
		if res.ExitCode != 0 {
			return "", "", nil
		}
		if pkg := q.parse(string(res.Stdout)); pkg != "" {
			return q.manager, pkg, nil
		}
		return "", "", nil
	}
	return "", "", nil
}

// parseDpkg reads dpkg-query -S output, "nodejs:amd64, other: /usr/bin/node",
// skipping diversion lines.
func parseDpkg(out string) string {
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, "diversion ") {
			continue
		}
		pkgs, _, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		pkg, _, _ := strings.Cut(pkgs, ", ")
		pkg, _, _ = strings.Cut(pkg, ":")
		return strings.TrimSpace(pkg)
	}
	return ""
}

func firstLine(out string) string {
	line, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(line)
}
