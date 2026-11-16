// Package cleanup finds what could go to free disk space: candidates, each
// with the reasons Hearth offers it, never a claim that it is unused.
// Plain rules over the snapshot, the sizes and the projects, with no I/O.
package cleanup

import (
	"cmp"
	"path/filepath"
	"slices"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// Kind is the rule a candidate comes from.
type Kind string

const (
	// Cache is a tool's cache: the tool downloads what it needs again.
	Cache Kind = "cache"
	// ExtraCopy is a copy of a tool nothing runs: neither the terminal
	// nor apps, its manager does not pick it by default, and no project
	// asks for it.
	ExtraCopy Kind = "extra-copy"
	// Leftover is the copies of a version manager the shell does not set
	// up, so none of them runs: what remains of a manager left for another.
	Leftover Kind = "leftover"
	// UnneededPackage is a package another pulled in that no installed
	// package needs any more.
	UnneededPackage Kind = "unneeded-package"
	// BuildCache is Docker's build cache: builds make it again.
	BuildCache Kind = "docker-build-cache"
	// UnusedImages are Docker images no container uses: pulled again when
	// needed.
	UnusedImages Kind = "docker-images"
	// StoppedContainers are Docker containers not running.
	StoppedContainers Kind = "docker-containers"
	// ProjectInstalls is the dependencies of a project not changed for long,
	// as its node_modules: installed again from its files.
	ProjectInstalls Kind = "project-installs"
	// Model is a local AI model: Ollama or LM Studio downloads it again.
	Model Kind = "model"
	// OldExtensions are the older versions of an editor's extensions an
	// update left behind.
	OldExtensions Kind = "old-extensions"
	// AgentHistory is an AI agent's sessions not written to for long.
	AgentHistory Kind = "agent-history"
)

// Group is a section candidates come in, in this order.
type Group int

const (
	Caches Group = iota
	Copies
	Docker
	Projects
	Models
	History
)

// Group returns the section of the kind.
func (k Kind) Group() Group {
	switch k {
	case Cache:
		return Caches
	case BuildCache, UnusedImages, StoppedContainers:
		return Docker
	case ProjectInstalls:
		return Projects
	case Model:
		return Models
	case AgentHistory:
		return History
	}
	return Copies
}

// Reason is why Hearth offers a candidate.
type Reason string

const (
	// Restorable: the tool fetches it again when it needs it.
	Restorable Reason = "restorable"
	// NotDefault: its manager picks another version by default.
	NotDefault Reason = "not-default"
	// Shadowed: on the terminal's PATH, after the copy that runs.
	Shadowed Reason = "shadowed"
	// NotOnPath: on neither the terminal's PATH nor the apps'.
	NotOnPath Reason = "not-on-path"
	// NoProject: no project Hearth knows asks for it.
	NoProject Reason = "no-project"
	// NotSetUp: the shell does not load its manager.
	NotSetUp Reason = "not-set-up"
	// NothingNeeds: no installed package depends on it.
	NothingNeeds Reason = "nothing-needs"
	// Rebuilt: builds make it again.
	Rebuilt Reason = "rebuilt"
	// Pulled: pulled again when a container needs it.
	Pulled Reason = "pulled"
	// Stopped: not running.
	Stopped Reason = "stopped"
	// Unchanged: the project has not changed since Candidate.Changed.
	Unchanged Reason = "unchanged"
	// Reinstalled: installed again from the project's files.
	Reinstalled Reason = "reinstalled"
	// NoAgent: no MCP server of the agents' names it.
	NoAgent Reason = "no-agent"
	// Downloaded: its app downloads it again.
	Downloaded Reason = "downloaded"
	// Superseded: a newer version of it is installed.
	Superseded Reason = "superseded"
	// Forgotten: the sessions leave the usage page, and the agent can no
	// longer resume them.
	Forgotten Reason = "forgotten"
)

// Candidate is something the user may remove to free space.
type Candidate struct {
	Kind Kind
	// ID names it in its kind: a cache's ID, as "npm"; a tool's, as
	// "node"; a version manager's, as "nvm"; a package's name.
	ID string
	// Manager is the package manager of an UnneededPackage, as "brew";
	// the app of a Model, "Ollama" or "LM Studio".
	Manager string `json:",omitempty"`
	// Paths are the folders that would go; none for Docker's.
	Paths []string
	// Copies are the copies of a tool that would go, for ExtraCopy and
	// Leftover.
	Copies []inventory.Installation
	// Project is the folder of the project, for ProjectInstalls; Changed
	// when it last changed.
	Project string
	Changed time.Time
	// Count is how many of Docker's images or containers, of the folders
	// of OldExtensions, or of the sessions of AgentHistory.
	Count int
	// Usage is the space it takes, when Known; Docker's is what pruning
	// frees.
	Usage inventory.Usage
	Known bool
	// Selected reports that it is chosen at once: what comes back by
	// itself, the caches and Docker's build cache, and the installs of a
	// project not changed for long.
	Selected bool
	Reasons  []Reason
}

// Frees is the maximum space removing it frees: what files linked from elsewhere
// keep aside.
func (c Candidate) Frees() int64 { return c.Usage.Bytes - c.Usage.Shared }

// Estimated reports that the tool may keep unshared files still in use.
// pnpm can import packages by cloning, without hard links to reveal usage.
func (c Candidate) Estimated() bool { return c.Kind == Cache && c.ID == "pnpm" }

// minFrees is the least a cache or Docker's data must free to be offered:
// less is not worth a look.
const minFrees = 1_000_000

// minModel is the least a model must take to be offered: a small one, as
// an embedding model, is not worth downloading again.
const minModel = 1_000_000_000

// Find returns the candidates in s, sizes and the projects ps, by group
// and then the biggest first. Volumes, the system's copies and what
// projects ask for are never candidates. lines are Node's release lines,
// nil when not known, which projects' "lts/*" needs; now is the time a
// project's age is judged at.
func Find(s *inventory.Snapshot, sizes *inventory.Sizes, ps []projects.Project, lines map[int]versions.Line, now time.Time) []Candidate {
	if s == nil {
		return nil
	}
	var out []Candidate
	usage := func(paths ...string) (inventory.Usage, bool) {
		var total inventory.Usage
		if sizes == nil || len(paths) == 0 {
			return total, false
		}
		for _, p := range paths {
			u, ok := sizes.Paths[p]
			if !ok {
				return inventory.Usage{}, false
			}
			total.Bytes += u.Bytes
			total.Shared += u.Shared
			total.Partial = total.Partial || u.Partial
		}
		return total, true
	}
	if sizes != nil {
		for _, c := range sizes.Caches {
			u, ok := usage(c.Path)
			if ok && u.Bytes-u.Shared >= minFrees {
				out = append(out, Candidate{Kind: Cache, ID: c.ID, Paths: []string{c.Path}, Usage: u, Known: true, Selected: true, Reasons: []Reason{Restorable}})
			}
		}
		out = append(out, docker(sizes.Docker)...)
	}
	out = append(out, copies(s, ps, lines, usage)...)
	out = append(out, models(s, ps)...)
	out = append(out, oldExtensions(s, usage)...)
	out = append(out, installs(sizes, ps, usage, now)...)
	out = append(out, history(s.Sessions, now)...)
	slices.SortStableFunc(out, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(a.Kind.Group(), b.Kind.Group()), cmp.Compare(b.Frees(), a.Frees()))
	})
	return out
}

