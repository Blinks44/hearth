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
	check := projects.Check{Tool: "node", Want: "18", File: ".nvmrc"}
	tests := []struct {
		is             issues.Issue
		title, explain string
		page           string
	}{
		{issues.Issue{Kind: issues.Duplicate, Tool: "node", Copies: []inventory.Installation{brew, fnm}},
			"Node: 2 copies on PATH",
			"The terminal runs v23.6.0 (brew). The other copies on PATH (v22.12.0 (fnm)) never run, and it is easy to update the wrong one.",
			"/tools/node"},
		{issues.Issue{Kind: issues.GUIDiffers, Tool: "node", Copies: []inventory.Installation{fnm, brew}},
			"Node differs outside the terminal",
			"Apps opened outside the terminal (VS Code, IDEs) find brew first and run v23.6.0 instead of v22.12.0.",
			"/tools/node"},
		{issues.Issue{Kind: issues.GUIDiffers, Tool: "node", Copies: []inventory.Installation{fnm}},
			"Node differs outside the terminal",
			"Apps opened outside the terminal (VS Code, IDEs) do not find node, though the terminal does.",
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
