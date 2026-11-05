package ui

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
)

func TestIssueText(t *testing.T) {
	brew := inventory.Installation{Version: "23.6.0", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
	fnm := inventory.Installation{Version: "22.12.0", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}}
	miseShim := inventory.Installation{Version: "24.22.2", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Shim: true}}
	check := projects.Check{Tool: "node", Want: "18", File: ".nvmrc"}
	pdf := inventory.Skill{Name: "pdf", Dir: home + "/.claude/skills/pdf"}
	sharedPDF := inventory.Skill{Name: "pdf", Dir: home + "/.agents/skills/pdf"}
	tests := []struct {
		is             issues.Issue
		title, explain string
		page           string
	}{
		{issues.Issue{Kind: issues.Duplicate, Tool: "node", Copies: []inventory.Installation{brew, fnm}},
			"Node: 2 copies on PATH",
			"The terminal runs v23.6.0 (brew). The other copies on PATH (v22.12.0 (fnm)) never run, and it is easy to update the wrong one.",
			"/tools/node"},
		{issues.Issue{Kind: issues.Duplicate, Tool: "node", Copies: []inventory.Installation{brew, miseShim}},
			"Node: 2 copies on PATH",
			"The terminal runs v23.6.0 (brew). The other copies on PATH (v24.22.2 (mise shim)) never run, and it is easy to update the wrong one.",
			"/tools/node"},
		{issues.Issue{Kind: issues.GUIDiffers, Tool: "node", Copies: []inventory.Installation{fnm, brew}},
			"Node differs outside the terminal",
			"Apps started outside the terminal find brew first and run v23.6.0 instead of v22.12.0.",
			"/tools/node"},
		{issues.Issue{Kind: issues.GUIDiffers, Tool: "node", Copies: []inventory.Installation{fnm}},
			"Apps outside the terminal do not see Node",
			"Apps started outside the terminal do not find node, MCP servers in desktop clients for one. VS Code and JetBrains IDEs usually take the terminal's PATH themselves.",
			"/tools/node"},
		{issues.Issue{Kind: issues.ProjectMissing, Tool: "node", Project: home + "/work/legacy-admin", Check: check},
			"legacy-admin needs Node 18",
			".nvmrc asks for Node 18, and no installed copy fits. Until it is installed, the project may not build or run.",
			projectPath(home + "/work/legacy-admin")},
		{issues.Issue{Kind: issues.ProjectTerminal, Tool: "node", Project: home + "/work/legacy-admin", Check: check, Copies: []inventory.Installation{fnm}},
			"legacy-admin: the terminal runs another Node",
			".nvmrc asks for Node 18. It is installed, but in this folder the terminal runs v22.12.0.",
			projectPath(home + "/work/legacy-admin")},
		{issues.Issue{Kind: issues.Unsupported, Tool: "node", Copies: []inventory.Installation{{Version: "18.20.4"}}},
			"Node v18.20.4: support ended",
			"These versions no longer get security fixes. Move projects to a supported version, then the old one can go.",
			"/tools/node"},
		{issues.Issue{Kind: issues.PathCurrentDir, Dirs: []string{".", ""}},
			"PATH includes the current folder",
			`PATH has ".", an empty entry. A command typed in the terminal may then run a file from whatever folder you are in, even a downloaded one.`,
			pathPage},
		{issues.Issue{Kind: issues.PathMissing, Dirs: []string{home + "/.old/bin", "/opt/gone"}},
			"PATH lists folders that do not exist",
			"~/.old/bin, /opt/gone: no such folders. Nothing breaks, but these are usually leftovers of removed programs and can go.",
			pathPage},
		{issues.Issue{Kind: issues.SkillCopies, Skills: []inventory.Skill{pdf, sharedPDF}},
			"Skill pdf: 2 copies",
			"The same files are in ~/.claude/skills/pdf, ~/.agents/skills/pdf. An edit to one copy misses the others: keep one and link the rest to it.",
			skillsPage},
		{issues.Issue{Kind: issues.SkillDiffers, Project: home + "/work/app", Skills: []inventory.Skill{pdf, sharedPDF}},
			"Copies of skill pdf differ",
			"~/.claude/skills/pdf, ~/.agents/skills/pdf hold different versions of it, so agents may follow different instructions. Keep the right one and link the rest to it.",
			skillsPage},
		{issues.Issue{Kind: issues.MCPMissing, MCP: []inventory.MCPServer{
			{Name: "github", Agent: "claude", Command: "docker"}, {Name: "github", Agent: "cursor", Command: "docker"},
			{Name: "sentry", Agent: "codex", Command: "docker"},
		}},
			"MCP: docker not found",
			"github (Claude Code, Cursor), sentry (Codex) start docker, but it is not there. Until it is installed or the path is fixed, the agents cannot start these servers.",
			mcpPage},
	}
	for _, tt := range tests {
		title, explain := issueText(tt.is, home)
		if title != tt.title || explain != tt.explain {
			t.Errorf("%s:\n got %q, %q\nwant %q, %q", tt.is.Kind, title, explain, tt.title, tt.explain)
		}
		if got := issuePage(tt.is); got != tt.page {
			t.Errorf("%s: page %q, want %q", tt.is.Kind, got, tt.page)
		}
	}
}

// A stub's issue says what the stub does instead of running the tool.
func TestIssueTextStub(t *testing.T) {
	is := issues.Issue{Kind: issues.StubOnPath, Tool: "python", Copies: []inventory.Installation{
		{Tool: "python", Path: "/usr/bin/python3", Stub: true, OnPath: 1}}}
	title, explain := issueText(is, "")
	if title != "A stub stands in for Python in PATH" ||
		explain != "/usr/bin/python3 is a stub: without Xcode's Command Line Tools it offers to install them instead of running Python." {
		t.Errorf("got %q, %q", title, explain)
	}
}
