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
	// Manager names the manager: "brew", "fnm", "nvm-windows", ...;
	// ScriptInstall for a tool's official install script, as Bun's; empty
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

// ScriptInstall is the Manager of a copy the tool's official install
// script put in its own directory, as ~/.bun.
const ScriptInstall = "script"

// PathRank is a position in a PATH, counting from 1; 0 is not on it.
type PathRank int

// PHPConfig describes a CLI copy under the terminal environment.
type PHPConfig struct {
	Files      []string
	Extensions []string
	Err        string `json:",omitempty"`
}

// RustToolchain is a toolchain rustup keeps.
type RustToolchain struct {
	// Name is rustup's, as stable-aarch64-apple-darwin.
	Name string
	// Date is the day its compiler was built, which tells nightlies apart.
	Date string `json:",omitempty"`
	// Targets are the ones whose standard library is installed, the host's
	// among them.
	Targets []string `json:",omitempty"`
}

// JDK describes a Java installation, from its release file.
type JDK struct{ Home, Vendor string }

// DotnetEntry is an SDK or a shared framework next to a dotnet host.
type DotnetEntry struct {
	Name, Version, Path string
}

// Dotnet describes the SDKs and shared frameworks visible to one host.
// Err records an unsuccessful list probe without hiding the host.
type Dotnet struct {
	SDKs, Runtimes []DotnetEntry
	Err            string
}

// Installation is one copy of a tool.
type Installation struct {
	Dotnet  *Dotnet        `json:",omitempty"`
	Java    *JDK           `json:",omitempty"`
	PHP     *PHPConfig     `json:",omitempty"`
	Rust    *RustToolchain `json:",omitempty"`
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
	// Stub reports a stand-in that does not run the tool: Microsoft
	// Store's python.exe, which opens the Store, or macOS's
	// /usr/bin/python3 without the Command Line Tools, which offers to
	// install them. Hearth never runs one.
	Stub bool `json:",omitempty"`
	// PathFile is the shell startup file that puts the copy's directory on
	// the terminal's PATH, as ~/.zshrc for Bun's install script; empty when
	// not known.
	PathFile string `json:",omitempty"`
}

// Package is a package by name and version.
type Package struct {
	Name, Version string
	// Commands are the ones it puts on PATH, like tsc for typescript.
	Commands []string `json:",omitempty"`
	// Module is the Go module a command go install built is from, as
	// golang.org/x/tools/gopls.
	Module string `json:",omitempty"`
}

// Manager is a tool's version manager, as it is set up.
type Manager struct {
	Name string
	// ConfigPath is an existing global configuration file, when found.
	ConfigPath string `json:",omitempty"`
	// SwitchOnCD reports that the manager switches the version as the
	// terminal enters a folder, by the files named Files there.
	SwitchOnCD bool
	Files      []string
	// Version is the manager's own, when known.
	Version string `json:",omitempty"`
	// Activated reports that the shell's startup activates the manager,
	// as mise activate, which only the terminal sees; Shims and ShimsInGUI
	// that its shims are on the terminal's PATH and on the apps'.
	Activated  bool `json:",omitempty"`
	Shims      bool `json:",omitempty"`
	ShimsInGUI bool `json:",omitempty"`
	// Admin reports that changing the manager's versions needs the
	// administrator, as n's in a /usr/local the user cannot write.
	Admin bool `json:",omitempty"`
	// Installable is the versions the manager's build definitions name, as
	// rbenv's ruby-build ones: it installs no newer release; nil when the
	// manager has none or Hearth did not find them.
	Installable []string `json:",omitempty"`
	// Settings are the manager's settings that change which version runs,
	// by name as it names them, as uv's python-preference.
	Settings map[string]string `json:",omitempty"`
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
	// Vars are the tool's settings as it reads them, as Go's GOPATH and
	// GOTOOLCHAIN.
	Vars map[string]string `json:",omitempty"`
	// Docker is the CLI's contexts and the engines found, for "docker" only.
	Docker *DockerState `json:",omitempty"`
	// Databases are the local servers, clients and listeners, for
	// "databases" only.
	Databases *DatabaseState `json:",omitempty"`
	// MiseTools are mise's tools without a page of their own, for "mise"
	// only.
	MiseTools []MiseTool `json:",omitempty"`
	// Git is how Git is set up, for "git" only.
	Git *GitState `json:",omitempty"`
	// Cpp is how C and C++ are set up, for "cpp" only.
	Cpp *CppState `json:",omitempty"`
	// Models are the local AI models' apps, for "models" only.
	Models *ModelsState `json:",omitempty"`
	// Cli is the command-line tools without a page, for "cli" only.
	Cli *CliState `json:",omitempty"`
	// Mobile is how iOS and Android development is set up, for "mobile"
	// only.
	Mobile *MobileState `json:",omitempty"`
	// Editors are the code editors and IDEs, for "editors" only.
	Editors *EditorsState `json:",omitempty"`
	// PackageManagers are the system package managers found, for "packages"
	// only.
	PackageManagers []PackageManager `json:",omitempty"`
	// Err says why the tool could not be checked; empty when it was.
	Err string `json:",omitempty"`
}

// PathSource is a possible declaration of a PATH directory.
type PathSource struct {
	File string
	Line int `json:",omitempty"`
}

// EnvironmentVariable is an allowlisted tool setting in both environments.
type EnvironmentVariable struct {
	Name, Terminal, GUI string
}

