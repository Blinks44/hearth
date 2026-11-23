package ai

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const hidden = inventory.Hidden

// The user's servers in every agent's format, in the agents' order; with
// no command on PATH, every stdio server by name is missing. The fixture
// writes its secrets as s3cr3t.
func TestMCP(t *testing.T) {
	home := fixtureHome(t)
	p := func(rel ...string) string { return filepath.Join(append([]string{home}, rel...)...) }
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + t.TempDir()}
	got, errs := MCP(env)
	if len(errs) > 0 {
		t.Errorf("errs: %q", errs)
	}
	claude, codex := p(".claude.json"), p(".codex", "config.toml")
	want := []inventory.MCPServer{
		{Name: "github", Agent: "claude", File: claude, Transport: "stdio", Command: "docker",
			Args: []string{"run", "-i", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "ghcr.io/github/github-mcp-server"},
			Env:  []string{"GITHUB_PERSONAL_ACCESS_TOKEN"}, Missing: true, PlainEnv: []string{"GITHUB_PERSONAL_ACCESS_TOKEN"}},
		{Name: "linear", Agent: "claude", File: claude, Transport: "http",
			URL: "https://mcp.linear.app/mcp?api_key=" + hidden, Headers: []string{"Authorization"},
			PlainHeaders: []string{"Authorization"}, PlainArgs: true},
		{Name: "postgres", Agent: "claude", File: claude, Project: "/work/app", Transport: "stdio", Command: "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-postgres", "postgresql://" + hidden + "@localhost/db"}, Missing: true, PlainArgs: true},
		{Name: "context7", Agent: "codex", File: codex, Transport: "stdio", Command: "npx",
			Args: []string{"-y", "@upstash/context7-mcp", "--api-key", hidden},
			Env:  []string{"CONTEXT7_HOME", "CONTEXT7_TOKEN"}, Missing: true, PlainEnv: []string{"CONTEXT7_TOKEN"}, PlainArgs: true},
		{Name: "sentry", Agent: "codex", File: codex, Transport: "http", URL: "https://mcp.sentry.dev/mcp",
			Headers: []string{"Authorization"}, Disabled: true},
		{Name: "playwright", Agent: "copilot", File: p(".copilot", "mcp-config.json"), Transport: "stdio", Command: "npx",
			Args: []string{"@playwright/mcp@latest"}, Missing: true},
		{Name: "remote", Agent: "gemini", File: p(".gemini", "settings.json"), Transport: "http",
			URL: "https://" + hidden + "@example.com/mcp", Headers: []string{"X-Api-Key"},
			PlainHeaders: []string{"X-Api-Key"}, PlainArgs: true},
		{Name: "filesystem", Agent: "cursor", File: p(".cursor", "mcp.json"), Transport: "stdio", Command: "uvx",
			Args: []string{"mcp-fs", "--token=" + hidden, "Authorization: " + hidden, "/work"}, Missing: true, PlainArgs: true},
		{Name: "local", Agent: "opencode", File: p(".config", "opencode", "opencode.json"), Transport: "stdio", Command: "bun",
			Args: []string{"x", "my-mcp"}, Env: []string{"MY_KEY"}, Missing: true, PlainEnv: []string{"MY_KEY"}},
		{Name: "remote", Agent: "opencode", File: p(".config", "opencode", "opencode.json"), Transport: "http",
			URL: "https://mcp.example.com/sse?token=" + hidden + "&team=" + hidden, PlainArgs: true},
		{Name: "docs", Agent: "devin", File: p(".config", "devin", "mcp_config.json"), Transport: "http",
			URL: "https://docs.example.com/sse", Disabled: true},
		{Name: "everything", Agent: "grok", File: p(".grok", "config.toml"), Transport: "stdio", Command: "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-everything"}, Missing: true},
		{Name: "search", Agent: "kimi", File: p(".kimi-code", "mcp.json"), Transport: "http", URL: "https://search.example.com/mcp"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d servers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("got  %+v\nwant %+v", got[i], want[i])
		}
	}
}

