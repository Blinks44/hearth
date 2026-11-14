// Package issues finds what needs the user's attention in a scan: plain
// rules over the snapshot and the projects, with no I/O.
package issues

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// Kind is the rule an issue comes from.
type Kind string

const (
	// DockerUnavailable is an unsuccessful probe of the selected local engine.
	DockerUnavailable Kind = "docker-unavailable"
	// DockerDenied is a selected local engine whose socket refused this user.
	DockerDenied Kind = "docker-denied"
	// DockerSignIn is DockerDenied for a user in the socket's group
	// already, whose session started before they joined it.
	DockerSignIn Kind = "docker-sign-in"
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
	// PlainSecret is a secret an MCP server's file writes out, as a token
	// in its env, headers, arguments or URL: whoever reads or copies the
	// file, or the project it is in, has the secret.
	PlainSecret Kind = "plain-secret"
	// SettingsSecret is a secret an agent's settings file writes out, as
	// an API key in the "env" of Claude Code's settings.json.
	SettingsSecret Kind = "settings-secret"
	// AgentUnasked is an agent's setting that lets it do anything without
	// asking, as Claude Code's bypassPermissions or an allowed Bash:
	// worth knowing, since nothing then stops a wrong command.
	AgentUnasked Kind = "agent-unasked"
	// StubOnPath is a stand-in on the terminal's PATH that does not run
	// the tool, as Microsoft Store's python.exe: a warning when it comes
	// first, since the command then opens the Store; else it takes over
	// once the copies before it go.
	StubOnPath Kind = "stub-on-path"
	// ExtraVersion is copies of a tool on no PATH that their manager does
	// not pick by default and no project asks for: likely left over, a
	// cleanup candidate. Info: nothing runs them.
	ExtraVersion Kind = "extra-version"
	// SlowStartup is a terminal's shell that takes SlowStartupFrom or
	// longer to start: each new terminal window waits for it.
	SlowStartup Kind = "slow-startup"
)

