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
}

// Tool returns the state of the tool by its ID, as "node", and whether the
// scan checked it.
func (s *Snapshot) Tool(id string) (ToolState, bool) {
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
