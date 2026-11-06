package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/locales"
)

// issueText says what an issue is and why it matters, in plain words;
// paths show home as ~.
func issueText(is issues.Issue, home string) (title, explain string) {
	tool := toolTitle(is.Tool)
	switch is.Kind {
	case issues.DatabasePortConflict:
		return locales.DatabaseConflictTitle(is.Dirs[0]), locales.DatabaseConflictExplain(filepath.Base(is.Project))
	case issues.DockerUnavailable:
		return locales.DockerUnavailableTitle(), locales.DockerUnavailableExplain()
	case issues.DockerDenied:
		return locales.DockerDeniedTitle(), locales.DockerDeniedExplain()
	case issues.Duplicate:
		first := is.Copies[0]
		version := copyVersion(first)
		if first.Version == "" {
			version = locales.CopyOfUnknownVersion()
		}
		var others []string
		for _, in := range is.Copies[1:] {
			others = append(others, copyVersion(in)+" ("+copySource(in)+")")
		}
		return locales.IssueDuplicateTitle(tool, len(is.Copies)),
			locales.IssueDuplicateExplain(version, copySource(first), strings.Join(others, ", "))
	case issues.GUIDiffers:
		if len(is.Copies) < 2 {
			return locales.IssueGuiMissingTitle(tool), locales.GuiNotFound(is.Tool)
		}
		term, gui := is.Copies[0], is.Copies[1]
		return locales.IssueGuiTitle(tool), locales.GuiRunsOther(sourceName(gui.Source), copyVersion(gui), copyVersion(term))
	case issues.ProjectMissing:
		want := is.Check.Want
		if want == "" {
			want = locales.AnyVersion()
		}
		return locales.IssueProjectMissingTitle(filepath.Base(is.Project), tool, want),
			locales.IssueProjectMissingExplain(is.Check.File, tool, want)
	case issues.ProjectTerminal:
		run := "—"
		if len(is.Copies) > 0 {
			run = copyVersion(is.Copies[0])
		}
		return locales.IssueProjectTerminalTitle(filepath.Base(is.Project), tool),
			locales.IssueProjectTerminalExplain(is.Check.File, tool, is.Check.Want, run)
	case issues.Unsupported:
		var vs []string
		for _, in := range is.Copies {
			vs = append(vs, copyVersion(in))
		}
		return locales.IssueUnsupportedTitle(tool, strings.Join(vs, ", ")), locales.IssueUnsupportedExplain()
	case issues.ExtraVersion:
		var vs []string
		for _, in := range is.Copies {
			vs = append(vs, copyVersion(in)+" ("+copySource(in)+")")
		}
		return locales.IssueExtraTitle(tool, len(is.Copies)), locales.IssueExtraExplain(strings.Join(vs, ", "))
	case issues.StubOnPath:
		in := is.Copies[0]
		if in.OnPath != 1 {
			return locales.IssueStubBehindTitle(tool), stubWarning([]inventory.Installation{in}, []int{0}, home)
		}
		return locales.IssueStubTitle(tool), stubWarning([]inventory.Installation{in}, []int{0}, home)
	case issues.PathCurrentDir:
		var entries []string
		for _, d := range is.Dirs {
			if d == "" {
				entries = append(entries, locales.EmptyPathEntry())
			} else {
				entries = append(entries, strconv.Quote(d))
			}
		}
		return locales.IssuePathCurrentTitle(), locales.IssuePathCurrentExplain(strings.Join(entries, ", "))
	case issues.PathMissing:
		dirs := make([]string, len(is.Dirs))
		for i, d := range is.Dirs {
			dirs[i] = tilde(d, home)
		}
		return locales.IssuePathMissingTitle(), locales.IssuePathMissingExplain(strings.Join(dirs, ", "))
	case issues.SkillCopies, issues.SkillDiffers:
		dirs := make([]string, len(is.Skills))
		for i, sk := range is.Skills {
			dirs[i] = tilde(sk.Dir, home)
		}
		name := is.Skills[0].Name
		if is.Kind == issues.SkillCopies {
			return locales.IssueSkillCopiesTitle(name, len(is.Skills)), locales.IssueSkillCopiesExplain(strings.Join(dirs, ", "))
		}
		return locales.IssueSkillDiffersTitle(name), locales.IssueSkillDiffersExplain(strings.Join(dirs, ", "))
	case issues.MCPMissing:
		// Servers by name, each with the agents declaring it.
		var names []string
		agents := map[string][]string{}
		for _, sv := range is.MCP {
			if _, ok := agents[sv.Name]; !ok {
				names = append(names, sv.Name)
			}
			if a := agentTitle(sv.Agent); !slices.Contains(agents[sv.Name], a) {
				agents[sv.Name] = append(agents[sv.Name], a)
			}
		}
		servers := make([]string, len(names))
		for i, n := range names {
			servers[i] = n + " (" + strings.Join(agents[n], ", ") + ")"
		}
		// The title names the command; the explanation says where it is.
		command := is.MCP[0].Command
		return locales.IssueMcpMissingTitle(filepath.Base(command)),
			locales.IssueMcpMissingExplain(strings.Join(servers, ", "), tilde(command, home))
	}
	return "", ""
}

// skillsPage and mcpPage are where skills' and MCP servers' issues live.
const (
	skillsPage = "/ai/skills"
	mcpPage    = "/ai/mcp"
)

// copySource is sourceName, a manager's shim named as one.
func copySource(in inventory.Installation) string {
	if in.Source.Shim && in.Source.Manager != "" {
		return locales.ShimOf(in.Source.Manager)
	}
	return sourceName(in.Source)
}

// issuePage is the path of the page an issue lives on: the skills', its
// project's, its tool's, or the PATH's.
func issuePage(is issues.Issue) string {
	switch {
	case len(is.Skills) > 0:
		return skillsPage
	case len(is.MCP) > 0:
		return mcpPage
	case is.Tool == "databases":
		return "/tools/databases"
	case is.Project != "":
		return projectPath(is.Project)
	case is.Tool == "docker":
		return "/tools/docker"
	case jsTool(is.Tool):
		return "/tools/jspm"
	case is.Tool != "":
		for _, l := range languages {
			if l.id == is.Tool {
				return l.path
			}
		}
		return overview.path
	}
	return pathPage
}