// SlowStartupFrom is how long a shell start takes for SlowStartup.
const SlowStartupFrom = time.Second

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
	// Startup is the shell start SlowStartup is about.
	Startup *inventory.ShellStartup
	// Agent is the agent whose settings SettingsSecret is about.
	Agent *inventory.Agent
	// Rule is the setting AgentUnasked is about.
	Rule *inventory.AgentRule
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
	out = append(out, DatabaseConflicts(s, ps)...)
	for _, ts := range s.Tools {
		if ts.Err != "" {
			continue
		}
		if ts.Docker != nil {
			for _, c := range ts.Docker.Contexts {
				if c.Name == ts.Docker.Selected && c.Status == "unavailable" {
					kind := DockerUnavailable
					switch {
					case c.Joined:
						kind = DockerSignIn
					case c.Denied:
						kind = DockerDenied
					}
					out = append(out, Issue{Kind: kind, Severity: Warning, Tool: "docker"})
				}
			}
		}
		out = append(out, stubs(ts, ps)...)
		if withNode(s, ts) {
			continue
		}
		out = append(out, duplicate(ts)...)
		out = append(out, guiDiffers(ts)...)
		if ts.Tool == "node" {
			out = append(out, unsupported(ts, lines, now)...)
		}
	}
	out = append(out, extraVersions(s, ps, lines, now)...)
	out = append(out, skillIssues(s.Skills)...)
	out = append(out, mcpMissing(s.MCP)...)
	out = append(out, plainSecrets(s.MCP)...)
	found := map[string]bool{}
	for i, a := range s.Agents {
		if len(a.PlainEnv) > 0 {
			out = append(out, Issue{Kind: SettingsSecret, Severity: Warning, Agent: &s.Agents[i]})
		}
		found[a.ID] = a.Found()
		out = append(out, unasked(a.Rules, found)...)
	}
	for _, p := range ps {
		out = append(out, skillIssues(p.Skills)...)
		out = append(out, mcpMissing(p.MCP)...)
		out = append(out, plainSecrets(p.MCP)...)
		out = append(out, unasked(p.Rules, found)...)
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

// extraVersions are the cleanup's copies on no PATH, extra ones and those
// of a manager the shell does not set up, one issue per tool and a copy
// per folder; those shadowed on PATH are Duplicate's.
func extraVersions(s *inventory.Snapshot, ps []projects.Project, lines map[int]versions.Line, now time.Time) []Issue {
	var out []Issue
	// seen are the folders listed: npm in nvm's Node folder is Node's.
	seen := map[string]bool{}
	add := func(in inventory.Installation) {
		if seen[in.Source.Root] {
			return
		}
		seen[in.Source.Root] = true
		i := slices.IndexFunc(out, func(is Issue) bool { return is.Tool == in.Tool })
		if i < 0 {
			out = append(out, Issue{Kind: ExtraVersion, Severity: Info, Tool: in.Tool})
			i = len(out) - 1
		}
		out[i].Copies = append(out[i].Copies, in)
	}
	for _, c := range cleanup.Find(s, nil, ps, lines, now) {
		switch {
		case c.Kind == cleanup.ExtraCopy && slices.Contains(c.Reasons, cleanup.NotOnPath):
			add(c.Copies[0])
		case c.Kind == cleanup.Leftover:
			for _, in := range c.Copies {
				add(in)
			}
		}
	}
	return out
}

// osDirs are PATH entries the OS itself adds that are often not there, as
// macOS's /etc/paths does /usr/local/bin before anything installs there,
// and /etc/paths.d/10-cryptex and 10-pmk-global theirs: leaving them is
// right, so they are no issue. Each ends in a slash and covers what is
// under it.
var osDirs = []string{
	"/var/run/com.apple.security.cryptexd/",
	"/pkg/env/global/bin/",
	"/usr/local/bin/",
}

// pathIssues are the terminal's and the apps' PATH entries that mean the
// working directory, and those naming no directory.
func pathIssues(s *inventory.Snapshot) []Issue {
	var current, missing []string
	for _, e := range slices.Concat(s.TerminalPath, s.GUIPath) {
		switch {
		case OSDir(e.Dir):
		case !filepath.IsAbs(e.Dir) && !slices.Contains(current, e.Dir):
			current = append(current, e.Dir)
		case e.Missing && !slices.Contains(missing, e.Dir):
			missing = append(missing, e.Dir)
		}
	}
	var out []Issue
	if st := s.Startup; st != nil && st.Took >= SlowStartupFrom {
		out = append(out, Issue{Kind: SlowStartup, Severity: Warning, Startup: st})
	}
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
	// A manager's shim runs that manager's copies, as its own install
	// directory beside it on PATH does under mise activate: one copy.
	on = slices.DeleteFunc(on, func(in inventory.Installation) bool {
		return in.Source.Shim && slices.ContainsFunc(on, func(o inventory.Installation) bool {
			return !o.Source.Shim && o.Source.Manager != "" && o.Source.Manager == in.Source.Manager
		})
	})
	if len(on) < 2 {
		return nil
	}
	// Outside a project a shim runs the version its manager picks by
	// default.
	for i, in := range on {
		if !in.Source.Shim {
			continue
		}
		if d := slices.IndexFunc(ts.Installations, func(o inventory.Installation) bool {
			return o.Default && !o.Source.Shim && o.Source.Manager == in.Source.Manager
		}); d >= 0 {
			on[i].Version = ts.Installations[d].Version
		}
	}
	slices.SortFunc(on, func(a, b inventory.Installation) int { return int(a.OnPath - b.OnPath) })
	return []Issue{{Kind: Duplicate, Severity: Warning, Tool: ts.Tool, Copies: on}}
}

// withNode reports that the terminal runs ts from the folder of the node
// it runs, as npm and corepack come with Node: Node's issues are theirs.
func withNode(s *inventory.Snapshot, ts inventory.ToolState) bool {
	node, _ := s.Tool("node")
	n, t := inventory.TerminalCopy(node.Installations), inventory.TerminalCopy(ts.Installations)
	return ts.Tool != "node" && n >= 0 && t >= 0 &&
		filepath.Dir(node.Installations[n].Path) == filepath.Dir(ts.Installations[t].Path)
}

// stubs is an issue for each stub on the terminal's PATH: a warning when
// it comes first and hides a copy behind it or a project needs the tool,
// as the command then does not run what was meant; else worth knowing,
// as the stub macOS keeps for a Java nobody needs.
func stubs(ts inventory.ToolState, ps []projects.Project) []Issue {
	needed := slices.ContainsFunc(ts.Installations, func(in inventory.Installation) bool { return !in.Stub && in.OnPath > 0 }) ||
		slices.ContainsFunc(ps, func(p projects.Project) bool {
			return slices.ContainsFunc(p.Requirements, func(r projects.Requirement) bool { return r.Tool == ts.Tool })
		})
	var out []Issue
	for _, in := range ts.Installations {
		if in.Stub && in.OnPath > 0 {
			severity := Info
			if in.OnPath == 1 && needed {
				severity = Warning
			}
			out = append(out, Issue{Kind: StubOnPath, Severity: severity, Tool: ts.Tool, Copies: []inventory.Installation{in}})
		}
	}
	return out
}

// guiDiffers is an issue when the terminal runs a copy and apps run
// another version, a warning, or none, which is only worth knowing: VS Code
// and JetBrains IDEs take the terminal's PATH themselves. The same
// version elsewhere is no matter, and the system's own copy in apps, as
// macOS's Ruby, only worth knowing: it is there on every computer.
func guiDiffers(ts inventory.ToolState) []Issue {
	term := inventory.TerminalCopy(ts.Installations)
	if term < 0 {
		return nil
	}
	is := Issue{Kind: GUIDiffers, Severity: Warning, Tool: ts.Tool, Copies: []inventory.Installation{ts.Installations[term]}}
	gui := inventory.GUICopy(ts.Installations)
	switch {
	case gui < 0:
		is.Severity = Info
	case ts.Installations[gui].Version == ts.Installations[term].Version:
		return nil
	default:
		is.Copies = append(is.Copies, ts.Installations[gui])
		if ts.Installations[gui].Source.Kind == inventory.KindSystem {
			is.Severity = Info
		}
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
// folder that several paths lead to is no copy, nor is a broken link, nor
// a skill Claude syncs from claude.ai: it writes that one over.
func skillIssues(skills []inventory.Skill) []Issue {
	var names []string
	byName := map[string][]inventory.Skill{}
	for _, sk := range skills {
		if sk.Broken || sk.Synced {
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

// OSDir reports that the OS itself puts dir on PATH, there or not, or an
// installer does before anything is in it: Node's Windows installer adds
// npm's global prefix, %APPDATA%\npm, which the first npm install -g makes.
func OSDir(dir string) bool {
	return slices.ContainsFunc(osDirs, func(p string) bool { return strings.HasPrefix(dir+"/", p) }) ||
		strings.HasSuffix(strings.ToLower(strings.TrimRight(dir, `\`)), `\appdata\roaming\npm`) ||
		strings.HasSuffix(strings.ReplaceAll(dir, `\`, "/"), "/.dotnet/tools")
}

// plainSecrets are the servers whose files write out a secret, an issue
// each, in the order found; one turned off too, since the file still has
// the secret.
func plainSecrets(servers []inventory.MCPServer) []Issue {
	var out []Issue
	for _, sv := range servers {
		if sv.PlainSecret() {
			out = append(out, Issue{Kind: PlainSecret, Severity: Warning, Project: sv.Project, MCP: []inventory.MCPServer{sv}})
		}
	}
	return out
}

// unasked are the risky rules of the agents found: one issue each.
func unasked(rules []inventory.AgentRule, found map[string]bool) []Issue {
	var out []Issue
	for i, r := range rules {
		if r.Risky && found[r.Agent] {
			out = append(out, Issue{Kind: AgentUnasked, Severity: Info, Project: r.Project, Rule: &rules[i]})
		}
	}
	return out
}