// A project's servers: .mcp.json counts for Claude Code and Copilot alike;
// a command by path is checked against the project, one by name is not;
// a file that does not read is an error, the others still count.
func TestProjectMCP(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}
	got, errs := ProjectMCP(dir)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), filepath.Join(".cursor", "mcp.json")) {
		t.Errorf("errs: %v", errs)
	}
	type server struct{ agent, name, command string }
	var names []server
	for _, s := range got {
		names = append(names, server{s.Agent, s.Name, s.Command})
		if s.Project != dir {
			t.Errorf("%s: project %q", s.Name, s.Project)
		}
		if s.Name == "db" && (s.Missing || s.CommandPath != "") {
			t.Errorf("db: a command by name is checked: %+v", s)
		}
		if s.Name == "local-tool" && (!s.Missing || !reflect.DeepEqual(s.Args, []string{"--secret", hidden})) {
			t.Errorf("local-tool: %+v", s)
		}
	}
	want := []server{
		{"claude", "db", "npx"}, {"claude", "local-tool", "./bin/missing-server"},
		{"codex", "db", "npx"},
		{"copilot", "db", "npx"}, {"copilot", "local-tool", "./bin/missing-server"},
		{"opencode", "db", "npx"},
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got  %v\nwant %v", names, want)
	}
}

// No secret of the fixtures reaches the snapshot, nor the cache it is
// saved to.
func TestMCPKeepsNoSecrets(t *testing.T) {
	home := fixtureHome(t)
	user, _ := MCP(sysenv.Env{"HOME=" + home, "USERPROFILE=" + home})
	project, _ := ProjectMCP(filepath.Join("testdata", "project"))
	cache := t.TempDir()
	if err := store.SaveSnapshot(cache, inventory.Snapshot{MCP: append(user, project...)}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) == 0 {
		t.Fatalf("cache: %v, %v", entries, err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(cache, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "s3cr3t") || !strings.Contains(string(data), "GITHUB_PERSONAL_ACCESS_TOKEN") {
			t.Errorf("%s keeps a secret, or not the names:\n%s", e.Name(), data)
		}
	}
}

func TestMaskArg(t *testing.T) {
	tests := []struct{ prev, in, out string }{
		{"", "--token=abc", "--token=" + hidden},
		{"--api-key", "abc", hidden},
		{"--api-key", "--verbose", "--verbose"},
		{"", "GITHUB_TOKEN=abc", "GITHUB_TOKEN=" + hidden},
		{"-e", "GITHUB_TOKEN", "GITHUB_TOKEN"},
		{"--header", "Authorization: Bearer abc", "Authorization: " + hidden},
		{"", "ghp_abcdef", hidden},
		{"", "https://x.dev/sse?key=abc", "https://x.dev/sse?key=" + hidden},
		{"", "postgres://u:p@db/x", "postgres://" + hidden + "@db/x"},
		{"", "@scope/pkg@latest", "@scope/pkg@latest"},
		{"--port", "8080", "8080"},
	}
	for _, tt := range tests {
		if got := maskArg(tt.in, tt.prev); got != tt.out {
			t.Errorf("%q after %q: %q, want %q", tt.in, tt.prev, got, tt.out)
		}
	}
}

func TestMCPClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "custom claude")
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(config, ".claude.json")
	if err := os.WriteFile(state, []byte(`{"mcpServers":{"hearth":{"type":"stdio","command":"hearth","args":["mcp"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "CLAUDE_CONFIG_DIR=" + config}
	if got := MCPFile("claude", env, ""); got != state {
		t.Fatalf("path %s", got)
	}
	servers, errs := MCP(env)
	if len(errs) != 0 || len(servers) != 1 || servers[0].File != state {
		t.Fatalf("servers %+v errors %v", servers, errs)
	}
}

func TestProjectMCPWithTermEnv(t *testing.T) {
	dir := t.TempDir()
	binDir := t.TempDir()
	cmdName := "fake-mcp-runner"
	cmdPath := filepath.Join(binDir, cmdName)
	if runtime.GOOS == "windows" {
		cmdPath += ".cmd"
	}
	if err := os.WriteFile(cmdPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{
  "mcpServers": {
    "found-srv": {"command": "`+cmdName+`"},
    "missing-srv": {"command": "no-such-mcp-binary-xyz"}
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"PATH=" + binDir}
	got, errs := ProjectMCP(dir, env)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	var sawFound, sawMissing bool
	for _, s := range got {
		if s.Agent != "claude" {
			continue
		}
		switch s.Name {
		case "found-srv":
			sawFound = true
			if s.Missing || s.CommandPath == "" {
				t.Errorf("found-srv: Missing=%v CommandPath=%q, want resolved", s.Missing, s.CommandPath)
			}
		case "missing-srv":
			sawMissing = true
			if !s.Missing {
				t.Errorf("missing-srv: Missing=false, want true")
			}
		}
	}
	if !sawFound || !sawMissing {
		t.Fatalf("expected both servers in %+v", got)
	}
}
