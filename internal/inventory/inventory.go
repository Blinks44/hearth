// Package inventory is Hearth's model of what is installed: plain data that
// detectors fill and the UI shows. It does no I/O.
package inventory

import "time"

// Kind tells what put an installation on the machine.
type Kind int

const (
	// KindUnknown is a path no rule recognizes.
	KindUnknown Kind = iota
	// KindVersionManager is a tool version manager: fnm, nvm, mise, ...
	KindVersionManager
	// KindPackageManager is a package manager: Homebrew, Scoop, WinGet, ...
	KindPackageManager
	// KindInstaller is an official installer or a copy put outside any
	// manager, like /usr/local/bin or Program Files.
	KindInstaller
	// KindSystem is part of the OS or of its packages: /usr/bin, C:\Windows.
	KindSystem
)

// Source is where an installation came from.
type Source struct {
	Kind Kind
	// Manager names the manager: "brew", "fnm", "nvm-windows", ...; empty
	// for an installer, and for the system until its package is known.
	Manager string
	// Root is the directory the manager keeps this installation in, like
	// ~/.nvm/versions/node/v22.11.0; for a shim, the manager's directory.
	Root string
	// Version is the version the path names, like "22.11.0"; empty when it
	// names none.
	Version string
	// Shim reports a manager's shim: a stand-in that picks the version when
	// run, so the path alone does not tell which one.
	Shim bool
}

// PathRank is a position in a PATH, counting from 1; 0 is not on it.
type PathRank int

// Installation is one copy of a tool.
type Installation struct {
	Tool    string
	Version string
	// Path is where the copy was found; RealPath is Path with symlinks
	// resolved.
	Path, RealPath string
	Source         Source
	// OnPath is the copy's rank in the terminal's PATH, InGUI in the PATH
	// apps started from the Dock or a desktop menu get.
	OnPath, InGUI PathRank
	// Default reports that the copy's manager picks it by default.
	Default bool
	// Packages are the ones installed globally with this copy, like npm
	// next to Node.
	Packages []Package
	// Corepack reports that Corepack's pnpm and yarn sit next to the copy,
	// as corepack enable puts them.
	Corepack bool `json:",omitempty"`
}

// Package is a package by name and version.
type Package struct {
	Name, Version string
	// Commands are the ones it puts on PATH, like tsc for typescript.
	Commands []string `json:",omitempty"`
}

// Manager is a tool's version manager, as it is set up.
type Manager struct {
	Name string
	// SwitchOnCD reports that the manager switches the version as the
	// terminal enters a folder, by the files named Files there.
	SwitchOnCD bool
	Files      []string
}

// ToolState is what a detector found of one tool.
type ToolState struct {
	Tool string
	// Installations come in the terminal's PATH order first, then the GUI's,
	// then the ones only a manager's directory holds.
	Installations []Installation
	// Managers are the version managers that keep installations, in their
	// order.
	Managers []Manager `json:",omitempty"`
	// Err says why the tool could not be checked; empty when it was.
	Err string `json:",omitempty"`
}

// PathEntry is a directory as a PATH lists it.
type PathEntry struct {
	// Dir is as written; an empty or relative one means the working
	// directory.
	Dir string
	// Missing reports that no directory is at Dir.
	Missing bool `json:",omitempty"`
}

// Snapshot is what one scan found on the computer.
type Snapshot struct {
	Taken time.Time
	Tools []ToolState
	// TerminalPath and GUIPath are the entries of the terminal's PATH and
	// of the one apps started from the Dock or a desktop menu get, in order.
	TerminalPath, GUIPath []PathEntry `json:",omitempty"`
	// Agents are the AI agents Hearth knows, found or not, in its order.
	Agents []Agent `json:",omitempty"`
	// Skills are the user's skills, those of projects aside.
	Skills []Skill `json:",omitempty"`
	// MCP are the MCP servers in the user's files of the agents, Claude
	// Code's servers of single projects in ~/.claude.json among them;
	// MCPErrs say which of those files could not be read.
	MCP     []MCPServer `json:",omitempty"`
	MCPErrs []string    `json:",omitempty"`
}

