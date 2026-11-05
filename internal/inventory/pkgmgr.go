package inventory

// SystemPackage is a package owned by a system package manager.
type SystemPackage struct {
	Name, Version, Available, Kind, Path string
	Dependencies                         []string `json:",omitempty"`
}

// PackageManager is a discovered manager and the results of its read probes.
type PackageManager struct {
	ID, Name, Path, Version, Prefix string
	Packages                        []SystemPackage
	// UpdatesKnown distinguishes an empty list from an unavailable check.
	UpdatesKnown bool
	// Unlisted reports that packages were not read: listing them could
	// refresh the manager's sources.
	Unlisted bool     `json:",omitempty"`
	Warnings []string `json:",omitempty"`
	Taps     []string `json:",omitempty"`
	Settings []string `json:",omitempty"`
}
