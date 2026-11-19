package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/inventory"
)

// accessFiles are where the agents keep what they may do without asking
// and their hooks: the user's, relative to the agent's settings
// directory, and a project's, relative to it. A directory holds rule
// files.
var accessFiles = map[string]struct{ user, project []string }{
	"claude": {[]string{"settings.json"}, []string{".claude/settings.json", ".claude/settings.local.json"}},
	"codex":  {[]string{"config.toml", "hooks.json", "rules"}, []string{".codex/config.toml", ".codex/hooks.json", ".codex/rules"}},
	// Gemini CLI does not read a project's policies.
	"gemini": {[]string{"settings.json", "policies"}, []string{".gemini/settings.json"}},
	"cursor": {[]string{"cli-config.json", "hooks.json"}, []string{".cursor/cli.json", ".cursor/hooks.json"}},
	"antigravity": {[]string{"settings.json", "~/.gemini/config/hooks.json"},
		[]string{".agents/hooks.json", ".agent/hooks.json", "_agents/hooks.json", "_agent/hooks.json"}},
}

// AccessKnown reports that Hearth reads the permissions and hooks of the
// agent id.
func AccessKnown(id string) bool {
	_, ok := accessFiles[id]
	return ok
}

// ProjectAccess are the rules and hooks of the agents' files in the
// project at dir, in the agents' order; errs say which files could not be
// read.
func ProjectAccess(dir string) (rules []inventory.AgentRule, hooks []inventory.AgentHook, errs []error) {
	for _, a := range agents {
		r, h, e := access(a.id, dir, "", accessFiles[a.id].project, dir)
		rules, hooks, errs = append(rules, r...), append(hooks, h...), append(errs, e...)
	}
	return rules, hooks, errs
}

// access reads the agent's rules and hooks of files in dir, set for
// project.
func access(agent, dir, home string, files []string, project string) (rules []inventory.AgentRule, hooks []inventory.AgentHook, errs []error) {
	for _, f := range files {
		path := resolve(f, dir, home)
		if path == "" {
			continue
		}
		paths := []string{path}
		if info, err := os.Stat(path); err != nil {
			continue
		} else if info.IsDir() {
			// A directory's rule files, by name.
			paths, _ = filepath.Glob(filepath.Join(path, "*"+map[string]string{"codex": ".rules", "gemini": ".toml"}[agent]))
		}
		for _, p := range paths {
			r, h, err := readAccess(agent, p)
			if err != nil {
				errs = append(errs, fmt.Errorf("read %s: %w", p, err))
			}
			for i := range r {
				r[i].Agent, r[i].File, r[i].Project = agent, p, project
			}
			for i := range h {
				h[i].Agent, h[i].File, h[i].Project = agent, p, project
			}
			rules, hooks = append(rules, r...), append(hooks, h...)
		}
	}
	return rules, hooks, errs
}

// readAccess reads the rules and hooks of the agent's file at path.
func readAccess(agent, path string) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	name := filepath.Base(path)
	switch {
	case agent == "claude":
		return claudeAccess(data)
	case agent == "codex" && name == "hooks.json", agent == "cursor" && name == "hooks.json":
		var cfg struct {
			Hooks map[string]json.RawMessage `json:"hooks"`
		}
		err := json.Unmarshal(data, &cfg)
		return nil, jsonHooks(cfg.Hooks), err
	case agent == "codex" && strings.HasSuffix(name, ".rules"):
		return codexRules(string(data)), nil, nil
	case agent == "codex":
		return codexAccess(data)
	case agent == "gemini" && strings.HasSuffix(name, ".toml"):
		return geminiPolicy(data)
	case agent == "gemini":
		return geminiAccess(data)
	case agent == "antigravity" && name == "hooks.json":
		hooks, err := antigravityHooks(data)
		return nil, hooks, err
	case agent == "antigravity":
		rules, err := antigravityAccess(data)
		return rules, nil, err
	case agent == "cursor":
		return cursorAccess(data)
	}
	return nil, nil, nil
}

