package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/locales"
)

// issueText says what an issue is and why it matters, in plain words;
// paths show home as ~.
func issueText(is issues.Issue, home string) (title, explain string) {
	tool := toolTitle(is.Tool)
	switch is.Kind {
	case issues.Duplicate:
		first := is.Copies[0]
		var others []string
		for _, in := range is.Copies[1:] {
			others = append(others, versionText(in.Version)+" ("+sourceName(in.Source)+")")
		}
		return locales.IssueDuplicateTitle(tool, len(is.Copies)),
			locales.IssueDuplicateExplain(versionText(first.Version), sourceName(first.Source), strings.Join(others, ", "))
	case issues.GUIDiffers:
		if len(is.Copies) < 2 {
			return locales.IssueGuiTitle(tool), locales.GuiNotFound(is.Tool)
		}
		term, gui := is.Copies[0], is.Copies[1]
		return locales.IssueGuiTitle(tool), locales.GuiRunsOther(sourceName(gui.Source), versionText(gui.Version), versionText(term.Version))
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
			run = versionText(is.Copies[0].Version)
		}
		return locales.IssueProjectTerminalTitle(filepath.Base(is.Project), tool),
			locales.IssueProjectTerminalExplain(is.Check.File, tool, is.Check.Want, run)
	case issues.Unsupported:
		var vs []string
		for _, in := range is.Copies {
			vs = append(vs, versionText(in.Version))
		}
		return locales.IssueUnsupportedTitle(tool, strings.Join(vs, ", ")), locales.IssueUnsupportedExplain()
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
	}
	return "", ""
}

// issuePage is the path of the page an issue lives on: its project's, its
// tool's, or the PATH's.
func issuePage(is issues.Issue) string {
	switch {
	case is.Project != "":
		return projectPath(is.Project)
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
