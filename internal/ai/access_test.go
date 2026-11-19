package ai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// writeFiles writes files, by path relative to dir, in dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Each agent's user files give its modes, rules and hooks as the file
// writes them, risky those doing anything unasked.
func TestAccess(t *testing.T) {
	const (
		mode  = inventory.RuleMode
		allow = inventory.RuleAllow
		ask   = inventory.RuleAsk
		deny  = inventory.RuleDeny
	)
	type rule struct {
		kind  inventory.RuleKind
		rule  string
		risky bool
		file  string
	}
	type hook struct{ event, matcher, command, file string }
	tests := []struct {
		agent string
		files map[string]string
		rules []rule
		hooks []hook
	}{
		{"claude", map[string]string{"settings.json": `{
			"permissions": {"defaultMode": "bypassPermissions", "allow": ["Bash(npm run *)", "Bash"], "ask": ["Bash(git push *)"], "deny": ["Read(./.env)"]},
			"hooks": {
				"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "~/.claude/hooks/check.sh"}]}],
				"Stop": [{"hooks": [{"type": "http", "url": "http://localhost:8080/stop"}, {"type": "mcp_tool", "server": "memory", "tool": "save"}]}],
				"UserPromptSubmit": [{"hooks": [{"type": "prompt", "prompt": "Is this safe?\nAnswer yes or no."}]}]
			}}`},
			[]rule{
				{mode, "defaultMode: bypassPermissions", true, "settings.json"},
				{allow, "Bash(npm run *)", false, "settings.json"}, {allow, "Bash", true, "settings.json"},
				{ask, "Bash(git push *)", false, "settings.json"}, {deny, "Read(./.env)", false, "settings.json"},
			},
			[]hook{
				{"PreToolUse", "Bash", "~/.claude/hooks/check.sh", "settings.json"},
				{"Stop", "", "http://localhost:8080/stop", "settings.json"}, {"Stop", "", "memory save", "settings.json"},
				{"UserPromptSubmit", "", "Is this safe?", "settings.json"},
			}},
		// The default mode is no rule.
		{"claude", map[string]string{"settings.json": `{"permissions": {"defaultMode": "default"}}`}, nil, nil},
		{"codex", map[string]string{
			"config.toml": `approval_policy = "never"
sandbox_mode = "danger-full-access"

[[hooks.PreToolUse]]
matcher = "^Bash$"

[[hooks.PreToolUse.hooks]]
type = "command"
command = "python3 ~/.codex/hooks/check.py"
`,
			"hooks.json": `{"hooks": {"SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "echo hi"}]}]}}`,
			"rules/default.rules": `# allowed from the TUI
prefix_rule(pattern = ["gh", "pr", "view"], match = ["gh pr view 1"])
prefix_rule(
    pattern = ["git", ["push", "reset"]],  # dangerous ones
    decision = "prompt",
    justification = "Ask (first)",
)
prefix_rule(pattern=["rm", "-rf"], decision="forbidden")
`,
		},
			[]rule{
				{mode, "approval_policy: never", true, "config.toml"}, {mode, "sandbox_mode: danger-full-access", false, "config.toml"},
				{allow, "gh pr view", false, "rules/default.rules"}, {ask, "git push|reset", false, "rules/default.rules"},
				{deny, "rm -rf", false, "rules/default.rules"},
			},
			[]hook{
				{"PreToolUse", "^Bash$", "python3 ~/.codex/hooks/check.py", "config.toml"},
				{"SessionStart", "startup", "echo hi", "hooks.json"},
			}},
		// A granular policy is not a word; never in a sandbox is safe.
		{"codex", map[string]string{"config.toml": "approval_policy = { granular = { rules = true } }\n"},
			[]rule{{mode, "approval_policy: granular", false, "config.toml"}}, nil},
		{"codex", map[string]string{"config.toml": "approval_policy = \"never\"\nsandbox_mode = \"workspace-write\"\n"},
			[]rule{{mode, "approval_policy: never", false, "config.toml"}, {mode, "sandbox_mode: workspace-write", false, "config.toml"}}, nil},
		{"gemini", map[string]string{
			"settings.json": `{
				"general": {"defaultApprovalMode": "auto_edit"},
				"tools": {"allowed": ["run_shell_command(git)", "run_shell_command"], "exclude": ["write_file"]},
				"hooks": {"BeforeTool": [{"matcher": "write_file|replace", "hooks": [{"name": "check", "type": "command", "command": "check.sh", "timeout": 5000}]}]},
				"hooksConfig": {"enabled": true}}`,
			"policies/user.toml": `[[rule]]
toolName = "run_shell_command"
commandPrefix = ["npm test", "npm run lint"]
decision = "allow"

[[rule]]
toolName = "run_shell_command"
decision = "allow"

[[rule]]
mcpName = "github"
toolName = "create_issue"
decision = "ask_user"

[[rule]]
toolName = ["write_file", "replace"]
argsPattern = "\\.env"
decision = "deny"
`,
		},
			[]rule{
				{mode, "defaultApprovalMode: auto_edit", false, "settings.json"},
				{allow, "run_shell_command(git)", false, "settings.json"}, {allow, "run_shell_command", true, "settings.json"},
				{deny, "write_file", false, "settings.json"},
				{allow, "run_shell_command(npm test|npm run lint)", false, "policies/user.toml"},
				{allow, "run_shell_command", true, "policies/user.toml"},
				{ask, "github__create_issue", false, "policies/user.toml"},
				{deny, `write_file|replace(/\.env/)`, false, "policies/user.toml"},
			},
			[]hook{{"BeforeTool", "write_file|replace", "check.sh", "settings.json"}}},
		{"cursor", map[string]string{
			"cli-config.json": `{"version": 1, "approvalMode": "unrestricted", "permissions": {"allow": ["Shell(ls)", "Shell(*)"], "deny": ["Read(.env*)"]}}`,
			"hooks.json":      `{"version": 1, "hooks": {"beforeShellExecution": [{"command": "./approve.sh", "matcher": "curl|wget"}], "stop": [{"type": "prompt", "prompt": "Done?"}]}}`,
		},
			[]rule{
				{mode, "approvalMode: unrestricted", true, "cli-config.json"},
				{allow, "Shell(ls)", false, "cli-config.json"}, {allow, "Shell(*)", true, "cli-config.json"},
				{deny, "Read(.env*)", false, "cli-config.json"},
			},
			[]hook{{"beforeShellExecution", "curl|wget", "./approve.sh", "hooks.json"}, {"stop", "", "Done?", "hooks.json"}}},
		{"cursor", map[string]string{"cli-config.json": `{"approvalMode": "allowlist", "permissions": {"allow": [], "deny": []}}`}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			rules, hooks, errs := access(tt.agent, dir, "", accessFiles[tt.agent].user, "")
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			var want []inventory.AgentRule
			for _, r := range tt.rules {
				want = append(want, inventory.AgentRule{Agent: tt.agent, Kind: r.kind, Rule: r.rule, Risky: r.risky, File: filepath.Join(dir, filepath.FromSlash(r.file))})
			}
			if !reflect.DeepEqual(rules, want) {
				t.Errorf("rules:\n got %+v\nwant %+v", rules, want)
			}
			var wantHooks []inventory.AgentHook
			for _, h := range tt.hooks {
				wantHooks = append(wantHooks, inventory.AgentHook{Agent: tt.agent, Event: h.event, Matcher: h.matcher, Command: h.command, File: filepath.Join(dir, filepath.FromSlash(h.file))})
			}
			if !reflect.DeepEqual(hooks, wantHooks) {
				t.Errorf("hooks:\n got %+v\nwant %+v", hooks, wantHooks)
			}
		})
	}
}

