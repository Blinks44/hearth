// Package history tells what changed between two snapshots, and keeps the
// changes of the last checks: plain rules, with no I/O.
package history

import (
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Kind is what happened to a thing between two checks.
type Kind string

const (
	Added   Kind = "added"
	Updated Kind = "updated"
	Removed Kind = "removed"
)

// Area is what kind of thing changed.
type Area string

const (
	// Copy is a copy of a tool: Name is the tool's ID, Path the copy's,
	// Root its folder.
	Copy Area = "copy"
	// Terminal is the version the terminal runs of the tool Name.
	Terminal Area = "terminal"
	// Packages are a package manager's packages, Name its ID and Names
	// theirs.
	Packages Area = "packages"
	// PathEntry is an entry of the terminal's PATH, Path its folder.
	PathEntry Area = "path"
	// Agent is an AI agent, Name its ID.
	Agent Area = "agent"
	// Skill is a skill, Name its name and Path its folder.
	Skill Area = "skill"
	// MCP is an MCP server, Name its name, Agent the agent's ID and Path
	// the file it is in.
	MCP Area = "mcp"
)

// Change is one thing that changed.
type Change struct {
	Kind Kind
	Area Area
	Name string
	// From and To are the versions before and after, where they tell it.
	From, To string `json:",omitempty"`
	// Manager is where a copy came from, as "fnm", or "" for an installer;
	// a package manager's name, as "Homebrew".
	Manager string `json:",omitempty"`
	Agent   string `json:",omitempty"`
	Path    string `json:",omitempty"`
	Root    string `json:",omitempty"`
	// Names are the packages of a Packages change.
	Names []string `json:",omitempty"`
	// Bytes is the space a removed copy took, when it was counted.
	Bytes int64 `json:",omitempty"`
}

// Entry is the changes one check found since the one before.
type Entry struct {
	Taken   time.Time
	Changes []Change
}

// Limit is how many checks with changes the history keeps.
const Limit = 30

// Add puts e first in entries, the newest first, keeping at most Limit;
// a check without changes adds nothing.
func Add(entries []Entry, e Entry) []Entry {
	if len(e.Changes) == 0 {
		return entries
	}
	out := append([]Entry{e}, entries...)
	return out[:min(len(out), Limit)]
}

// Diff returns what changed from prev to cur; nil when either is nil, as
// before the first check. sizes are the ones counted of prev, for the
// space removed copies took; nil when not known.
func Diff(prev, cur *inventory.Snapshot, sizes *inventory.Sizes) []Change {
	if prev == nil || cur == nil {
		return nil
	}
	var out []Change
	for _, nt := range cur.Tools {
		ot, ok := prev.Tool(nt.Tool)
		if !ok || ot.Err != "" || nt.Err != "" {
			continue
		}
		out = append(out, copies(nt.Tool, ot.Installations, nt.Installations, sizes)...)
		ov, nv := terminalVersion(ot), terminalVersion(nt)
		if ov != "" && nv != "" && ov != nv {
			out = append(out, Change{Kind: Updated, Area: Terminal, Name: nt.Tool, From: ov, To: nv})
		}
		for _, nm := range nt.PackageManagers {
			i := slices.IndexFunc(ot.PackageManagers, func(m inventory.PackageManager) bool { return m.ID == nm.ID && m.Path == nm.Path })
			if i >= 0 {
				out = append(out, packages(ot.PackageManagers[i], nm)...)
			}
		}
	}
	out = append(out, pathEntries(prev.TerminalPath, cur.TerminalPath)...)
	out = append(out, agents(prev.Agents, cur.Agents)...)
	out = append(out, skills(prev.Skills, cur.Skills)...)
	out = append(out, servers(prev.MCP, cur.MCP)...)
	return out
}

func terminalVersion(ts inventory.ToolState) string {
	if i := inventory.TerminalCopy(ts.Installations); i >= 0 {
		return ts.Installations[i].Version
	}
	return ""
}

// copies compares a tool's copies by path: a shim is no copy of its own.
func copies(tool string, prev, cur []inventory.Installation, sizes *inventory.Sizes) []Change {
	var out []Change
	find := func(ins []inventory.Installation, path string) int {
		return slices.IndexFunc(ins, func(in inventory.Installation) bool { return !in.Source.Shim && in.Path == path })
	}
	for _, in := range cur {
		if in.Source.Shim || in.Stub {
			continue
		}
		c := Change{Area: Copy, Name: tool, Manager: in.Source.Manager, Path: in.Path, Root: in.Source.Root, To: in.Version}
		switch i := find(prev, in.Path); {
		case i < 0:
			c.Kind = Added
		case prev[i].Version != in.Version && in.Version != "":
			c.Kind, c.From = Updated, prev[i].Version
		default:
			continue
		}
		out = append(out, c)
	}
	for _, in := range prev {
		if in.Source.Shim || in.Stub || find(cur, in.Path) >= 0 {
			continue
		}
		c := Change{Kind: Removed, Area: Copy, Name: tool, Manager: in.Source.Manager, Path: in.Path, Root: in.Source.Root, From: in.Version}
		if sizes != nil {
			c.Bytes = sizes.Paths[in.Source.Root].Bytes
		}
		out = append(out, c)
	}
	return out
}

// packages compares a manager's packages by what tells them apart, as
// Flatpak's branches of one name: one change for those added, one for
// those updated, one for those removed.
func packages(prev, cur inventory.PackageManager) []Change {
	key := func(p inventory.SystemPackage) string {
		return strings.Join([]string{p.Name, p.Kind, p.Reference, p.Scope, p.Architecture}, "\x00")
	}
	before := map[string]string{}
	for _, p := range prev.Packages {
		before[key(p)] = p.Version
	}
	var added, updated, removed []string
	for _, p := range cur.Packages {
		v, ok := before[key(p)]
		switch {
		case !ok:
			added = append(added, p.Name)
		case v != p.Version:
			updated = append(updated, p.Name)
		}
		delete(before, key(p))
	}
	for k := range before {
		name, _, _ := strings.Cut(k, "\x00")
		removed = append(removed, name)
	}
	slices.Sort(removed)
	var out []Change
	for _, g := range []struct {
		kind  Kind
		names []string
	}{{Added, added}, {Updated, updated}, {Removed, removed}} {
		if len(g.names) > 0 {
			out = append(out, Change{Kind: g.kind, Area: Packages, Name: cur.ID, Manager: cur.Name, Names: g.names})
		}
	}
	return out
}

// pathEntries are the folders that came onto the terminal's PATH and those
// that left it.
func pathEntries(prev, cur []inventory.PathEntry) []Change {
	var out []Change
	has := func(es []inventory.PathEntry, dir string) bool {
		return slices.ContainsFunc(es, func(e inventory.PathEntry) bool { return e.Dir == dir })
	}
	for _, e := range cur {
		if !has(prev, e.Dir) {
			c := Change{Kind: Added, Area: PathEntry, Path: e.Dir}
			if len(e.Sources) > 0 {
				c.Name = e.Sources[0].File
			}
			out = append(out, c)
		}
	}
	for _, e := range prev {
		if !has(cur, e.Dir) {
			out = append(out, Change{Kind: Removed, Area: PathEntry, Path: e.Dir})
		}
	}
	return out
}

// agents compares the agents found and the version of their first copy.
func agents(prev, cur []inventory.Agent) []Change {
	version := func(a inventory.Agent) string {
		if len(a.Installations) > 0 {
			return a.Installations[0].Version
		}
		return a.AppVersion
	}
	var out []Change
	for _, na := range cur {
		i := slices.IndexFunc(prev, func(a inventory.Agent) bool { return a.ID == na.ID })
		wasFound := i >= 0 && prev[i].Found()
		switch {
		case na.Found() && !wasFound:
			out = append(out, Change{Kind: Added, Area: Agent, Name: na.ID, To: version(na)})
		case !na.Found() && wasFound:
			out = append(out, Change{Kind: Removed, Area: Agent, Name: na.ID, From: version(prev[i])})
		case na.Found() && version(na) != version(prev[i]) && version(na) != "":
			out = append(out, Change{Kind: Updated, Area: Agent, Name: na.ID, From: version(prev[i]), To: version(na)})
		}
	}
	return out
}

// skills compares the user's skills by folder; cur files make one
// updated.
func skills(prev, cur []inventory.Skill) []Change {
	var out []Change
	for _, ns := range cur {
		i := slices.IndexFunc(prev, func(s inventory.Skill) bool { return s.Dir == ns.Dir })
		switch {
		case i < 0:
			out = append(out, Change{Kind: Added, Area: Skill, Name: ns.Name, Path: ns.Dir})
		case prev[i].Sum != ns.Sum && ns.Sum != "" && prev[i].Sum != "":
			out = append(out, Change{Kind: Updated, Area: Skill, Name: ns.Name, Path: ns.Dir})
		}
	}
	for _, ps := range prev {
		if !slices.ContainsFunc(cur, func(s inventory.Skill) bool { return s.Dir == ps.Dir }) {
			out = append(out, Change{Kind: Removed, Area: Skill, Name: ps.Name, Path: ps.Dir})
		}
	}
	return out
}

// servers compares the MCP servers by agent, file and name.
func servers(prev, cur []inventory.MCPServer) []Change {
	key := func(s inventory.MCPServer) string {
		return s.Agent + "\x00" + s.File + "\x00" + s.Project + "\x00" + s.Name
	}
	has := func(list []inventory.MCPServer, k string) bool {
		return slices.ContainsFunc(list, func(s inventory.MCPServer) bool { return key(s) == k })
	}
	var out []Change
	for _, s := range cur {
		if !has(prev, key(s)) {
			out = append(out, Change{Kind: Added, Area: MCP, Name: s.Name, Agent: s.Agent, Path: s.File})
		}
	}
	for _, s := range prev {
		if !has(cur, key(s)) {
			out = append(out, Change{Kind: Removed, Area: MCP, Name: s.Name, Agent: s.Agent, Path: s.File})
		}
	}
	return out
}
