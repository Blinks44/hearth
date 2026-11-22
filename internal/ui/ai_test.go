package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
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
			{ID: "copilot", Dir: home + "/.copilot", Files: []inventory.AgentFile{{Path: home + "/.agents/skills"}}},
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

func TestAntigravityAgentPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Snapshot.Agents = append(a.Snapshot.Agents, inventory.Agent{
		ID: "antigravity", Dir: home + "/.gemini/antigravity-cli",
		Installations: []inventory.Installation{
			{Tool: "antigravity", Version: "1.5.12", Path: home + "/.local/bin/agy", OnPath: 1},
		},
		Files: []inventory.AgentFile{
			{Path: home + "/.gemini/GEMINI.md", Kind: inventory.FileInstructions},
			{Path: home + "/.gemini/config/mcp_config.json", Kind: inventory.FileMCP},
		},
		Rules: []inventory.AgentRule{{Kind: inventory.RuleAsk, Rule: "command(*)"}},
		Hooks: []inventory.AgentHook{{Event: "PostToolUse", Command: "make lint"}},
	})
	tt := openAI(t, a, "/ai/agents")
	hasTexts(t, tt, "Antigravity CLI", "Gemini CLI", "1.5.12")
	if err := tt.Click("Antigravity CLI"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Switching from Gemini CLI", "GEMINI.md", "mcp_config.json", "command(*)", "make lint")
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
	if tt.HasText("settings left in ~/.copilot") {
		t.Fatal("a shared skills directory told as this agent's settings")
	}
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

// An agent not found installs through the system's package manager; a
// copy Homebrew installed tells its update and updates through Homebrew,
// one npm installed updates through npm.
func TestAgentOperations(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	root := "/opt/homebrew/Caskroom/claude-code"
	a.Snapshot.Agents[0].Installations[0].Source.Root = root
	a.Snapshot.Tools = []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
		Packages: []inventory.SystemPackage{{Name: "claude-code", Kind: "cask", Version: "2.1.308", Available: "2.1.322", Path: root}}}}}}
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, nil))
	}
	tt := openAI(t, a, "/ai/agents")
	screenshot(t, tt, "agents-actions")
	hasTexts(t, tt, "2.1.322 available")
	if err := tt.Click("Install…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "install", "--cask", "copilot-cli"}) {
		t.Fatalf("plan %+v", a.op)
	}
	a.op = nil
	a.router.Push(agentPath("claude"))
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Update to 2.1.322"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "upgrade", "--cask", "claude-code"}) {
		t.Fatalf("plan %+v", a.op)
	}
	a.op = nil
	a.router.Push(agentPath("codex"))
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Update through npm…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"npm", "install", "--global", "--prefix", filepath.Dir(filepath.Dir(a.Snapshot.Agents[1].Installations[0].Path)), "@openai/codex@latest"}) {
		t.Fatalf("plan %+v", a.op)
	}
}

// A skill an agent does not get offers to connect it; a link it gets it
// by switches off; the copies' hint offers to merge them. Each plan is
// made in the background.
func TestSkillOperations(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.SkillBackups = home + "/backups"
	built := 0
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		built++
	}
	tt := openAI(t, a, skillsPage)
	screenshot(t, tt, "skills-actions")
	// pdf in ~/.claude/skills: Claude Code has it, Codex can connect it.
	if err := tt.Click("pdf"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Connect"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Merge…"); err != nil {
		t.Fatal(err)
	}
	// plan reaches Claude Code and Codex by links: each switches off.
	if err := tt.Click("plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Connected: Codex"); err != nil {
		t.Fatal(err)
	}
	if built != 3 {
		t.Errorf("plans asked for: %d, want 3", built)
	}
}

// A skill synced from claude.ai is tagged, counts as no copy of the
// user's own and offers nothing to change: Claude writes it over.
func TestSyncedSkill(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	dir := home + "/.claude/skills/synced/org_1/morning"
	a.Snapshot.Skills = []inventory.Skill{
		{Name: "morning", Dir: dir, Synced: true, Sum: "a", Paths: []inventory.SkillPath{{Path: dir, Agents: []string{"claude"}}}},
		{Name: "morning", Dir: home + "/.agents/skills/morning", Sum: "a", Paths: []inventory.SkillPath{{Path: home + "/.agents/skills/morning", Agents: []string{"codex"}}}},
	}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	tt := openAI(t, a, skillsPage)
	hasTexts(t, tt, "claude.ai", "Claude syncs this skill from claude.ai and writes it over: change it there.")
	for _, text := range []string{"Connect", "2 copies"} {
		if tt.HasText(text) {
			t.Errorf("unexpected %q: %q", text, tt.Texts())
		}
	}
}

