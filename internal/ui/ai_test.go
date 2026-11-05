package ui

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

// aiSnapshot has Claude Code from Homebrew and Codex from npm; the others
// not found, Gemini's settings left. The user has a skill copied to two
// folders, one shared through a symlink, and one linked to nothing; MCP
// servers: github declared by both agents differently, linear over HTTP,
// and one whose command is gone.
func aiSnapshot() *inventory.Snapshot {
	brew := inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}
	npm := inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm"}
	s := &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Dir: home + "/.claude", Installations: []inventory.Installation{
				{Tool: "claude", Version: "2.1.308", Path: "/opt/homebrew/bin/claude", OnPath: 1, Source: brew},
			}, Files: []inventory.AgentFile{
				{Path: home + "/.claude/settings.json", Kind: inventory.FileSettings},
				{Path: home + "/.claude/CLAUDE.md", Kind: inventory.FileInstructions},
				{Path: home + "/.claude/skills", Kind: inventory.FileSkills},
			}},
			{ID: "codex", Dir: home + "/.codex", Installations: []inventory.Installation{
				{Tool: "codex", Version: "0.166.0", Path: home + "/.npm/bin/codex", OnPath: 1, InGUI: 1, Source: npm},
			}, Files: []inventory.AgentFile{{Path: home + "/.codex/AGENTS.md", Kind: inventory.FileInstructions}}},
			{ID: "copilot"},
			{ID: "gemini", Dir: home + "/.gemini", Files: []inventory.AgentFile{{Path: home + "/.gemini/settings.json"}}},
		},
		Skills: []inventory.Skill{
			{Name: "pdf", Description: "Read and fill PDFs", Dir: home + "/.claude/skills/pdf", Sum: "a",
				Paths: []inventory.SkillPath{{Path: home + "/.claude/skills/pdf", Agents: []string{"claude"}}}},
			{Name: "pdf", Description: "Read and fill PDFs", Dir: home + "/.agents/skills/pdf", Sum: "a",
				Paths: []inventory.SkillPath{{Path: home + "/.agents/skills/pdf", Agents: []string{"codex", "copilot"}}}},
			{Name: "plan", Description: "Plan the work", Dir: home + "/dev/skills/plan", Sum: "b",
				Paths: []inventory.SkillPath{
					{Path: home + "/.claude/skills/plan", Link: true, Agents: []string{"claude"}},
					{Path: home + "/.agents/skills/plan", Link: true, Agents: []string{"codex"}},
				}},
			{Name: "gone", Dir: home + "/old/gone", Broken: true,
				Paths: []inventory.SkillPath{{Path: home + "/.claude/skills/gone", Link: true, Agents: []string{"claude"}}}},
		},
		MCP: []inventory.MCPServer{
			{Name: "github", Agent: "claude", File: home + "/.claude.json", Transport: "stdio", Command: "npx",
				Args: []string{"-y", "@modelcontextprotocol/server-github"}, Env: []string{"GITHUB_TOKEN"}, CommandPath: "/opt/homebrew/bin/npx"},
			{Name: "linear", Agent: "claude", File: home + "/.claude.json", Transport: "http",
				URL: "https://mcp.linear.app/mcp", Headers: []string{"Authorization"}},
			{Name: "github", Agent: "codex", File: home + "/.codex/config.toml", Transport: "stdio", Command: "docker",
				Args: []string{"run", "ghcr.io/github/github-mcp-server"}, Env: []string{"GITHUB_TOKEN"}, CommandPath: "/usr/local/bin/docker"},
			{Name: "weftcut", Agent: "codex", File: home + "/.codex/config.toml", Transport: "stdio",
				Command: "/Applications/WeftCut.app/Contents/MacOS/WeftCut", Missing: true},
		},
	}
	return s
}

