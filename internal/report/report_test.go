package report

import (
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
)

const home = "/Users/ann"

func TestWrite(t *testing.T) {
	fnm := inventory.Installation{Tool: "node", Version: "22.12.0", Path: home + "/.fnm/node-versions/v22.12.0/bin/node", OnPath: 1, InGUI: 1, Default: true,
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm", Root: home + "/.fnm/node-versions/v22.12.0"}}
	brew := inventory.Installation{Tool: "node", Version: "23.6.0", Path: "/opt/homebrew/bin/node", OnPath: 2,
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
	s := &inventory.Snapshot{
		Taken: time.Date(2026, 11, 6, 10, 42, 0, 0, time.UTC),
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{fnm, brew}, Managers: []inventory.Manager{{Name: "fnm", Version: "1.38.1"}}},
			{Tool: "packages", PackageManagers: []inventory.PackageManager{{Name: "Homebrew", Version: "Homebrew 4.4.20", UpdatesKnown: true,
				Packages: []inventory.SystemPackage{{Name: "node", Available: "24.0"}, {Name: "git"}}}}},
			{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{{Kind: "postgres", Name: "acme-billing-db", Version: "17.2", Source: "docker", Status: "running", Ports: []string{"5432"}}}}},
		},
		TerminalPath: []inventory.PathEntry{{Dir: home + "/.fnm/bin"}, {Dir: "/opt/homebrew/bin"}, {Dir: home + "/gone", Missing: true}},
		GUIPath:      []inventory.PathEntry{{Dir: "/opt/homebrew/bin"}},
		Variables:    []inventory.EnvironmentVariable{{Name: "JAVA_HOME", Terminal: home + "/.sdkman/candidates/java/current"}},
		Agents:       []inventory.Agent{{ID: "claude", Installations: []inventory.Installation{{Version: "2.1.3"}}}, {ID: "codex"}},
		Skills:       []inventory.Skill{{Name: "review", Dir: home + "/.claude/skills/review"}},
		MCP: []inventory.MCPServer{{Name: "github", Agent: "claude", File: home + "/.claude.json", Transport: "stdio",
			Command: "npx", Args: []string{"-y", "server-github", "--token", inventory.Hidden}, Env: []string{"GITHUB_TOKEN"}},
			{Name: "docs", Agent: "cursor", Transport: "http", URL: "https://mcp.acme.internal/docs", Project: home + "/work/acme-secret"}},
	}
	ps := []projects.Project{{Dir: home + "/work/acme-secret", Requirements: []projects.Requirement{{Tool: "node", Want: "18", File: ".nvmrc"}}}}
	found := []issues.Issue{
		{Kind: issues.Duplicate, Tool: "node", Copies: []inventory.Installation{fnm, brew}},
		{Kind: issues.ProjectMissing, Tool: "node", Project: home + "/work/acme-secret"},
		{Kind: issues.PathMissing, Dirs: []string{home + "/gone"}},
	}
	got := Write(s, ps, found, Info{Version: "0.1.0", OS: "macOS 15.4 (arm64)", Home: home})
	for _, want := range []string{
		"Hearth 0.1.0 · macOS 15.4 (arm64)", "Checked 2026-11-06 10:42 UTC",
		"- node 22.12.0 in the terminal", "  - 22.12.0 · fnm · ~/.fnm/node-versions/v22.12.0/bin/node (runs, PATH #1, apps, default)",
		"  - 23.6.0 · brew · /opt/homebrew/bin/node (PATH #2)", "  - manager fnm (1.38.1)",
		"- Homebrew 4.4.20 · 2 packages · 1 updates", "- postgres 17.2 · docker (running, 5432)",
		"1. ~/.fnm/bin", "3. ~/gone (missing)", "Only the terminal has: ~/.fnm/bin, ~/gone",
		"- JAVA_HOME: terminal ~/.sdkman/candidates/java/current · apps —",
		"- claude 2.1.3", "Skills: 1", "- github · claude · stdio · user", "- docs · cursor · http · project",
		"- project 1: node 18 (.nvmrc)",
		"- duplicate · node: 22.12.0 (fnm), 23.6.0 (brew)", "- project-missing · node in project 1", "- path-missing: ~/gone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	// Nothing personal or secret: the home, the project's and container's
	// names, MCP commands, arguments, URLs and variables, agents not found.
	for _, leak := range []string{home, "ann", "acme", "npx", "server-github", "--token", "GITHUB_TOKEN", "https://", "codex"} {
		if strings.Contains(got, leak) {
			t.Errorf("%q in the report:\n%s", leak, got)
		}
	}
}

func TestWriteBeforeScan(t *testing.T) {
	if got := Write(nil, nil, nil, Info{OS: "Ubuntu 24.04 (arm64)"}); !strings.Contains(got, "Hearth dev · Ubuntu 24.04 (arm64)") || !strings.Contains(got, "Not checked yet.") {
		t.Errorf("%s", got)
	}
}

func TestPrivate(t *testing.T) {
	for _, tc := range []struct{ path, home, want string }{
		{"/Users/ann/.bun", "/Users/ann", "~/.bun"},
		{"/Users/anna/.bun", "/Users/ann", "/Users/anna/.bun"},
		{"/Users/ann", "/Users/ann", "~"},
		{`c:\users\ann\scoop\shims`, `C:\Users\Ann`, `~\scoop\shims`},
		{"/opt/homebrew/bin", "/Users/ann", "/opt/homebrew/bin"},
	} {
		if got := private(tc.path, tc.home); got != tc.want {
			t.Errorf("private(%q, %q) = %q, want %q", tc.path, tc.home, got, tc.want)
		}
	}
}