// MCPServer is an MCP server as one agent's file declares it. Secrets are
// never kept: of env and headers only the names, and the values in the
// arguments and the URL that look like secrets are Hidden.
type MCPServer struct {
	Name string
	// Agent is the ID of the agent whose file it is in, as "claude".
	Agent string
	File  string
	// Project is the folder of the project it is for: one with the file,
	// or one Claude Code keeps servers of in ~/.claude.json; "" for all.
	Project string `json:",omitempty"`
	// Transport is "stdio" for a command the agent starts, "http" or "sse"
	// for a server at URL.
	Transport string
	Command   string   `json:",omitempty"`
	Args      []string `json:",omitempty"`
	URL       string   `json:",omitempty"`
	// Env and Headers are the names of the variables and HTTP headers it
	// gets, in order.
	Env     []string `json:",omitempty"`
	Headers []string `json:",omitempty"`
	// Disabled reports it is declared but turned off.
	Disabled bool `json:",omitempty"`
	// CommandPath is where Command is, Missing that it is nowhere; neither
	// is set when Command was not checked.
	CommandPath string `json:",omitempty"`
	Missing     bool   `json:",omitempty"`
}

// Entry is a file or folder in another folder, as a skill's.
type Entry struct {
	Name string
	Dir  bool
	// Size is a file's, in bytes; Files counts the files a folder has, in
	// it and deeper.
	Size     int64
	Files    int
	Modified time.Time
}

// Hidden stands for a value kept out of the snapshot: a secret, or what
// may be one.
const Hidden = "•••"

// Skill is a folder with a SKILL.md, as the agents find it in their
// skills directories: one folder, under one path or several.
type Skill struct {
	// Name and Description are SKILL.md's; Name is the folder's when it
	// names none.
	Name, Description string
	// Dir is the folder, symlinks followed; for a broken link, where it
	// leads.
	Dir string
	// Project is the folder of the project whose skills directories have
	// it; "" for the user's.
	Project string `json:",omitempty"`
	// Paths are where the agents find it, in the order of their skills
	// directories.
	Paths []SkillPath
	// Sum is a hash of its files' names and contents: copies with the same
	// Sum are the same.
	Sum string `json:",omitempty"`
	// Broken reports a symlink that leads nowhere.
	Broken bool `json:",omitempty"`
	// Err says why SKILL.md could not be read; empty when it was.
	Err string `json:",omitempty"`
}

// SkillPath is a path an agent finds a skill by.
type SkillPath struct {
	Path string
	// Link reports that Path is a symlink, or is in a directory that is.
	Link bool `json:",omitempty"`
	// Agents are the IDs of the agents reading the skills directory Path
	// is in, as "claude", in Hearth's order.
	Agents []string
}

// Agent is an AI coding agent as a scan found it.
type Agent struct {
	// ID names the agent: "claude", "codex", "copilot", "gemini",
	// "cursor", "opencode", "devin".
	ID string
	// Installations are the copies of its command, found as a tool's are.
	Installations []Installation `json:",omitempty"`
	// App is where its desktop app is, as Cursor's editor, with its
	// version; "" for none.
	App, AppVersion string `json:",omitempty"`
	// Dir is the directory of the user's settings for it, there or not;
	// Files are the user's files of it that are there.
	Dir   string      `json:",omitempty"`
	Files []AgentFile `json:",omitempty"`
}

// Found reports that the agent is on the computer: its command or its
// app. Files left by one removed do not count.
func (a Agent) Found() bool { return len(a.Installations) > 0 || a.App != "" }

// FileKind is what an agent's file holds.
type FileKind int

const (
	FileSettings     FileKind = iota // settings, as settings.json
	FileInstructions                 // instructions, as CLAUDE.md
	FileMCP                          // MCP servers, as mcp.json
	FileSkills                       // a directory of skills
)

// AgentFile is a file or directory of an agent's.
type AgentFile struct {
	Path string
	Kind FileKind
}

// Tool returns the state of the tool by its ID, as "node", and whether the
// scan checked it; a nil snapshot, before the first scan, checked none.
func (s *Snapshot) Tool(id string) (ToolState, bool) {
	if s == nil {
		return ToolState{}, false
	}
	for _, t := range s.Tools {
		if t.Tool == id {
			return t, true
		}
	}
	return ToolState{}, false
}

// TerminalCopy returns the index of the copy in ins the terminal runs, or
// -1 for none.
func TerminalCopy(ins []Installation) int {
	return runs(ins, func(in Installation) PathRank { return in.OnPath })
}

// GUICopy returns the index of the copy in ins apps started outside the
// terminal run, or -1 for none.
func GUICopy(ins []Installation) int {
	return runs(ins, func(in Installation) PathRank { return in.InGUI })
}

// runs returns the copy a PATH runs, by each one's rank in it, or -1 for
// none: the first, or when that is a shim, the version its manager picks
// by default.
func runs(ins []Installation, rank func(Installation) PathRank) int {
	for i, in := range ins {
		if rank(in) != 1 {
			continue
		}
		if !in.Source.Shim {
			return i
		}
		for j, d := range ins {
			if d.Default && !d.Source.Shim && d.Source.Manager == in.Source.Manager {
				return j
			}
		}
	}
	return -1
}