func openAI(t *testing.T, a *App, path string) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push(path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The agents found make a table with where they are, their version, what
// installed them, their skills, servers and instructions; the others are
// listed apart, with settings left told; a row opens the agent's page.
func TestAgentsPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	tt := openAI(t, a, "/ai/agents")
	hasTexts(t, tt, "AI agents", "Claude Code", "/opt/homebrew/bin/claude", "2.1.308", "brew", "CLAUDE.md",
		"Codex", "~/.npm/bin/codex", "0.166.0", "npm", "AGENTS.md",
		"Not found on this computer", "Copilot CLI", "Gemini CLI", "settings left in ~/.gemini")
	if err := tt.Click("Claude Code"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := agentPath("claude"); a.router.Location() != want {
		t.Fatalf("went to %q, want %q", a.router.Location(), want)
	}
	// Claude Code has pdf and plan, not the broken link; github and linear.
	hasTexts(t, tt, "installed through brew", "first in the terminal's PATH · brew", "Settings and instructions",
		"~/.claude/settings.json", "instructions for every project", "pdf", "plan", "all in Skills",
		"github", "linear", "2 servers")
	if tt.HasText("gone") {
		t.Errorf("a broken skill counts for the agent: %q", tt.Texts())
	}
	if err := tt.Click("all in Skills"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != skillsPage || a.skillAgent != "claude" {
		t.Errorf("went to %q filtered by %q", a.router.Location(), a.skillAgent)
	}
}

// Skills show by scope with their copies and broken links; the first is
// chosen, its agents, files and SKILL.md show, and filters narrow the list.
func TestSkillsPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Projects = []projects.Project{{Dir: home + "/dev/app", Skills: []inventory.Skill{
		{Name: "deploy", Description: "Ship the app", Dir: home + "/dev/app/.agents/skills/deploy", Project: home + "/dev/app",
			Paths: []inventory.SkillPath{{Path: home + "/dev/app/.agents/skills/deploy", Agents: []string{"codex"}}}},
	}}}
	var loads []string
	a.LoadSkill = func(dir string, done func([]inventory.Entry, string, error)) {
		loads = append(loads, dir)
		name := filepath.Base(dir)
		done([]inventory.Entry{{Name: "SKILL.md", Size: 4200}, {Name: "references", Dir: true, Files: 3}},
			"---\nname: "+name+"\n---\n\n# Steps of "+name+"\n", nil)
	}
	shell := &fakeShell{}
	a.Shell = shell
	tt := openAI(t, a, skillsPage)
	// The project's skills come first; the copies and the broken link are
	// tagged, and a hint tells of the copies.
	hasTexts(t, tt, "PROJECT · APP", ".agents/skills", "deploy", "USER", "pdf", "2 copies", "broken link",
		"1 skill is copied to several folders. One folder can stay, and the others become symlinks to it.",
		"AVAILABLE TO AGENTS", "SKILL.md", "5 KB", "references/", "3 files", "---\nname: deploy\n---", "Steps of deploy")
	if want := []string{home + "/dev/app/.agents/skills/deploy"}; !reflect.DeepEqual(loads, want) {
		t.Errorf("loaded %q, want %q", loads, want)
	}
	if err := tt.Click("Open in editor"); err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(home, "dev/app/.agents/skills/deploy/SKILL.md")}; !reflect.DeepEqual(shell.opened, want) {
		t.Errorf("opened %q, want %q", shell.opened, want)
	}

	// The linked skill shows its paths, the symlinks tagged.
	if err := tt.Click("plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "~/.claude/skills/plan", "~/.agents/skills/plan", "symlink")

	// Claude Code's skills alone: not the project's, which only Codex reads.
	a.skillAgent = "claude"
	tt.Frame()
	if tt.HasText("deploy") || !tt.HasText("pdf") {
		t.Errorf("filtered by Claude Code: %q", tt.Texts())
	}
	a.skillAgent, a.skillScope = "", 2
	tt.Frame()
	if tt.HasText("pdf") || !tt.HasText("deploy") {
		t.Errorf("projects' scope: %q", tt.Texts())
	}
	a.skillScope, a.skillQuery = 0, "nothing like it"
	tt.Frame()
	hasTexts(t, tt, "Nothing matches")
}

// Servers show once per name and scope, a conflict and a missing command
// tagged and the issue in a banner; details show the variables with their
// values hidden and where each agent declares it.
func TestMCPPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	tt := openAI(t, a, mcpPage)
	hasTexts(t, tt, "MCP servers", "github", "conflict", "linear", "http · https://mcp.linear.app/mcp",
		"weftcut", "no command", "MCP: WeftCut not found",
		"Declared for 2 agents. The agents start it differently.", "GITHUB_TOKEN", "••••••",
		"~/.claude.json · npx", "~/.codex/config.toml · docker")
	if err := tt.Click("weftcut"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "command not found", "not declared")
}

// The overview's card lists the agents found with their skills and
// servers, and the hero counts them.
func TestOverviewAgents(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	tt := openOverview(t, a)
	hasTexts(t, tt, "AI agents", "Skills and MCP", "Claude Code", "2 skills · 2 MCP", "Codex", "2 skills · 2 MCP",
		"Found 0 tools and 2 AI agents.")
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := agentPath("codex"); a.router.Location() != want {
		t.Errorf("went to %q, want %q", a.router.Location(), want)
	}
}

// The sidebar counts the agents found, the skills and the servers.
func TestSidebarAICounts(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	tt := openOverview(t, a)
	for _, want := range []string{"2", "4", "3"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	front, body := splitFrontmatter("---\r\nname: x\r\n---\r\n\r\n# X\r\n")
	if front != "---\nname: x\n---" || body != "\n# X\n" {
		t.Errorf("%q, %q", front, body)
	}
	if front, body := splitFrontmatter("# X"); front != "" || body != "# X" {
		t.Errorf("%q, %q", front, body)
	}
}
