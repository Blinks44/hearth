package inventory

import "time"

// Sizes are what a count, separate from the scan, found the folders Hearth
// knows take on disk: copies of tools, packages, caches, and Docker's data.
type Sizes struct {
	Taken time.Time
	// Paths are the folders and files counted, by path as the snapshot
	// names them.
	Paths map[string]Usage
	// Caches are the tools' caches found, counted in Paths.
	Caches []Cache `json:",omitempty"`
	// Installs are the projects' folders of dependencies found under the
	// home folder, as node_modules, counted in Paths.
	Installs []ProjectInstall `json:",omitempty"`
	// Docker is what the selected local engine reports it keeps;
	// DockerErr says why it could not be asked.
	Docker    []DockerUsage `json:",omitempty"`
	DockerErr string        `json:",omitempty"`
}

// Usage is the space a folder takes, each file counted once however many
// hard links lead to it.
type Usage struct {
	Bytes int64
	// Shared is the part of Bytes in files hard links outside the folder
	// also lead to, as a pnpm store's in projects' node_modules: removing
	// the folder does not free it.
	Shared int64 `json:",omitempty"`
	// Partial reports that some of it could not be read: it takes at
	// least Bytes.
	Partial bool `json:",omitempty"`
}

// Cache is a tool's cache folder or set of temporary folders: files the
// tool downloads or recreates again when they are gone.
type Cache struct {
	// ID names the cache: "npm", "pnpm", "uv", "go-build", ...
	ID   string
	Path string
	// Paths holds every path belonging to a multi-path cache (for example
	// cargo registry + git, or an agent's temporary cache subfolders). When
	// non-empty, Path is Paths[0].
	Paths []string `json:",omitempty"`
	// OptIn marks a cache that Cleanup leaves unchecked by default, such as
	// abandoned agent worktrees.
	OptIn bool `json:",omitempty"`
}

// AllPaths returns all paths of c: Paths when set, else Path when non-empty.
func (c Cache) AllPaths() []string {
	if len(c.Paths) > 0 {
		return c.Paths
	}
	if c.Path != "" {
		return []string{c.Path}
	}
	return nil
}

// InstallRole values classify project artifact directories in Cleanup.
const (
	InstallRoleDeps  = "deps"
	InstallRoleBuild = "build"
)

// ProjectInstall is a project's folder of dependencies or build artifacts,
// Project/Name, with its role, optional enclosing repository/workspace Root
// when Project is a nested package, and when the project last changed.
type ProjectInstall struct {
	Project, Name string
	Root          string `json:",omitempty"`
	Role          string `json:",omitempty"`
	Changed       time.Time
}

// DockerUsage is one line of docker system df: the images, the containers,
// the volumes or the build cache.
type DockerUsage struct {
	// Type is Docker's: "Images", "Containers", "Local Volumes" or
	// "Build Cache".
	Type          string
	Count, Active int
	// Bytes is the space they take, Reclaimable what pruning those not in
	// use would free; both as Docker rounds them.
	Bytes, Reclaimable int64
}

// Size returns the space the package takes on disk, from the manager's
// data or as counted, and whether it is known.
func (s *Sizes) Size(p SystemPackage) (Usage, bool) {
	if p.Size > 0 {
		return Usage{Bytes: p.Size}, true
	}
	if s == nil || p.Path == "" {
		return Usage{}, false
	}
	var total Usage
	for _, path := range append([]string{p.Path}, p.Artifacts...) {
		u, ok := s.Paths[path]
		if !ok {
			return Usage{}, false
		}
		total.Bytes += u.Bytes
		total.Shared += u.Shared
		total.Partial = total.Partial || u.Partial
	}
	return total, true
}
