package pkgmgr

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// orphans marks packages that the manager's local database reports as
// unneeded dependencies (APT via apt-get --simulate autoremove, Pacman
// via pacman -Qdtq). Failures are ignored so detection never degrades.
func orphans(ctx context.Context, r run.Runner, env sysenv.Env, m *inventory.PackageManager) {
	if len(m.Packages) == 0 {
		return
	}
	switch m.ID {
	case "apt":
		aptGet := "apt-get"
		if filepath.IsAbs(m.Path) {
			aptGet = filepath.Join(filepath.Dir(m.Path), "apt-get")
		}
		res, err := r.Run(ctx, run.Cmd{Args: []string{aptGet, "--simulate", "autoremove"}, Env: env, Timeout: 15 * time.Second, MaxOutput: 4 << 20})
		if err != nil || res.Truncated || res.ExitCode != 0 {
			return
		}
		markAPTOrphans(m, res.Stdout)
	case "pacman":
		res, err := r.Run(ctx, run.Cmd{Args: []string{m.Path, "-Qdtq"}, Env: env, Timeout: 15 * time.Second, MaxOutput: 4 << 20})
		if err != nil || res.Truncated || res.ExitCode != 0 {
			return
		}
		markPacmanOrphans(m, res.Stdout)
	}
}

// markAPTOrphans parses "Remv <pkg>[:<arch>] ..." lines from apt-get -s autoremove.
func markAPTOrphans(m *inventory.PackageManager, data []byte) {
	byName := map[string]bool{}
	byArch := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "Remv ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, arch, hasArch := strings.Cut(fields[1], ":")
		if hasArch {
			byArch[name+":"+arch] = true
		} else {
			byName[name] = true
		}
	}
	for i := range m.Packages {
		p := &m.Packages[i]
		if byName[p.Name] || (p.Architecture != "" && byArch[p.Name+":"+p.Architecture]) {
			p.Orphan = true
		}
	}
}

// markPacmanOrphans marks packages listed one per line by pacman -Qdtq.
func markPacmanOrphans(m *inventory.PackageManager, data []byte) {
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names[name] = true
		}
	}
	for i := range m.Packages {
		if names[m.Packages[i].Name] {
			m.Packages[i].Orphan = true
		}
	}
}
