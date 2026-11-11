// Package updates finds newer versions of the tools in a scan, from the
// cached version lists: plain rules over the snapshot and the projects,
// with no I/O.
package updates

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// Kind is how an update goes in.
type Kind int

const (
	// Patch is a newer release of a copy's own line: it replaces the copy
	// and its projects do not notice.
	Patch Kind = iota
	// NewLine is the newest stable line, none of whose releases is
	// installed: it goes in next to the copies, which stay the default.
	NewLine
)

// Update is one newer version of a tool.
type Update struct {
	Kind Kind
	// Tool is the ID of the tool, as "node".
	Tool string
	// Copy is the copy a Patch replaces; zero for a NewLine.
	Copy inventory.Installation
	// Version is the release to install, as "22.23.4".
	Version string
	// LTS is the codename of a Node release's LTS line, as "Jod"; "" for
	// another tool.
	LTS string
}

// Lists are the version lists the updates come from; a nil one means its
// tool is not known, and gets no updates.
type Lists struct {
	Node                          *versions.Node
	Go                            *versions.Go
	Python, PHP, Ruby, Java, Rust *versions.Cycles
	Dotnet                        *versions.Dotnet
	Bun                           *versions.Latest
	// Mise is for the tools of mise's without a page of their own.
	Mise *versions.Mise
}

// Find returns the updates of the scan s, by tool in the snapshot's order:
// a Patch per copy whose line has a newer release, then the NewLine. The
// system's copies, shims, stand-ins, copies no rule recognizes and
// prereleases get none. A copy a project asks for is not patched past what
// the project asks; rustup's stable toolchain moves to the newest line in
// place. now is the time support is judged at.
func Find(s *inventory.Snapshot, ps []projects.Project, l Lists, now time.Time) []Update {
	if s == nil {
		return nil
	}
	var out []Update
	for _, ts := range s.Tools {
		if ts.Err != "" {
			continue
		}
		if ts.Tool == "mise" {
			out = append(out, l.mise(ts.MiseTools)...)
			continue
		}
		t, ok := l.tool(ts.Tool, now)
		if !ok {
			continue
		}
		var wants []string
		for _, p := range ps {
			for _, r := range p.Requirements {
				if r.Tool == ts.Tool {
					wants = append(wants, r.Want)
				}
			}
		}
		lineOf := func(in inventory.Installation) string {
			// rustup update stable moves the stable toolchain to the
			// newest line.
			if in.Rust != nil && strings.HasPrefix(in.Rust.Name, "stable") && t.newest != "" {
				return t.line(t.newest)
			}
			return t.line(in.Version)
		}
		installed := map[string]bool{}
		lines := map[string]bool{}
		var newest *semver.Version
		var own []inventory.Installation
		for _, in := range ts.Installations {
			installed[in.Version] = true
			v := parse(in.Version)
			if v == nil || v.Prerelease() != "" || in.Stub || in.Source.Shim ||
				in.Source.Kind == inventory.KindSystem || in.Source.Kind == inventory.KindUnknown {
				continue
			}
			own = append(own, in)
			lines[lineOf(in)] = true
			if newest == nil || v.GreaterThan(newest) {
				newest = v
			}
		}
		for _, in := range own {
			to, ok := t.latest[lineOf(in)]
			if !ok || installed[to] || !newer(to, in.Version) || !keeps(ts.Tool, wants, in.Version, to, l.nodeLines()) {
				continue
			}
			out = append(out, Update{Kind: Patch, Tool: ts.Tool, Copy: in, Version: to, LTS: l.lts(to)})
		}
		if t.newest != "" && newest != nil && !lines[t.line(t.newest)] && newer(t.newest, newest.Original()) {
			out = append(out, Update{Kind: NewLine, Tool: ts.Tool, Version: t.newest, LTS: l.lts(t.newest)})
		}
	}
	return out
}

// mise is the updates of mise's tools by what mise outdated said: a
// Patch to the newest release the tool's config allows, a NewLine to the
// newest overall. A tool no config asks for gets only the NewLine, as
// mise installs it next to the copy.
func (l Lists) mise(tools []inventory.MiseTool) []Update {
	if l.Mise == nil {
		return nil
	}
	byDir := map[string]versions.MiseTool{}
	for name, t := range l.Mise.Tools {
		// installs/ names npm:prettier's folder npm-prettier.
		byDir[strings.NewReplacer(":", "-", "/", "-").Replace(name)] = t
	}
	var out []Update
	for _, mt := range tools {
		t, ok := byDir[mt.Name]
		if !ok {
			continue
		}
		i := slices.IndexFunc(mt.Installations, func(in inventory.Installation) bool { return in.Version == t.Current })
		if i < 0 {
			continue
		}
		installed := func(v string) bool {
			return slices.ContainsFunc(mt.Installations, func(in inventory.Installation) bool { return in.Version == v })
		}
		top := t.Current
		if newer(t.InRange, t.Current) && !installed(t.InRange) {
			out = append(out, Update{Kind: Patch, Tool: mt.Name, Copy: mt.Installations[i], Version: t.InRange})
			top = t.InRange
		}
		if newer(t.Latest, top) && !installed(t.Latest) {
			out = append(out, Update{Kind: NewLine, Tool: mt.Name, Version: t.Latest})
		}
	}
	return out
}

