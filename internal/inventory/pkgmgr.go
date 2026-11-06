package inventory

import "time"

// SystemPackage is a package owned by a system package manager.
type SystemPackage struct {
	Name, Version, Available, Kind, Path string
	Scope, Origin, Reference             string   `json:",omitempty"`
	Architecture                         string   `json:",omitempty"`
	Dependencies                         []string `json:",omitempty"`
	// Dependency is a package another pulled in, not one asked for; known
	// only where the manager's ReasonsKnown says so.
	Dependency bool `json:",omitempty"`
}

// PackageManager is a discovered manager and the results of its read probes.
type PackageManager struct {
	ID, Name, Path, Version, Prefix string
	Packages                        []SystemPackage
	// UpdatesKnown distinguishes an empty list from an unavailable check.
	UpdatesKnown   bool
	UpdatesFetched time.Time `json:",omitzero"`
	UpdateError    string    `json:",omitempty"`
	// Unlisted reports an incomplete package list: a full query could
	// refresh the manager's sources.
	Unlisted bool `json:",omitempty"`
	// ReasonsKnown says the packages tell whether they were asked for or
	// came as another's dependency.
	ReasonsKnown bool     `json:",omitempty"`
	Warnings     []string `json:",omitempty"`
	Taps         []string `json:",omitempty"`
	Settings     []string `json:",omitempty"`
}