func antigravityAccess(data []byte) ([]inventory.AgentRule, error) {
	var cfg struct {
		AgentMode      string `json:"agentMode"`
		ToolPermission string `json:"toolPermission"`
		Permissions    struct {
			Allow []string `json:"allow"`
			Ask   []string `json:"ask"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	var rules []inventory.AgentRule
	if cfg.AgentMode != "" {
		rules = append(rules, mode("agentMode", cfg.AgentMode, false))
	}
	if cfg.ToolPermission != "" {
		rules = append(rules, mode("toolPermission", cfg.ToolPermission, cfg.ToolPermission == "always-proceed"))
	}
	p := cfg.Permissions
	rules = append(rules, listRules(inventory.RuleAllow, p.Allow, "command(*)")...)
	rules = append(rules, listRules(inventory.RuleAsk, p.Ask)...)
	return append(rules, listRules(inventory.RuleDeny, p.Deny)...), nil
}

func antigravityHooks(data []byte) ([]inventory.AgentHook, error) {
	var definitions map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &definitions); err != nil {
		return nil, err
	}
	var hooks []inventory.AgentHook
	for _, name := range slices.Sorted(maps.Keys(definitions)) {
		events := definitions[name]
		var enabled bool
		if raw, ok := events["enabled"]; ok && json.Unmarshal(raw, &enabled) == nil && !enabled {
			continue
		}
		hooks = append(hooks, jsonHooks(events)...)
	}
	return hooks, nil
}

// mode is the rule of a mode setting key set to value.
func mode(key, value string, risky bool) inventory.AgentRule {
	return inventory.AgentRule{Kind: inventory.RuleMode, Rule: key + ": " + value, Risky: risky}
}

// listRules are rules of one kind, risky those allowing any command:
// the rules anything lists.
func listRules(kind inventory.RuleKind, rules []string, anything ...string) []inventory.AgentRule {
	var out []inventory.AgentRule
	for _, r := range rules {
		out = append(out, inventory.AgentRule{Kind: kind, Rule: r, Risky: kind == inventory.RuleAllow && slices.Contains(anything, r)})
	}
	return out
}

// hookGroup is hooks of an event as Claude Code, Codex and Gemini CLI
// write them: a matcher and handlers; Cursor's are handlers alone, the
// matcher in each.
type hookGroup struct {
	Matcher string        `json:"matcher" toml:"matcher"`
	Hooks   []hookHandler `json:"hooks" toml:"hooks"`
	hookHandler
}

// hookHandler is what a hook runs: a command, or, by its type, a URL,
// an MCP server's tool or a prompt.
type hookHandler struct {
	Command string `json:"command" toml:"command"`
	URL     string `json:"url" toml:"url"`
	Server  string `json:"server" toml:"server"`
	Tool    string `json:"tool" toml:"tool"`
	Prompt  string `json:"prompt" toml:"prompt"`
}

func (h hookHandler) runs() string {
	switch {
	case h.Command != "":
		return h.Command
	case h.URL != "":
		return h.URL
	case h.Server != "":
		return strings.TrimSpace(h.Server + " " + h.Tool)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(h.Prompt), "\n")
	return line
}

// hooksOf are the hooks of groups by event, events by name.
func hooksOf(events map[string][]hookGroup) []inventory.AgentHook {
	var out []inventory.AgentHook
	for _, event := range slices.Sorted(maps.Keys(events)) {
		for _, g := range events[event] {
			handlers := g.Hooks
			if len(handlers) == 0 {
				handlers = []hookHandler{g.hookHandler}
			}
			for _, h := range handlers {
				if run := h.runs(); run != "" {
					out = append(out, inventory.AgentHook{Event: event, Matcher: g.Matcher, Command: run})
				}
			}
		}
	}
	return out
}

// jsonHooks are the hooks of a JSON "hooks" object; an event that is not
// a list of hooks, as a switch of an older version, is skipped.
func jsonHooks(raw map[string]json.RawMessage) []inventory.AgentHook {
	events := map[string][]hookGroup{}
	for event, data := range raw {
		var gs []hookGroup
		if json.Unmarshal(data, &gs) == nil {
			events[event] = gs
		}
	}
	return hooksOf(events)
}

// claudeAccess reads Claude Code's settings.json: permissions and hooks.
func claudeAccess(data []byte) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	var cfg struct {
		Permissions struct {
			Allow       []string `json:"allow"`
			Ask         []string `json:"ask"`
			Deny        []string `json:"deny"`
			DefaultMode string   `json:"defaultMode"`
		} `json:"permissions"`
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, err
	}
	p := cfg.Permissions
	var rules []inventory.AgentRule
	if m := p.DefaultMode; m != "" && m != "default" && m != "manual" {
		rules = append(rules, mode("defaultMode", m, m == "bypassPermissions"))
	}
	rules = append(rules, listRules(inventory.RuleAllow, p.Allow, "Bash", "Bash(*)")...)
	rules = append(rules, listRules(inventory.RuleAsk, p.Ask)...)
	rules = append(rules, listRules(inventory.RuleDeny, p.Deny)...)
	return rules, jsonHooks(cfg.Hooks), nil
}

// codexAccess reads Codex's config.toml: its approval policy, sandbox
// and hooks. Approving nothing in no sandbox does everything unasked.
func codexAccess(data []byte) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	var cfg struct {
		// ApprovalPolicy is a word, or a table of granular choices.
		ApprovalPolicy any                    `toml:"approval_policy"`
		SandboxMode    string                 `toml:"sandbox_mode"`
		Hooks          map[string][]hookGroup `toml:"hooks"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, nil, err
	}
	approval, ok := cfg.ApprovalPolicy.(string)
	if !ok && cfg.ApprovalPolicy != nil {
		approval = "granular"
	}
	var rules []inventory.AgentRule
	if approval != "" {
		rules = append(rules, mode("approval_policy", approval, approval == "never" && cfg.SandboxMode == "danger-full-access"))
	}
	if cfg.SandboxMode != "" {
		rules = append(rules, mode("sandbox_mode", cfg.SandboxMode, false))
	}
	return rules, hooksOf(cfg.Hooks), nil
}

// prefixRule starts a rule of Codex's rules files, which are Starlark.
var prefixRule = regexp.MustCompile(`\bprefix_rule\s*\(`)

// starlarkArg finds an argument's value in a call's arguments.
var (
	patternArg  = regexp.MustCompile(`\bpattern\s*=\s*\[`)
	decisionArg = regexp.MustCompile(`\bdecision\s*=\s*["'](\w+)["']`)
)

// codexRules are the prefix_rule calls of a rules file: its command
// prefix, alternatives as "a|b", and decision.
func codexRules(src string) []inventory.AgentRule {
	var out []inventory.AgentRule
	for _, m := range prefixRule.FindAllStringIndex(src, -1) {
		args := starlarkUntil(src[m[1]:], 1, ')')
		p := patternArg.FindStringIndex(args)
		if p == nil {
			continue
		}
		kind := inventory.RuleAllow
		if d := decisionArg.FindStringSubmatch(args); d != nil {
			switch d[1] {
			case "prompt":
				kind = inventory.RuleAsk
			case "forbidden":
				kind = inventory.RuleDeny
			}
		}
		out = append(out, inventory.AgentRule{Kind: kind, Rule: starlarkPattern(starlarkUntil(args[p[1]:], 1, ']'))})
	}
	return out
}

// starlarkUntil is src up to where the bracket open depth deep closes
// with end, strings and comments skipped.
func starlarkUntil(src string, depth int, end byte) string {
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '(', '[':
			depth++
		case ')', ']':
			if depth--; depth == 0 && c == end {
				return src[:i]
			}
		case '"', '\'':
			j := strings.IndexByte(src[i+1:], c)
			if j < 0 {
				return src
			}
			i += j + 1
		case '#':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				return src
			}
			i += j
		}
	}
	return src
}