// keeps reports that moving a copy at from to to keeps every project that
// asks for from asking for to.
func keeps(tool string, wants []string, from, to string, lines map[int]versions.Line) bool {
	for _, want := range wants {
		if _, uses := projects.Pick(tool, want, []string{from}, lines); !uses {
			continue
		}
		if _, still := projects.Pick(tool, want, []string{to}, lines); !still {
			return false
		}
	}
	return true
}

// releases is what a tool's list says: the newest release of each line by
// the line's name, and the newest stable line's newest release.
type releases struct {
	line   func(version string) string
	latest map[string]string
	newest string
}

// tool reads tool's list; false when it has none.
func (l Lists) tool(tool string, now time.Time) (releases, bool) {
	switch tool {
	case "node":
		if l.Node == nil {
			return releases{}, false
		}
		r := releases{line: major, latest: map[string]string{}}
		for _, rel := range l.Node.Releases {
			if _, seen := r.latest[major(rel.Version)]; !seen {
				r.latest[major(rel.Version)] = rel.Version
			}
		}
		if rec, ok := l.Node.Recommended(); ok {
			r.newest = rec.Version
		}
		return r, true
	case "go":
		if l.Go == nil {
			return releases{}, false
		}
		r := releases{line: minor, latest: map[string]string{}}
		for _, v := range l.Go.Releases {
			if _, seen := r.latest[minor(v)]; !seen {
				r.latest[minor(v)] = v
			}
		}
		if len(l.Go.Releases) > 0 {
			r.newest = l.Go.Releases[0]
		}
		return r, true
	case "dotnet":
		if l.Dotnet == nil {
			return releases{}, false
		}
		r := releases{line: minor, latest: map[string]string{}}
		for _, dl := range l.Dotnet.Lines {
			if strings.Contains(dl.Version, "-") {
				continue // a preview
			}
			if _, seen := r.latest[minor(dl.Version)]; !seen {
				r.latest[minor(dl.Version)] = dl.Version
			}
			if r.newest == "" && dl.Phase != "eol" {
				r.newest = dl.Version
			}
		}
		return r, true
	case "bun":
		if l.Bun == nil {
			return releases{}, false
		}
		// Bun has no lines: every release follows the one before.
		return releases{line: func(string) string { return "" }, latest: map[string]string{"": l.Bun.Version}}, true
	}
	cycles := map[string]*versions.Cycles{"python": l.Python, "php": l.PHP, "ruby": l.Ruby, "java": l.Java, "rust": l.Rust}[tool]
	if cycles == nil {
		return releases{}, false
	}
	r := releases{
		line: func(v string) string {
			c, _ := cycles.Line(v)
			return c.Cycle
		},
		latest: map[string]string{},
		newest: cycles.Newest(now),
	}
	if tool == "java" {
		// A Java line without long-term support ends in half a year.
		r.newest = cycles.NewestLTS(now)
	}
	for _, c := range cycles.Lines {
		if c.Latest != "" {
			r.latest[c.Cycle] = c.Latest
		}
	}
	return r, true
}

func (l Lists) nodeLines() map[int]versions.Line {
	if l.Node == nil {
		return nil
	}
	return l.Node.Lines
}

// lts is the codename of the Node release v's line, "" when v is not a
// Node LTS release.
func (l Lists) lts(v string) string {
	if l.Node == nil {
		return ""
	}
	for _, r := range l.Node.Releases {
		if r.Version == v {
			return r.LTS
		}
	}
	return ""
}

// major is a version's line by its major number, as "22" of 22.12.0.
func major(v string) string {
	return strconv.Itoa(versions.Major(v))
}

// minor is a version's line by its first two numbers, as "1.27" of 1.27.3.
func minor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// newer reports that version a is past b; false when either is none.
func newer(a, b string) bool {
	va, vb := parse(a), parse(b)
	return va != nil && vb != nil && va.GreaterThan(vb)
}

// parse reads a version; OpenJDK's updates add a fourth number, as
// 21.0.12.1, which semver does not take. nil when it is no version.
func parse(v string) *semver.Version {
	if parts := strings.Split(v, "."); len(parts) > 3 {
		v = strings.Join(parts[:3], ".")
	}
	sv, err := semver.NewVersion(v)
	if err != nil {
		return nil
	}
	return sv
}