// installs offers projects' folders of dependencies: those of the known
// projects not changed for long, and every node_modules found under home,
// chosen at once only when its project has not changed for long.
func installs(sizes *inventory.Sizes, ps []projects.Project, usage func(...string) (inventory.Usage, bool), now time.Time) []Candidate {
	var out []Candidate
	add := func(project, name string, changed time.Time) {
		path := filepath.Join(project, name)
		if slices.ContainsFunc(out, func(c Candidate) bool { return c.Paths[0] == path }) {
			return
		}
		stale := !changed.IsZero() && now.Sub(changed) >= projects.StaleAfter
		reasons := []Reason{Reinstalled}
		if stale {
			reasons = []Reason{Unchanged, Reinstalled}
		}
		u, ok := usage(path)
		out = append(out, Candidate{Kind: ProjectInstalls, ID: name, Paths: []string{path}, Project: project, Changed: changed,
			Usage: u, Known: ok, Selected: stale, Reasons: reasons})
	}
	for _, p := range ps {
		if p.Changed.IsZero() || now.Sub(p.Changed) < projects.StaleAfter {
			continue
		}
		for _, name := range p.Installs {
			add(p.Dir, name, p.Changed)
		}
	}
	if sizes != nil {
		for _, in := range sizes.Installs {
			add(in.Project, in.Name, in.Changed)
		}
	}
	return out
}

