// Package report writes the environment as text to paste into an issue or
// a chat: in English, without secrets and without personal paths. It does
// no I/O.
package report

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
)

// Info is what the report says of Hearth and the computer: never its name.
type Info struct {
	// Version is Hearth's, "" in a development build.
	Version string
	// OS is as "macOS 15.4 (arm64)".
	OS string
	// Home is the user's home, which the report writes as ~.
	Home string
}

// Write returns the report of s, the projects ps and the issues found.
// Paths under the home show as ~, projects by number, MCP servers by name
// and transport only: their commands, arguments and URLs may hold
// secrets.
func Write(s *inventory.Snapshot, ps []projects.Project, found []issues.Issue, info Info) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	path := func(p string) string { return private(p, info.Home) }
	version := info.Version
	if version == "" {
		version = "dev"
	}
	line("Hearth environment report")
	line("Hearth %s · %s", version, info.OS)
	if s == nil {
		line("Not checked yet.")
		return b.String()
	}
	line("Checked %s", s.Taken.UTC().Format("2006-01-02 15:04 UTC"))

	line("\n## Tools")
	for _, ts := range s.Tools {
		if ts.Tool == "packages" || ts.Tool == "databases" || len(ts.Installations) == 0 && ts.Err == "" {
			continue
		}
		term := inventory.TerminalCopy(ts.Installations)
		head := ts.Tool
		if term >= 0 {
			head += " " + or(ts.Installations[term].Version, "?") + " in the terminal"
		}
		if ts.Err != "" {
			head += " (not checked: " + ts.Err + ")"
		}
		line("- %s", head)
		for i, in := range ts.Installations {
			var tags []string
			if i == term {
				tags = append(tags, "runs")
			}
			if in.OnPath > 0 {
				tags = append(tags, fmt.Sprintf("PATH #%d", in.OnPath))
			}
			if in.InGUI > 0 {
				tags = append(tags, "apps")
			}
			if in.Default {
				tags = append(tags, "default")
			}
			if in.Source.Shim {
				tags = append(tags, "shim")
			}
			if in.Stub {
				tags = append(tags, "stub")
			}
			line("  - %s · %s · %s%s", or(in.Version, "?"), source(in.Source), path(in.Path), suffix(tags))
		}
		for _, m := range ts.Managers {
			line("  - manager %s%s", m.Name, suffix(nonEmpty(m.Version, flag(m.Activated, "activated"), flag(m.Shims, "shims"))))
		}
	}

	if st, ok := s.Tool("packages"); ok && len(st.PackageManagers) > 0 {
		line("\n## Package managers")
		for _, m := range st.PackageManagers {
			updates := "updates unknown"
			if m.UpdatesKnown {
				n := 0
				for _, p := range m.Packages {
					if p.Available != "" {
						n++
					}
				}
				updates = fmt.Sprintf("%d updates", n)
			}
			// brew --version says "Homebrew 4.4.20".
			line("- %s %s · %d packages · %s", m.Name, or(strings.TrimPrefix(m.Version, m.Name+" "), "?"), len(m.Packages), updates)
		}
	}

	if db, ok := s.Tool("databases"); ok && db.Databases != nil && len(db.Databases.Servers) > 0 {
		line("\n## Databases")
		for _, d := range db.Databases.Servers {
			line("- %s %s · %s%s", d.Kind, or(d.Version, "?"), or(d.Source, "?"), suffix(nonEmpty(d.Status, strings.Join(d.Ports, " "))))
		}
	}

	line("\n## PATH (terminal)")
	for i, e := range s.TerminalPath {
		mark := ""
		if e.Missing {
			mark = " (missing)"
		}
		line("%d. %s%s", i+1, or(path(e.Dir), "(empty)"), mark)
	}
	if only := onlyIn(s.TerminalPath, s.GUIPath); len(only) > 0 {
		line("Only the terminal has: %s", strings.Join(mapped(only, path), ", "))
	}

	if len(s.Variables) > 0 {
		line("\n## Variables")
		for _, v := range s.Variables {
			line("- %s: terminal %s · apps %s", v.Name, or(path(v.Terminal), "—"), or(path(v.GUI), "—"))
		}
	}

	line("\n## AI agents")
	for _, a := range s.Agents {
		if !a.Found() {
			continue
		}
		v := a.AppVersion
		if len(a.Installations) > 0 {
			v = a.Installations[0].Version
		}
		line("- %s %s", a.ID, or(v, "?"))
	}
	line("Skills: %d", len(s.Skills))
	if len(s.MCP) > 0 {
		line("MCP servers:")
		for _, m := range s.MCP {
			scope := "user"
			if m.Project != "" {
				scope = "project"
			}
			line("- %s · %s · %s · %s%s", m.Name, m.Agent, m.Transport, scope, suffix(nonEmpty(flag(m.Disabled, "disabled"), flag(m.Missing, "command missing"))))
		}
	}

	if len(ps) > 0 {
		line("\n## Projects")
		for i, p := range ps {
			var reqs []string
			for _, r := range p.Requirements {
				reqs = append(reqs, strings.TrimSpace(r.Tool+" "+r.Want)+" ("+r.File+")")
			}
			line("- project %d: %s", i+1, or(strings.Join(reqs, ", "), "no requirements"))
		}
	}

	line("\n## Issues")
	if len(found) == 0 {
		line("None.")
	}
	for _, is := range found {
		subject := is.Tool
		if is.Project != "" {
			subject += " in project " + projectNumber(ps, is.Project)
		}
		var versions []string
		for _, in := range is.Copies {
			versions = append(versions, or(in.Version, "?")+" ("+source(in.Source)+")")
		}
		if len(is.Dirs) > 0 {
			versions = append(versions, mapped(is.Dirs, path)...)
		}
		if len(is.Skills) > 0 {
			versions = append(versions, is.Skills[0].Name)
		}
		if len(is.MCP) > 0 {
			versions = append(versions, is.MCP[0].Name)
		}
		line("- %s%s%s", is.Kind, prefix(" · ", strings.TrimSpace(subject)), prefix(": ", strings.Join(versions, ", ")))
	}
	return b.String()
}

