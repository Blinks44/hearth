// Package pkgmgr reads installed packages and checks availability. Detect is
// local; Network may refresh manager metadata when allowed. Neither installs
// packages, changes profiles or accepts source agreements.
package pkgmgr

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type spec struct {
	id, name, command    string
	installed, updates   []string
	format, updateFormat string
	// updatesExit is an exit code of the updates query that still means
	// success: dnf's 100 when updates exist, pacman's 1 when none do.
	updatesExit int
}

// Detect finds managers in the terminal's and GUI's PATHs, plus standard
// installation locations. One failing probe preserves the other results.
func Detect(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	st := inventory.ToolState{Tool: "packages"}
	for _, s := range specs() {
		if ctx.Err() != nil {
			break
		}
		paths := append(sysenv.LookPathAll(s.command, term), sysenv.LookPathAll(s.command, gui)...)
		paths = append(paths, standardPaths(s.id, term)...)
		seen := map[string]bool{}
		for _, path := range paths {
			real, err := filepath.EvalSymlinks(path)
			if err != nil || seen[real] {
				continue
			}
			seen[real] = true
			m := inventory.PackageManager{ID: s.id, Name: s.name, Path: path}
			env := append(slices.Clone(term), "LC_ALL=C", "LANG=C", "NO_COLOR=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_ENV_HINTS=1")
			probe := func(args ...string) ([]byte, error) {
				res, err := r.Run(ctx, run.Cmd{Args: append([]string{path}, args...), Env: env, Timeout: 15 * time.Second, MaxOutput: 16 << 20})
				if err != nil {
					return nil, fmt.Errorf("%s: %w", strings.Join(args, " "), err)
				}
				if res.Truncated {
					return nil, fmt.Errorf("%s: output truncated", strings.Join(args, " "))
				}
				if s.id == "apk" && slices.Equal(args, s.updates) && len(res.Stderr) > 0 {
					return nil, fmt.Errorf("%s: %s", strings.Join(args, " "), strings.TrimSpace(string(res.Stderr)))
				}
				if !s.exitOK(args, res.ExitCode) {
					return nil, fmt.Errorf("%s: exit %d", strings.Join(args, " "), res.ExitCode)
				}
				return res.Stdout, nil
			}
			if data, err := probe("--version"); err == nil {
				m.Version = firstLine(string(data))
			}
			if s.id == "brew" {
				brew(&m, probe)
				for _, key := range []string{"HOMEBREW_NO_AUTO_UPDATE", "HOMEBREW_NO_ANALYTICS"} {
					value := term.Get(key)
					if value == "" {
						value = "—"
					}
					m.Settings = append(m.Settings, key+"="+value)
				}
			} else if s.id == "winget" {
				wingetPackages(&m)
			} else if s.id == "nix" && nixProfiles(&m, term, probe) {
				// Modern profiles are local JSON, not nix-env environments.
			} else if len(s.installed) == 0 {
				m.Unlisted = true
			} else {
				args := s.installed
				// Chocolatey before 2.0 lists the remote source unless told
				// otherwise.
				if s.id == "choco" && (strings.HasPrefix(m.Version, "0.") || strings.HasPrefix(m.Version, "1.")) {
					args = append(slices.Clone(args), "--local-only")
				}
				data, err := probe(args...)
				if err == nil {
					m.Packages, err = parse(s.format, data)
				}
				if err != nil {
					m.Warnings = append(m.Warnings, err.Error())
				}
				if len(s.updates) > 0 && err == nil {
					data, err = probe(s.updates...)
					if err == nil {
						format := s.updateFormat
						if format == "" {
							format = s.format
						}
						err = applyUpdates(&m, format, data)
					}
					if err != nil {
						m.Warnings = append(m.Warnings, err.Error())
					} else {
						m.UpdatesKnown = true
					}
				}
			}
			slices.SortFunc(m.Packages, func(a, b inventory.SystemPackage) int { return strings.Compare(a.Name, b.Name) })
			st.PackageManagers = append(st.PackageManagers, m)
		}
	}
	st.PackageManagers = append(st.PackageManagers, localManagers(term)...)
	return st, nil
}

// exitOK reports whether a query of args that exited with code succeeded.
func (s spec) exitOK(args []string, code int) bool {
	return code == 0 || code == s.updatesExit && len(s.updates) > 0 && slices.Equal(args, s.updates)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

func applyUpdates(m *inventory.PackageManager, format string, data []byte) error {
	packages, err := parse(format, data)
	if err != nil {
		return err
	}
	for _, u := range packages {
		for i := range m.Packages {
			if m.Packages[i].Name == u.Name && m.Packages[i].Architecture == u.Architecture {
				m.Packages[i].Available = u.Version
			}
		}
	}
	return nil
}