// docker offers what pruning frees: the build cache, images without
// containers, stopped containers. Volumes hold data, so never.
func docker(rows []inventory.DockerUsage) []Candidate {
	var out []Candidate
	for _, r := range rows {
		c := Candidate{Count: r.Count - r.Active, Usage: inventory.Usage{Bytes: r.Reclaimable}, Known: true}
		switch r.Type {
		case "Build Cache":
			c.Kind, c.Selected, c.Reasons = BuildCache, true, []Reason{Rebuilt}
		case "Images":
			c.Kind, c.Reasons = UnusedImages, []Reason{Pulled}
		case "Containers":
			c.Kind, c.Reasons = StoppedContainers, []Reason{Stopped}
		default:
			continue
		}
		// Docker counts an image a stopped container uses as reclaimable,
		// though it goes only with the container: none without a container
		// is nothing to offer.
		if c.Kind != BuildCache && c.Count <= 0 {
			continue
		}
		if r.Reclaimable >= minFrees {
			out = append(out, c)
		}
	}
	return out
}

// copies offers the copies of tools nothing runs, by their root: a root
// goes only when no copy in it runs, as npm in Node's folder. A version
// manager none of whose copies runs, and the shell does not set up, is a
// Leftover; its copies come together. Then the packages another pulled
// in that nothing needs, of those that are no such copy.
func copies(s *inventory.Snapshot, ps []projects.Project, lines map[int]versions.Line, usage func(...string) (inventory.Usage, bool)) []Candidate {
	// kept are the roots that stay: a copy in them runs, or a project or
	// a package needs it.
	kept := map[string]bool{}
	for _, p := range ps {
		for _, st := range projects.Resolve(p, s, lines) {
			if st.Terminal != nil {
				kept[st.Terminal.Source.Root] = true
			}
			for _, ch := range st.Checks {
				if ch.Installed != nil {
					kept[ch.Installed.Source.Root] = true
				}
			}
		}
	}
	type root struct {
		path   string
		tool   string
		copies []inventory.Installation
	}
	var roots []*root
	// set are the version managers the shell sets up or whose copies run.
	set := map[string]bool{}
	// add takes a tool's copies by their roots, keeping the ones that run.
	add := func(tool string, ins []inventory.Installation) {
		term, gui := inventory.TerminalCopy(ins), inventory.GUICopy(ins)
		for i, in := range ins {
			if in.OnPath > 0 || in.InGUI > 0 {
				set[in.Source.Manager] = true
			}
			r := in.Source.Root
			// The system's and an installer's in a shared folder are
			// not the copy's own folder to remove. uv's Pythons are never
			// on PATH: virtual environments and uv run use them by path.
			if r == "" || in.Source.Kind == inventory.KindSystem || in.Source.Kind == inventory.KindInstaller && in.Source.Manager == "" ||
				in.Source.Manager == "uv" {
				continue
			}
			if i == term || i == gui || in.Default || in.Source.Shim || in.Stub || inventory.NeededBySystem(in) {
				kept[r] = true
			}
			j := slices.IndexFunc(roots, func(x *root) bool { return x.path == r })
			if j < 0 {
				roots = append(roots, &root{path: r, tool: tool})
				j = len(roots) - 1
			}
			roots[j].copies = append(roots[j].copies, in)
		}
	}
	for _, ts := range s.Tools {
		for _, m := range ts.Managers {
			if m.Activated || m.Shims || m.ShimsInGUI {
				set[m.Name] = true
			}
		}
		for _, m := range ts.PackageManagers {
			deps := dependents(m)
			for _, p := range m.Packages {
				if deps[p.Name] > 0 {
					kept[p.Path] = true
				}
			}
		}
		add(ts.Tool, ts.Installations)
		// mise's tools without a page are tools of their own.
		for _, mt := range ts.MiseTools {
			add(mt.Name, mt.Installations)
		}
	}
	var out []Candidate
	offered := map[string]bool{}
	leftovers := map[string]int{}
	for _, r := range roots {
		if kept[r.path] {
			continue
		}
		offered[r.path] = true
		in := r.copies[0]
		u, ok := usage(r.path)
		if in.Source.Kind == inventory.KindVersionManager && !set[in.Source.Manager] {
			if i, ok := leftovers[in.Source.Manager]; ok {
				c := &out[i]
				c.Paths, c.Copies = append(c.Paths, r.path), append(c.Copies, r.copies...)
				c.Usage, c.Known = usage(c.Paths...)
				continue
			}
			leftovers[in.Source.Manager] = len(out)
			out = append(out, Candidate{Kind: Leftover, ID: in.Source.Manager, Paths: []string{r.path}, Copies: r.copies, Usage: u, Known: ok,
				Reasons: []Reason{NotSetUp, NoProject}})
			continue
		}
		var reasons []Reason
		if in.Source.Kind == inventory.KindVersionManager {
			reasons = append(reasons, NotDefault)
		}
		if slices.ContainsFunc(r.copies, func(c inventory.Installation) bool { return c.OnPath > 0 }) {
			reasons = append(reasons, Shadowed)
		} else {
			reasons = append(reasons, NotOnPath)
		}
		out = append(out, Candidate{Kind: ExtraCopy, ID: r.tool, Paths: []string{r.path}, Copies: r.copies, Usage: u, Known: ok,
			Reasons: append(reasons, NoProject)})
	}
	for _, ts := range s.Tools {
		for _, m := range ts.PackageManagers {
			// Only a manager listing every package's dependencies knows
			// that nothing needs one.
			if m.ID != "brew" || !m.ReasonsKnown {
				continue
			}
			for _, p := range m.Packages {
				if !p.Dependency || p.Path == "" || kept[p.Path] || offered[p.Path] {
					continue
				}
				u, ok := usage(append([]string{p.Path}, p.Artifacts...)...)
				out = append(out, Candidate{Kind: UnneededPackage, ID: p.Name, Manager: m.ID, Paths: append([]string{p.Path}, p.Artifacts...),
					Usage: u, Known: ok, Reasons: []Reason{NothingNeeds}})
			}
		}
	}
	return out
}

