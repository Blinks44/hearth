package projects

import (
	"fmt"
	"slices"
	"strconv"
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
	// EngineDown reports, for Docker, that the engine of the selected
	// local context does not answer: the CLI alone runs no containers.
	EngineDown bool
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
		ts, ok := snapshotTool(s, st.Tool)
		if !ok || ts.Err != "" {
			continue
		}
		st.Checked = true
		if st.Tool == "dotnet" {
			resolveDotnet(st, ts)
			continue
		}
		st.Installations = ts.Installations
		st.Terminal, st.Downloads = terminal(ts, st.Checks, lines)
		if d := ts.Docker; d != nil {
			i := slices.IndexFunc(d.Contexts, func(c inventory.DockerContext) bool { return c.Name == d.Selected })
			st.EngineDown = i >= 0 && d.Contexts[i].Local && d.Contexts[i].Status == "unavailable"
		}
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
	// A shim with no project file to go by runs its manager's default, or
	// for rbenv and pyenv the next copy on PATH.
	if i := inventory.TerminalCopy(ts.Installations); i >= 0 {
		return &ts.Installations[i], false
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
		v, err := installedVersion(in.Version)
		if err != nil {
			continue
		}
		if best == nil || v.GreaterThan(bestV) {
			best, bestV = &ins[i], v
		}
	}
	return best
}

// installedVersion reads a copy's version. OpenJDK's updates add a fourth
// number, as 21.0.12.1, which semver does not take.
func installedVersion(version string) (*semver.Version, error) {
	if parts := strings.Split(version, "."); len(parts) > 3 {
		version = strings.Join(parts[:3], ".")
	}
	return semver.NewVersion(version)
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
	// A Java pin with a distribution, like SDKMAN's 21.0.5-tem or mise's
	// temurin-21, names a manager's install directory.
	if tool == "java" && strings.ContainsAny(want, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_") {
		return func(in inventory.Installation) bool {
			v := in.Source.Version
			return !in.Source.Shim && (v == want || strings.HasPrefix(v, want+"."))
		}, nil
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
	if tool == "terraform" || tool == "tofu" {
		want = terraformConstraint(want)
	}
	c, err := semver.NewConstraint(want)
	if err != nil {
		return nil, fmt.Errorf("version %q: %w", want, err)
	}
	return func(in inventory.Installation) bool {
		v, err := installedVersion(in.Version)
		return err == nil && c.Check(v)
	}, nil
}

// snapshotTool returns a tool's state from s, synthesizing terraform and tofu
// from the cli detector and mise installs since they share the CLI page.
func snapshotTool(s *inventory.Snapshot, tool string) (inventory.ToolState, bool) {
	if s == nil {
		return inventory.ToolState{}, false
	}
	if ts, ok := s.Tool(tool); ok {
		return ts, true
	}
	if tool != "terraform" && tool != "tofu" {
		return inventory.ToolState{}, false
	}
	cli, ok := s.Tool("cli")
	if !ok || cli.Err != "" {
		return inventory.ToolState{}, false
	}
	ts := inventory.ToolState{Tool: tool}
	if cli.Cli != nil {
		for _, in := range cli.Cli.Tools {
			if in.Tool == tool {
				ts.Installations = append(ts.Installations, in)
			}
		}
	}
	if mise, ok := s.Tool("mise"); ok && mise.Err == "" {
		for _, mt := range mise.MiseTools {
			if mt.Name != tool && (tool != "tofu" || mt.Name != "opentofu") {
				continue
			}
			for _, in := range mt.Installations {
				in.Tool = tool
				dup := false
				for i := range ts.Installations {
					ex := &ts.Installations[i]
					if ex.Source.Manager == "mise" && !ex.Source.Shim && ex.Version == in.Version {
						ex.Default = ex.Default || in.Default
						dup = true
						break
					}
				}
				if !dup {
					ts.Installations = append(ts.Installations, in)
				}
			}
		}
	}
	for _, t := range s.Tools {
		for _, m := range t.Managers {
			if m.Name != "mise" {
				continue
			}
			mm := m
			mm.Files = slices.Clone(m.Files)
			if tool == "terraform" {
				mm.Files = append(mm.Files, ".terraform-version")
			} else {
				mm.Files = append(mm.Files, ".opentofu-version", ".tofu-version")
			}
			ts.Managers = append(ts.Managers, mm)
			return ts, true
		}
	}
	return ts, true
}

// terraformConstraint translates HashiCorp/OpenTofu pessimistic constraints
// (~> 1.10 -> >= 1.10.0, < 2.0.0; ~> 1.10.2 -> >= 1.10.2, < 1.11.0) into
// semver constraints.
func terraformConstraint(want string) string {
	parts := strings.Split(want, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "~>") {
			out = append(out, p)
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p[2:]), "v"))
		core, _, _ := strings.Cut(v, "-")
		segs := strings.Split(core, ".")
		switch len(segs) {
		case 1:
			if maj, err := strconv.Atoi(segs[0]); err == nil {
				out = append(out, fmt.Sprintf(">= %d.0.0, < %d.0.0", maj, maj+1))
				continue
			}
		case 2:
			maj, err1 := strconv.Atoi(segs[0])
			min, err2 := strconv.Atoi(segs[1])
			if err1 == nil && err2 == nil {
				out = append(out, fmt.Sprintf(">= %d.%d.0, < %d.0.0", maj, min, maj+1))
				continue
			}
		case 3:
			maj, err1 := strconv.Atoi(segs[0])
			min, err2 := strconv.Atoi(segs[1])
			_, err3 := strconv.Atoi(segs[2])
			if err1 == nil && err2 == nil && err3 == nil {
				out = append(out, fmt.Sprintf(">= %s, < %d.%d.0", v, maj, min+1))
				continue
			}
		}
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}

// Pick returns the newest of releases, newest first, that want asks of
// tool; false when none does or want cannot be understood.
func Pick(tool, want string, releases []string, lines map[int]versions.Line) (string, bool) {
	match, err := matcher(tool, want, lines)
	if err != nil {
		return "", false
	}
	for _, v := range releases {
		if match(inventory.Installation{Version: v}) {
			return v, true
		}
	}
	return "", false
}