// private writes p with the home as ~; "" stays "".
func private(p, home string) string {
	if home == "" || p == "" {
		return p
	}
	if p == home {
		return "~"
	}
	for _, sep := range []string{"/", `\`} {
		if rest, ok := strings.CutPrefix(p, home+sep); ok {
			return "~" + sep + rest
		}
	}
	// Windows writes the home in any case.
	if len(p) > len(home) && strings.EqualFold(p[:len(home)], home) && (p[len(home)] == '\\' || p[len(home)] == '/') {
		return "~" + p[len(home):]
	}
	return p
}

func source(s inventory.Source) string {
	switch {
	case s.Manager != "":
		return s.Manager
	case s.Kind == inventory.KindSystem:
		return "system"
	case s.Kind == inventory.KindInstaller:
		return "installer"
	}
	return "unknown"
}

func projectNumber(ps []projects.Project, dir string) string {
	if i := slices.IndexFunc(ps, func(p projects.Project) bool { return p.Dir == dir }); i >= 0 {
		return fmt.Sprint(i + 1)
	}
	return "?"
}

// onlyIn are the entries of a that b has not.
func onlyIn(a, b []inventory.PathEntry) []string {
	var out []string
	for _, e := range a {
		if !slices.ContainsFunc(b, func(o inventory.PathEntry) bool { return o.Dir == e.Dir }) {
			out = append(out, e.Dir)
		}
	}
	return out
}

func mapped(list []string, f func(string) string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = f(s)
	}
	return out
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func flag(on bool, s string) string {
	if on {
		return s
	}
	return ""
}

func nonEmpty(list ...string) []string {
	return slices.DeleteFunc(list, func(s string) bool { return s == "" })
}

// suffix is " (a, b)", or "" for none.
func suffix(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return " (" + strings.Join(tags, ", ") + ")"
}

func prefix(p, s string) string {
	if s == "" {
		return ""
	}
	return p + s
}