// A project's files give each agent's rules and hooks with the project
// set; a file that does not read is an error and the others still count.
func TestProjectAccess(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".claude/settings.json":       `{"permissions": {"allow": ["Bash(make *)"]}}`,
		".claude/settings.local.json": `{"permissions": {"allow": ["Bash(git *)"]}, "hooks": {"PostToolUse": [{"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "make fmt"}]}]}}`,
		".codex/config.toml":          "approval_policy = \"on-request\"\n",
		".codex/rules/team.rules":     `prefix_rule(pattern = ["make"])`,
		".gemini/settings.json":       `{not json`,
		".cursor/cli.json":            `{"permissions": {"allow": ["Shell(make)"], "deny": []}}`,
		".cursor/hooks.json":          `{"version": 1, "hooks": {"afterFileEdit": [{"command": ".cursor/hooks/fmt.sh"}]}}`,
		// Gemini CLI reads no project's policies.
		".gemini/policies/p.toml": "[[rule]]\ntoolName = \"*\"\ndecision = \"allow\"\n",
	})
	p := func(rel string) string { return filepath.Join(dir, filepath.FromSlash(rel)) }
	rules, hooks, errs := ProjectAccess(dir)
	want := []inventory.AgentRule{
		{Agent: "claude", Kind: inventory.RuleAllow, Rule: "Bash(make *)", File: p(".claude/settings.json"), Project: dir},
		{Agent: "claude", Kind: inventory.RuleAllow, Rule: "Bash(git *)", File: p(".claude/settings.local.json"), Project: dir},
		{Agent: "codex", Kind: inventory.RuleMode, Rule: "approval_policy: on-request", File: p(".codex/config.toml"), Project: dir},
		{Agent: "codex", Kind: inventory.RuleAllow, Rule: "make", File: p(".codex/rules/team.rules"), Project: dir},
		{Agent: "cursor", Kind: inventory.RuleAllow, Rule: "Shell(make)", File: p(".cursor/cli.json"), Project: dir},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("rules:\n got %+v\nwant %+v", rules, want)
	}
	wantHooks := []inventory.AgentHook{
		{Agent: "claude", Event: "PostToolUse", Matcher: "Edit|Write", Command: "make fmt", File: p(".claude/settings.local.json"), Project: dir},
		{Agent: "cursor", Event: "afterFileEdit", Command: ".cursor/hooks/fmt.sh", File: p(".cursor/hooks.json"), Project: dir},
	}
	if !reflect.DeepEqual(hooks, wantHooks) {
		t.Errorf("hooks:\n got %+v\nwant %+v", hooks, wantHooks)
	}
	if len(errs) != 1 {
		t.Errorf("errs = %v, want the one of .gemini/settings.json", errs)
	}
}