// starlarkPattern writes a pattern's list: its words, a list in it as
// alternatives "a|b".
func starlarkPattern(list string) string {
	var words, alts []string
	inner := false
	for i := 0; i < len(list); i++ {
		switch c := list[i]; c {
		case '[':
			inner, alts = true, nil
		case ']':
			inner = false
			words = append(words, strings.Join(alts, "|"))
		case '"', '\'':
			j := strings.IndexByte(list[i+1:], c)
			if j < 0 {
				return strings.Join(words, " ")
			}
			if lit := list[i+1 : i+1+j]; inner {
				alts = append(alts, lit)
			} else {
				words = append(words, lit)
			}
			i += j + 1
		}
	}
	return strings.Join(words, " ")
}

// geminiShell are Gemini CLI's names of its shell tool: allowed without
// more, it runs any command.
var geminiShell = []string{"run_shell_command", "ShellTool"}

// geminiAccess reads Gemini CLI's settings.json: its approval mode, the
// tools it may use unasked and those taken away, and hooks.
func geminiAccess(data []byte) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	var cfg struct {
		General struct {
			DefaultApprovalMode string `json:"defaultApprovalMode"`
		} `json:"general"`
		Tools struct {
			Allowed []string `json:"allowed"`
			Exclude []string `json:"exclude"`
		} `json:"tools"`
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, err
	}
	var rules []inventory.AgentRule
	if m := cfg.General.DefaultApprovalMode; m != "" && m != "default" {
		rules = append(rules, mode("defaultApprovalMode", m, false))
	}
	rules = append(rules, listRules(inventory.RuleAllow, cfg.Tools.Allowed, geminiShell...)...)
	rules = append(rules, listRules(inventory.RuleDeny, cfg.Tools.Exclude)...)
	return rules, jsonHooks(cfg.Hooks), nil
}

