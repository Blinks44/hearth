// Package issues finds what needs the user's attention in a scan: plain
// rules over the snapshot and the projects, with no I/O.
package issues

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// Kind is the rule an issue comes from.
type Kind string

const (
	// DockerUnavailable is an unsuccessful probe of the selected local engine.
	DockerUnavailable Kind = "docker-unavailable"
	// Duplicate is a tool with several copies on the terminal's PATH: the
	// first runs, the others never do.
	Duplicate Kind = "duplicate"
	// GUIDiffers is a tool apps started outside the terminal run at
	// another version than the terminal, or do not find.
	GUIDiffers Kind = "gui-differs"
	// ProjectMissing is a project needing a version no copy has.
	ProjectMissing Kind = "project-missing"
	// ProjectTerminal is a project needing a version that is installed,
	// though the terminal runs another in its folder.
	ProjectTerminal Kind = "project-terminal"
	// Unsupported is a tool with copies of versions whose support ended.
	Unsupported Kind = "unsupported"
	// PathCurrentDir is an empty or relative PATH entry: commands are then
	// looked for in whatever folder the terminal is in.
	PathCurrentDir Kind = "path-current-dir"
	// PathMissing is a PATH entry naming a directory that is not there.
	PathMissing Kind = "path-missing"
	// SkillCopies is a skill copied to several folders of the user's or of
	// one project, the same files in each: an edit to one misses the others.
	SkillCopies Kind = "skill-copies"
	// SkillDiffers is several folders of the user's or of one project
	// holding a skill by one name, with different files: agents may get
	// different versions of it.
	SkillDiffers Kind = "skill-differs"
	// MCPMissing is a command MCP servers start that is not there: the
	// agents cannot start them.
	MCPMissing Kind = "mcp-missing"
	// StubOnPath is a stand-in on the terminal's PATH that does not run
	// the tool, as Microsoft Store's python.exe: a warning when it comes
	// first, since the command then opens the Store; else it takes over
	// once the copies before it go.
	StubOnPath Kind = "stub-on-path"
)

// Severity is how much an issue matters.
type Severity int

const (
	// Info is worth knowing; nothing breaks for it.
	Info Severity = iota
	// Warning makes something run other than the user expects.
	Warning
)

// Issue is one thing that needs attention. A skill's issue lives on the
// skills page, an MCP server's on the MCP page; the page of another follows from Project, then Tool, and
// an issue with neither is about the PATH.
type Issue struct {
	Kind     Kind
	Severity Severity
	// Tool is the ID of the tool concerned, as "node"; "" for the PATH.
	Tool string
	// Project is the folder of the project concerned; "" for none.
	Project string
	// Copies are the copies concerned: for Duplicate the ones on PATH in
	// its order; for GUIDiffers the terminal's, then the apps' if they
	// find one; for a project the terminal's in its folder, if any; for
	// Unsupported the ones out of support.
	Copies []inventory.Installation
	// Check is the requirement a project issue is about.
	Check projects.Check
	// Dirs are the PATH entries a PATH issue is about, each once.
	Dirs []string
	// Skills are the copies a skill's issue is about, in the order found.
	Skills []inventory.Skill
	// MCP are the servers an MCP issue is about, in the order found.
	MCP []inventory.MCPServer
}

// Find returns the issues of the scan s and the projects ps, the PATH's
// and the tools' first, then the user's skills' and MCP servers', then the
// projects'. node is what nodejs.org says
// of Node's versions, nil when not known: without it no version counts as
// out of support, and lts/* meets nothing. now is the time support is
// judged at.
func Find(s *inventory.Snapshot, ps []projects.Project, node *versions.Node, now time.Time) []Issue {
	if s == nil {
		return nil
	}
	var lines map[int]versions.Line
	if node != nil {
		lines = node.Lines
	}
	out := pathIssues(s)
	for _, ts := range s.Tools {
		if ts.Err != "" {
			continue
		}
		if ts.Docker != nil {
			for _, c := range ts.Docker.Contexts {
				if c.Name == ts.Docker.Selected && c.Status == "unavailable" {
					out = append(out, Issue{Kind: DockerUnavailable, Severity: Warning, Tool: "docker"})
				}
			}
		}
		out = append(out, duplicate(ts)...)
		out = append(out, stubs(ts)...)
		out = append(out, guiDiffers(ts)...)
		if ts.Tool == "node" {
			out = append(out, unsupported(ts, lines, now)...)
		}
	}
	out = append(out, skillIssues(s.Skills)...)
	out = append(out, mcpMissing(s.MCP)...)
	for _, p := range ps {
		out = append(out, skillIssues(p.Skills)...)
		out = append(out, mcpMissing(p.MCP)...)
		for _, st := range projects.Resolve(p, s, lines) {
			ch, ok := st.Unmet()
			if !ok {
				continue
			}
			is := Issue{Kind: ProjectTerminal, Severity: Warning, Tool: st.Tool, Project: p.Dir, Check: ch}
			if ch.Installed == nil {
				is.Kind = ProjectMissing
			}
			if st.Terminal != nil {
				is.Copies = []inventory.Installation{*st.Terminal}
			}
			out = append(out, is)
		}
	}
	return out
}

// osDirs are PATH entries the OS itself adds that are often not there, as
// macOS's /etc/paths.d/10-cryptex and 10-pmk-global do: leaving them is
// right, so they are no issue.
var osDirs = []string{
	"/var/run/com.apple.security.cryptexd/",
	"/pkg/env/global/bin",
}

