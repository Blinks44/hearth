package projects

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// Status is how a project's requirements of one tool stand against a scan.
type Status struct {
	Tool string
	// Checked reports that the scan looked for the tool; when it did not,
	// Terminal and the checks' Installed are empty.
	Checked bool
	// Terminal is the copy the terminal runs in the project folder; nil
	// when there is none.
	Terminal *inventory.Installation
	// Downloads reports that Terminal is not installed: Corepack
	// downloads that packageManager version on first use in the project.
	Downloads bool
	// Installations are the tool's copies the scan found.
	Installations []inventory.Installation
	Checks        []Check
}

// Check is one requirement against what is installed.
type Check struct {
	Requirement
	// Installed is the newest installed copy that meets it; nil when none.
	Installed *inventory.Installation
	// Met reports that Terminal meets it.
	Met bool
	// Err says why Want could not be understood; the check then meets
	// nothing.
	Err string
}

// Unmet returns the first requirement the terminal does not meet in the
// project, when the scan checked the tool; one that could not be
// understood is not counted.
func (st Status) Unmet() (Check, bool) {
	if !st.Checked {
		return Check{}, false
	}
	for _, ch := range st.Checks {
		if ch.Err == "" && !ch.Met {
			return ch, true
		}
	}
	return Check{}, false
}

// Resolve checks p's requirements against the scan s, a status per tool in
// the order the tools first come in p. lines are Node's release lines,
// which lts/* and lts/<codename> need; without them those meet nothing.
func Resolve(p Project, s *inventory.Snapshot, lines map[int]versions.Line) []Status {
	var out []Status
	for _, req := range p.Requirements {
		i := slices.IndexFunc(out, func(st Status) bool { return st.Tool == req.Tool })
		if i < 0 {
			out = append(out, Status{Tool: req.Tool})
			i = len(out) - 1
		}
		out[i].Checks = append(out[i].Checks, Check{Requirement: req})
	}
	for i := range out {
		st := &out[i]
		ts, ok := s.Tool(st.Tool)
		if !ok || ts.Err != "" {
			continue
		}
		st.Checked = true
		st.Installations = ts.Installations
		st.Terminal, st.Downloads = terminal(ts, st.Checks, lines)
		for j := range st.Checks {
			c := &st.Checks[j]
			match, err := matcher(st.Tool, c.Want, lines)
			if err != nil {
				c.Err = err.Error()
				continue
			}
			c.Installed = newest(ts.Installations, match)
			c.Met = st.Terminal != nil && match(*st.Terminal)
		}
	}
	return out
}

// terminal is the copy the terminal runs in the project folder: the one a
// version manager that switches on cd picks by the project's files, or
// else the first one on PATH. A manager that finds no copy of the version
// asked says so and leaves the one in use, as fnm and nvm's hook do.
// Corepack runs only the packageManager version: the cached copy, or, when
// none is cached, the version it downloads on first use, reported by the
// second result. With a version Hearth cannot read, no known copy runs.
func terminal(ts inventory.ToolState, checks []Check, lines map[int]versions.Line) (*inventory.Installation, bool) {
	for _, m := range ts.Managers {
		if m.Name == "corepack" {
			for _, c := range checks {
				if c.Key != "packageManager" {
					continue
				}
				match, err := matcher(ts.Tool, c.Want, lines)
				if err != nil {
					return nil, false
				}
				if in := newest(ts.Installations, func(in inventory.Installation) bool {
					return in.Source.Manager == "corepack" && !in.Source.Shim && match(in)
				}); in != nil {
					return in, false
				}
				if _, err := semver.StrictNewVersion(c.Want); err != nil {
					return nil, false
				}
				return &inventory.Installation{Tool: ts.Tool, Version: c.Want, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack"}}, true
			}
		}

		if !m.SwitchOnCD {
			continue
		}
		for _, c := range checks {
			if !reads(m, c.Requirement) {
				continue
			}
			match, err := matcher(ts.Tool, c.Want, lines)
			if err != nil {
				break
			}
			if in := newest(ts.Installations, func(in inventory.Installation) bool {
				return in.Source.Manager == m.Name && !in.Source.Shim && match(in)
			}); in != nil {
				return in, false
			}
			break
		}
	}
	var first *inventory.Installation
	for i, in := range ts.Installations {
		if in.OnPath > 0 && (first == nil || in.OnPath < first.OnPath) {
			first = &ts.Installations[i]
		}
	}
	if first == nil || !first.Source.Shim {
		return first, false
	}
	// A shim with no project file to go by runs its manager's default.
	for i, in := range ts.Installations {
		if in.Default && in.Source.Manager == first.Source.Manager && !in.Source.Shim {
			return &ts.Installations[i], false
		}
	}
	return nil, false
}

// reads reports that the version manager m picks the version by req.
func reads(m inventory.Manager, req Requirement) bool {
	file := req.File
	switch file {
	case ".mise.toml":
		file = "mise.toml"
	case "package.json":
		// Volta reads its own field of package.json, not the engines.
		if !strings.HasPrefix(req.Key, "volta.") {
			return false
		}
	}
	return slices.Contains(m.Files, file)
}

// newest is the copy of the newest version that match accepts; nil when
// there is none.
func newest(ins []inventory.Installation, match func(inventory.Installation) bool) *inventory.Installation {
	var best *inventory.Installation
	var bestV *semver.Version
	for i, in := range ins {
		if !match(in) {
			continue
		}
		v, err := semver.NewVersion(in.Version)
		if err != nil {
			continue
		}
		if best == nil || v.GreaterThan(bestV) {
			best, bestV = &ins[i], v
		}
	}
	return best
}

// matcher reads want, as a project file writes it, into a test of
// installations. Besides npm's ranges it knows nvm's aliases for Node.
func matcher(tool, want string, lines map[int]versions.Line) (func(inventory.Installation) bool, error) {
	want = strings.TrimSpace(want)
	all := func(inventory.Installation) bool { return true }
	switch strings.ToLower(want) {
	case "", "*", "latest":
		return all, nil
	}
	if tool == "node" {
		alias := strings.ToLower(want)
		switch {
		case alias == "node" || alias == "stable" || alias == "current":
			return all, nil
		case alias == "system":
			return func(in inventory.Installation) bool {
				return in.Source.Kind != inventory.KindVersionManager
			}, nil
		case alias == "lts" || alias == "lts/*":
			return func(in inventory.Installation) bool {
				return !lines[versions.Major(in.Version)].LTS.IsZero()
			}, nil
		case strings.HasPrefix(alias, "lts/"):
			codename := strings.TrimPrefix(alias, "lts/")
			return func(in inventory.Installation) bool {
				l := lines[versions.Major(in.Version)]
				return !l.LTS.IsZero() && strings.EqualFold(l.Codename, codename)
			}, nil
		}
	}
	c, err := semver.NewConstraint(want)
	if err != nil {
		return nil, fmt.Errorf("version %q: %w", want, err)
	}
	return func(in inventory.Installation) bool {
		v, err := semver.NewVersion(in.Version)
		return err == nil && c.Check(v)
	}, nil
}