// geminiPolicy reads a policy file of Gemini CLI: each [[rule]] as its
// tool, with the command prefix or pattern it is for.
func geminiPolicy(data []byte) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	var cfg struct {
		Rule []struct {
			ToolName      any    `toml:"toolName"`
			MCPName       string `toml:"mcpName"`
			CommandPrefix any    `toml:"commandPrefix"`
			CommandRegex  string `toml:"commandRegex"`
			ArgsPattern   string `toml:"argsPattern"`
			Decision      string `toml:"decision"`
		} `toml:"rule"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, nil, err
	}
	var rules []inventory.AgentRule
	for _, r := range cfg.Rule {
		kind := inventory.RuleAllow
		switch r.Decision {
		case "ask_user":
			kind = inventory.RuleAsk
		case "deny":
			kind = inventory.RuleDeny
		}
		tools := words(r.ToolName)
		if len(tools) == 0 {
			tools = []string{"*"}
		}
		rule := strings.Join(tools, "|")
		if r.MCPName != "" {
			rule = r.MCPName + "__" + rule
		}
		prefixes := words(r.CommandPrefix)
		var narrow string
		switch {
		case len(prefixes) > 0:
			narrow = strings.Join(prefixes, "|")
		case r.CommandRegex != "":
			narrow = "/" + r.CommandRegex + "/"
		case r.ArgsPattern != "":
			narrow = "/" + r.ArgsPattern + "/"
		}
		risky := false
		if narrow != "" {
			rule += "(" + narrow + ")"
		} else if kind == inventory.RuleAllow && r.MCPName == "" {
			risky = slices.ContainsFunc(tools, func(t string) bool { return t == "*" || slices.Contains(geminiShell, t) })
		}
		rules = append(rules, inventory.AgentRule{Kind: kind, Rule: rule, Risky: risky})
	}
	return rules, nil, nil
}

// words is a TOML value that is a string or a list of them.
func words(v any) []string {
	if s, ok := v.(string); ok {
		return []string{s}
	}
	return stringList(v)
}

// cursorAccess reads Cursor's cli-config.json or a project's cli.json:
// its approval mode and permissions.
func cursorAccess(data []byte) ([]inventory.AgentRule, []inventory.AgentHook, error) {
	var cfg struct {
		ApprovalMode string `json:"approvalMode"`
		Permissions  struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, err
	}
	var rules []inventory.AgentRule
	if m := cfg.ApprovalMode; m != "" && m != "allowlist" {
		rules = append(rules, mode("approvalMode", m, m == "unrestricted"))
	}
	rules = append(rules, listRules(inventory.RuleAllow, cfg.Permissions.Allow, "Shell(*)")...)
	rules = append(rules, listRules(inventory.RuleDeny, cfg.Permissions.Deny)...)
	return rules, nil, nil
}