// Each declaration's menu opens and shows its own file, not only the first
// declaration's, the one the buttons under the list open.
func TestMCPOpenFile(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	shell := &fakeShell{}
	a.Shell = shell
	tt := openAI(t, a, mcpPage)
	if err := tt.Click("weftcut"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, item := range []string{"Open config.toml", showInFolder()} {
		if err := tt.Click("More"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	file := home + "/.codex/config.toml"
	if !slices.Equal(shell.opened, []string{file}) || !slices.Equal(shell.revealed, []string{file}) {
		t.Errorf("opened %q, revealed %q", shell.opened, shell.revealed)
	}
}

// A server an agent does not declare offers to connect it; Codex's turns
// off by its switch; a declaration's menu removes it. Each plan is made
// in the background.
func TestMCPOperations(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	built := 0
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		built++
	}
	tt := openAI(t, a, mcpPage)
	screenshot(t, tt, "mcp-actions")
	if err := tt.Click("On: Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove from Claude Code…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("linear"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Connect"); err != nil {
		t.Fatal(err)
	}
	if built != 3 {
		t.Errorf("plans asked for: %d, want 3", built)
	}
}

// The check starts the declaration in use and lists the tools the server
// answered with, the rest on a button; a failure says why and shows what
// the server wrote.
func TestMCPCheck(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	var checked []inventory.MCPServer
	var done func(ai.MCPCheck, error)
	a.CheckMCP = func(s inventory.MCPServer, d func(ai.MCPCheck, error)) {
		checked, done = append(checked, s), d
	}
	tt := openAI(t, a, mcpPage)
	hasTexts(t, tt, "Hearth starts the server, or connects to it, as Claude Code does, and asks for its tools.")
	screenshot(t, tt, "mcp-check-idle")
	if err := tt.Click("Check"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(checked) != 1 || checked[0].Name != "github" || checked[0].Agent != "claude" {
		t.Fatalf("checked %+v, want github of Claude Code", checked)
	}
	hasTexts(t, tt, "Waiting for the server. A first start through npx or uvx downloads it and can take a minute.")
	var tools []ai.MCPTool
	for i := range 10 {
		tools = append(tools, ai.MCPTool{Name: "tool_" + strconv.Itoa(i), Description: "Does " + strconv.Itoa(i) + "."})
	}
	done(ai.MCPCheck{Server: "github-mcp-server", Version: "0.5.0", Tools: tools, Took: 1200 * time.Millisecond}, nil)
	tt.Frame()
	hasTexts(t, tt, "github-mcp-server 0.5.0 answered in 1.20 s · 10 tools", "tool_0", "Does 0.", "Show 2 more")
	if tt.HasText("tool_9") {
		t.Error("shows all 10 tools at once")
	}
	if err := tt.Click("Show 2 more"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "tool_9")
	screenshot(t, tt, "mcp-check-tools")

	if err := tt.Click("Check again"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	done(ai.MCPCheck{}, &ai.MCPError{Kind: ai.MCPExited, Detail: "Error: GITHUB_PERSONAL_ACCESS_TOKEN not set"})
	tt.Frame()
	hasTexts(t, tt, "The server stopped before it answered. What it wrote:", "Error: GITHUB_PERSONAL_ACCESS_TOKEN not set")
	screenshot(t, tt, "mcp-check-failed")
}

// A server whose file writes out a secret is warned of in its details,
// its variable marked, with the button that moves it; the Issues page has
// it, and the agent's settings with a key, each with a fix.
func TestPlainSecrets(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Snapshot.MCP[0].PlainEnv = []string{"GITHUB_TOKEN"}
	a.Snapshot.Agents[0].Settings = home + "/.claude/settings.json"
	a.Snapshot.Agents[0].PlainEnv = []string{"ANTHROPIC_API_KEY"}
	built := 0
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) { built++ }
	tt := openAI(t, a, mcpPage)
	hasTexts(t, tt, "Secret in plain text · Claude Code", "~/.claude.json · GITHUB_TOKEN", "••••••  in plain text", "Move to variables…")
	screenshot(t, tt, "mcp-secret")
	if err := tt.Click("Move to variables…"); err != nil {
		t.Fatal(err)
	}
	if built != 1 {
		t.Errorf("plans asked for: %d, want 1", built)
	}

	tt = openAI(t, a, problemsPage)
	hasTexts(t, tt, "MCP: github has a secret in plain text",
		"~/.claude.json holds GITHUB_TOKEN in plain text. Anyone who reads or copies the file, or the repository it is in, gets the secret. Keep it in an environment variable and let the file refer to it.",
		"Claude Code: a key in plain text in its settings",
		`~/.claude/settings.json sets ANTHROPIC_API_KEY in "env" in plain text. Anyone who reads or copies the file gets the key. Keep it in an environment variable of the terminal: the agent started there gets it just the same.`)
	screenshot(t, tt, "problems-secrets")
	found := issues.Find(a.Snapshot, nil, nil, time.Now())
	for _, is := range found {
		if is.Kind == issues.PlainSecret || is.Kind == issues.SettingsSecret {
			if fix := a.issueFix(is, found); fix != nil {
				fix()
			}
		}
	}
	if built != 3 {
		t.Errorf("plans asked for: %d, want 3", built)
	}
}

// An agent's page lists its rules, the user's then the projects', a risky
// one tagged, and its hooks; the risky rule is worth knowing on the
// Issues page. An agent whose rules Hearth does not read gets no cards.
func TestAgentRulesAndHooks(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	settings := home + "/.claude/settings.json"
	a.Snapshot.Agents[0].Rules = []inventory.AgentRule{
		{Agent: "claude", Kind: inventory.RuleMode, Rule: "defaultMode: bypassPermissions", Risky: true, File: settings},
		{Agent: "claude", Kind: inventory.RuleAllow, Rule: "Bash(npm run *)", File: settings},
		{Agent: "claude", Kind: inventory.RuleDeny, Rule: "Read(./.env)", File: settings},
	}
	a.Snapshot.Agents[0].Hooks = []inventory.AgentHook{
		{Agent: "claude", Event: "PreToolUse", Matcher: "Bash", Command: "~/.claude/hooks/check-bash.sh", File: settings},
	}
	app := home + "/dev/app"
	a.Projects = []projects.Project{{Dir: app,
		Rules: []inventory.AgentRule{{Agent: "claude", Kind: inventory.RuleAsk, Rule: "Bash(git push *)", File: app + "/.claude/settings.json", Project: app}},
		Hooks: []inventory.AgentHook{{Agent: "claude", Event: "SessionStart", Command: "make setup", File: app + "/.claude/settings.json", Project: app}},
	}}
	tt := openAI(t, a, agentPath("claude"))
	hasTexts(t, tt, "Without asking", "4 rules · 2 files",
		"defaultMode: bypassPermissions", "no confirmation", "Bash(npm run *)", "allowed", "Read(./.env)", "denied",
		"Bash(git push *)", "asks", "project · app · ~/dev/app/.claude/settings.json",
		"Hooks", "2 hooks", "~/.claude/hooks/check-bash.sh", "Bash · user · ~/.claude/settings.json", "PreToolUse",
		"make setup", "SessionStart")
	screenshot(t, tt, "agent-rules")

	tt = openAI(t, a, agentPath("codex"))
	hasTexts(t, tt, "No explicit permission rules found. The agent’s defaults and session options still apply.", "No hooks")

	tt = openAI(t, a, problemsPage)
	hasTexts(t, tt, "Claude Code works without confirmation",
		"~/.claude/settings.json sets defaultMode: bypassPermissions: the agent runs commands and edits files without asking. That suits a sandbox, but nothing stops a wrong command.")

	a.Snapshot.Agents = append(a.Snapshot.Agents, inventory.Agent{ID: "aider", Installations: []inventory.Installation{{Tool: "aider", Path: "/usr/local/bin/aider", OnPath: 1}}})
	tt = openAI(t, a, agentPath("aider"))
	if tt.HasText("Without asking") || tt.HasText("Hooks") {
		t.Error("an agent whose rules Hearth does not read has the cards")
	}
}

func TestMCPAddEditAndConflictSelector(t *testing.T) {
	homeDir := t.TempDir()
	cursorFile := filepath.Join(homeDir, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cursorFile, []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","server-v2"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	a := NewApp()
	a.Home = homeDir
	a.Snapshot = aiSnapshot()
	// Add a conflicting declaration for github in Cursor so CheckMCP offers a declaration selector.
	a.Snapshot.MCP = append(a.Snapshot.MCP, inventory.MCPServer{
		Name: "github", Agent: "cursor", File: cursorFile, Transport: "stdio", Command: "npx", Args: []string{"-y", "server-v2"},
	})
	var checked []inventory.MCPServer
	a.CheckMCP = func(s inventory.MCPServer, done func(ai.MCPCheck, error)) {
		checked = append(checked, s)
		done(ai.MCPCheck{Server: s.Agent + "-srv", Took: 100 * time.Millisecond}, nil)
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	tt := openAI(t, a, mcpPage)
	hasTexts(t, tt, "Add server…", "Check", "Claude Code", "Cursor")

	// Select Cursor declaration in Check selector and verify CheckMCP gets Cursor's server.
	if err := tt.Click("Cursor"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Check"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(checked) != 1 || checked[0].Agent != "cursor" {
		t.Fatalf("checked = %+v, want cursor declaration", checked)
	}

	// Open Add server form and propose adding a new HTTP server.
	if err := tt.Click("Add server…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.mcpForm == nil || !a.mcpForm.open {
		t.Fatal("add MCP form did not open")
	}
	a.mcpForm.name = "docs"
	a.mcpForm.agent = slices.Index(mcpCapableAgents, "cursor")
	a.mcpForm.transport = 1
	a.mcpForm.url = "https://mcp.example.com/mcp"
	tt.Frame()
	if err := tt.Click("Review plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Add docs to Cursor" {
		t.Fatalf("add plan = %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Open Edit form via helper (same path as menu item) and verify prefilled values + plan.
	a.openEditMCPForm(a.Snapshot.MCP[len(a.Snapshot.MCP)-1])
	tt.Frame()
	if a.mcpForm == nil || !a.mcpForm.edit || a.mcpForm.command != "npx" {
		t.Fatalf("edit form not prefilled: %+v", a.mcpForm)
	}
	a.mcpForm.command = "uvx"
	tt.Frame()
	if err := tt.Click("Review plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Update github in Cursor" {
		t.Fatalf("edit plan = %+v", a.op)
	}
}
