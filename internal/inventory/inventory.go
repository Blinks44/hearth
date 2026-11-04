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
}

// Package is a package by name and version.
type Package struct {
	Name, Version string
}

// ToolState is what a detector found of one tool.
type ToolState struct {
	Tool string
	// Installations come in the terminal's PATH order first, then the GUI's,
	// then the ones only a manager's directory holds.
	Installations []Installation
	// Err says why the tool could not be checked; empty when it was.
	Err string `json:",omitempty"`
}

// Snapshot is what one scan found on the computer.
type Snapshot struct {
	Taken time.Time
	Tools []ToolState
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