// PathEntry retains possible declarations without claiming shell evaluation.
type PathEntry struct {
	Sources []PathSource `json:",omitempty"`
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
	// AppsAgent is the file of Hearth's launch agent, which gives apps the
	// terminal's PATH as the user signs in, when it is there (macOS).
	AppsAgent string `json:",omitempty"`
	// Variables are the tool settings both environments have, by name.
	Variables []EnvironmentVariable `json:",omitempty"`
	// Agents are the AI agents Hearth knows, found or not, in its order.
	Agents []Agent `json:",omitempty"`
	// Skills are the user's skills, those of projects aside.
	Skills []Skill `json:",omitempty"`
	// MCP are the MCP servers in the user's files of the agents, Claude
	// Code's servers of single projects in ~/.claude.json among them;
	// MCPErrs say which of those files could not be read.
	MCP     []MCPServer `json:",omitempty"`
	MCPErrs []string    `json:",omitempty"`
	// Startup is how long a new terminal's shell takes to start; nil when
	// it could not be measured.
	Startup *ShellStartup `json:",omitempty"`
	// Sessions are the AI agents' sessions in their local history.
	Sessions []AgentSession `json:",omitempty"`
}

// AgentSession is a session of an AI agent from its local history: the
// folder it ran in, its files and the tokens it used.
type AgentSession struct {
	// Agent is the agent's ID, as "claude" or "codex".
	Agent string
	ID    string
	// Dir is the folder the session ran in; "" when the history does not
	// say.
	Dir string `json:",omitempty"`
	// Files are its transcript and the folder of what it kept beside it,
	// which go together.
	Files []string
	// Size is what Files take; Modified is when it last wrote to them.
	Size     int64
	Modified time.Time
	// Usage is the tokens it used, a day and model each.
	Usage []TokenUsage `json:",omitempty"`
}

// TokenUsage is the tokens of a model on a day: those read anew, written
// to the prompt cache for 5 minutes and for an hour, read from it, and
// written out, thinking with them.
type TokenUsage struct {
	// Day is the local date, as "2026-11-13".
	Day   string
	Model string
	// Input leaves out what the cache gave.
	Input, CacheWrite, CacheWrite1h, CacheRead, Output int64
}

// Tokens is all of u's tokens.
func (u TokenUsage) Tokens() int64 {
	return u.Input + u.CacheWrite + u.CacheWrite1h + u.CacheRead + u.Output
}

// ShellStartup is how long the shell a new terminal opens takes to start,
// and which lines of its startup files take the time.
type ShellStartup struct {
	// Shell is the shell's name, as zsh, bash, fish or powershell.
	Shell string
	// Took is the whole start, the startup files with it.
	Took time.Duration
	// Bare is the start without the profile, on Windows; zero elsewhere.
	Bare time.Duration `json:",omitempty"`
	// Files are the startup files in the order they ran, with their time
	// when the start was traced; on Windows the profiles there are, with
	// no time.
	Files []StartupFile `json:",omitempty"`
	// Lines are the slowest lines of the startup files, slowest first, when
	// the start was slow enough to trace; on Windows the lines of the
	// profiles known to be slow, with no time.
	Lines []StartupLine `json:",omitempty"`
}

// StartupFile is a startup file of the shell and the time its lines took.
type StartupFile struct {
	File string
	Took time.Duration `json:",omitempty"`
}

// StartupLine is a line of a startup file and the time it took, all it
// ran with it: the files it sourced, the code it evaluated, the functions
// it called.
type StartupLine struct {
	File string
	Line int
	// Text is the line as the file has it.
	Text string
	Took time.Duration `json:",omitempty"`
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
	// Synced reports a skill Claude syncs from claude.ai into
	// skills/synced: Claude writes it over, so it is not the user's to
	// change.
	Synced bool `json:",omitempty"`
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
	// "cursor", "opencode", "devin", "grok", "kimi", "aider".
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

// NeededBySystem reports a copy the OS itself runs, as the system's
// Python or macOS's Git and cc from the Command Line Tools: it is no
// duplicate to remove, whatever comes before it.
func NeededBySystem(in Installation) bool {
	return (in.Tool == "python" || in.Tool == "ruby" || in.Tool == "git" || in.Tool == "cpp") && in.Source.Kind == KindSystem
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
// by default; a stub first runs no copy.
func runs(ins []Installation, rank func(Installation) PathRank) int {
	for i, in := range ins {
		if rank(in) != 1 || in.Stub {
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
		if FallsToSystem(in.Source.Manager) {
			return next(ins, rank, in.Source.Manager)
		}
	}
	return -1
}

// FallsToSystem reports that the shims of manager, with no version of its
// own picked, run the next copy on PATH: rbenv's and pyenv's "system".
func FallsToSystem(manager string) bool {
	return manager == "rbenv" || manager == "pyenv"
}

// next is the copy after manager's on a PATH, -1 for none.
func next(ins []Installation, rank func(Installation) PathRank, manager string) int {
	best := -1
	for i, in := range ins {
		if rank(in) > 1 && !in.Stub && in.Source.Manager != manager && (best < 0 || rank(in) < rank(ins[best])) {
			best = i
		}
	}
	return best
}

// MiseTool is a tool mise installed that has no page of its own, as zig,
// by its directory under installs/.
type MiseTool struct {
	// Name is the directory's, as "zig" or "npm-prettier" for a backend's.
	Name string
	// Installations are its versions, newest first, each as a copy of
	// mise's: Root is the version's directory.
	Installations []Installation
}