// pathIssues are the terminal's and the apps' PATH entries that mean the
// working directory, and those naming no directory.
func pathIssues(s *inventory.Snapshot) []Issue {
	var current, missing []string
	for _, e := range slices.Concat(s.TerminalPath, s.GUIPath) {
		switch {
		case osDir(e.Dir):
		case !filepath.IsAbs(e.Dir) && !slices.Contains(current, e.Dir):
			current = append(current, e.Dir)
		case e.Missing && !slices.Contains(missing, e.Dir):
			missing = append(missing, e.Dir)
		}
	}
	var out []Issue
	if len(current) > 0 {
		out = append(out, Issue{Kind: PathCurrentDir, Severity: Warning, Dirs: current})
	}
	if len(missing) > 0 {
		out = append(out, Issue{Kind: PathMissing, Severity: Info, Dirs: missing})
	}
	return out
}

func duplicate(ts inventory.ToolState) []Issue {
	var on []inventory.Installation
	for _, in := range ts.Installations {
		// A stub is no copy of the tool, stubs says of it; one the OS
		// needs stays.
		if in.OnPath > 0 && !in.Stub && !inventory.NeededBySystem(in) {
			on = append(on, in)
		}
	}
	if len(on) < 2 {
		return nil
	}
	slices.SortFunc(on, func(a, b inventory.Installation) int { return int(a.OnPath - b.OnPath) })
	return []Issue{{Kind: Duplicate, Severity: Warning, Tool: ts.Tool, Copies: on}}
}

// stubs is an issue for each stub on the terminal's PATH.
func stubs(ts inventory.ToolState) []Issue {
	var out []Issue
	for _, in := range ts.Installations {
		if in.Stub && in.OnPath > 0 {
			severity := Info
			if in.OnPath == 1 {
				severity = Warning
			}
			out = append(out, Issue{Kind: StubOnPath, Severity: severity, Tool: ts.Tool, Copies: []inventory.Installation{in}})
		}
	}
	return out
}

// guiDiffers is an issue when the terminal runs a copy and apps run
// another version or none; the same version elsewhere is no matter.
func guiDiffers(ts inventory.ToolState) []Issue {
	term := inventory.TerminalCopy(ts.Installations)
	if term < 0 {
		return nil
	}
	is := Issue{Kind: GUIDiffers, Severity: Warning, Tool: ts.Tool, Copies: []inventory.Installation{ts.Installations[term]}}
	gui := inventory.GUICopy(ts.Installations)
	switch {
	case gui < 0:
	case ts.Installations[gui].Version == ts.Installations[term].Version:
		return nil
	default:
		is.Copies = append(is.Copies, ts.Installations[gui])
	}
	return []Issue{is}
}

// unsupported is an issue when copies are of release lines whose support
// ended; a warning when the terminal runs one of them.
func unsupported(ts inventory.ToolState, lines map[int]versions.Line, now time.Time) []Issue {
	term := inventory.TerminalCopy(ts.Installations)
	is := Issue{Kind: Unsupported, Severity: Info, Tool: ts.Tool}
	for i, in := range ts.Installations {
		l, ok := lines[versions.Major(in.Version)]
		if in.Source.Shim || !ok || !l.Ended(now) {
			continue
		}
		is.Copies = append(is.Copies, in)
		if i == term {
			is.Severity = Warning
		}
	}
	if len(is.Copies) == 0 {
		return nil
	}
	return []Issue{is}
}

// skillIssues are the copies by one name among one scope's skills. One
// folder that several paths lead to is no copy, nor is a broken link.
func skillIssues(skills []inventory.Skill) []Issue {
	var names []string
	byName := map[string][]inventory.Skill{}
	for _, sk := range skills {
		if sk.Broken {
			continue
		}
		if _, ok := byName[sk.Name]; !ok {
			names = append(names, sk.Name)
		}
		byName[sk.Name] = append(byName[sk.Name], sk)
	}
	var out []Issue
	for _, name := range names {
		copies := byName[name]
		if len(copies) < 2 {
			continue
		}
		is := Issue{Kind: SkillCopies, Severity: Info, Project: copies[0].Project, Skills: copies}
		if slices.ContainsFunc(copies, func(c inventory.Skill) bool { return c.Sum == "" || c.Sum != copies[0].Sum }) {
			is.Kind, is.Severity = SkillDiffers, Warning
		}
		out = append(out, is)
	}
	return out
}

// mcpMissing are the commands of servers that are not there, an issue per
// command and project, in the order found. A server turned off is none.
func mcpMissing(servers []inventory.MCPServer) []Issue {
	var out []Issue
	for _, sv := range servers {
		if !sv.Missing || sv.Disabled {
			continue
		}
		i := slices.IndexFunc(out, func(is Issue) bool {
			return is.Project == sv.Project && is.MCP[0].Command == sv.Command
		})
		if i < 0 {
			out = append(out, Issue{Kind: MCPMissing, Severity: Warning, Project: sv.Project})
			i = len(out) - 1
		}
		out[i].MCP = append(out[i].MCP, sv)
	}
	return out
}

func osDir(dir string) bool {
	return slices.ContainsFunc(osDirs, func(p string) bool { return strings.HasPrefix(dir, p) })
}