// dependents counts, by name, the installed packages of m that depend on
// each.
func dependents(m inventory.PackageManager) map[string]int {
	out := map[string]int{}
	for _, p := range m.Packages {
		for _, d := range p.Dependencies {
			out[d]++
		}
	}
	return out
}

// models are the local models of a gigabyte or more that are not in
// memory and no MCP server names, never chosen at once: what to keep is
// the user's call.
func models(s *inventory.Snapshot, ps []projects.Project) []Candidate {
	st, _ := s.Tool("models")
	if st.Models == nil {
		return nil
	}
	servers := slices.Clone(s.MCP)
	for _, p := range ps {
		servers = append(servers, p.MCP...)
	}
	var out []Candidate
	for _, app := range []struct {
		name string
		app  *inventory.ModelApp
	}{{"Ollama", st.Models.Ollama}, {"LM Studio", st.Models.LMStudio}} {
		if app.app == nil {
			continue
		}
		for _, m := range app.app.Models {
			if m.Size < minModel || m.Loaded || len(inventory.ModelMentions(servers, m)) > 0 {
				continue
			}
			out = append(out, Candidate{Kind: Model, ID: m.Name, Manager: app.name, Paths: []string{m.Path}, Changed: m.Downloaded,
				Usage: inventory.Usage{Bytes: m.Size, Shared: m.Size - m.Frees}, Known: true, Reasons: []Reason{NoAgent, Downloaded}})
		}
	}
	return out
}

// HistoryAfter is how long an agent's session goes unwritten before
// Cleanup offers it.
const HistoryAfter = 90 * 24 * time.Hour

// history offers the sessions of each agent not written to for
// HistoryAfter, never chosen at once: the user may want to resume one.
func history(sessions []inventory.AgentSession, now time.Time) []Candidate {
	var out []Candidate
	at := map[string]int{}
	for _, s := range sessions {
		if now.Sub(s.Modified) < HistoryAfter {
			continue
		}
		i, ok := at[s.Agent]
		if !ok {
			at[s.Agent] = len(out)
			out = append(out, Candidate{Kind: AgentHistory, ID: s.Agent, Known: true, Reasons: []Reason{Forgotten}})
			i = len(out) - 1
		}
		c := &out[i]
		c.Paths = append(c.Paths, s.Files...)
		c.Count++
		c.Usage.Bytes += s.Size
	}
	return slices.DeleteFunc(out, func(c Candidate) bool { return c.Usage.Bytes < minFrees })
}

// oldExtensions are the folders of extensions' older versions, an
// editor each.
func oldExtensions(s *inventory.Snapshot, usage func(...string) (inventory.Usage, bool)) []Candidate {
	st, _ := s.Tool("editors")
	if st.Editors == nil {
		return nil
	}
	var out []Candidate
	for _, e := range st.Editors.Editors {
		if len(e.Superseded) == 0 {
			continue
		}
		u, ok := usage(e.Superseded...)
		out = append(out, Candidate{Kind: OldExtensions, ID: e.Name, Paths: e.Superseded, Count: len(e.Superseded), Usage: u, Known: ok, Reasons: []Reason{Superseded}})
	}
	return out
}
