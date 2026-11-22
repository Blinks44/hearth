package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const token = "ghp_secret123456"

// mcpHome is a home where Claude Code declares github, with a token, and
// Codex has a config of its own already.
func mcpHome(t *testing.T) (string, sysenv.Env) {
	t.Helper()
	home := t.TempDir()
	write := func(path, text string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude.json"), `{
  "numStartups": 3,
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {"GITHUB_TOKEN": "`+token+`"}
    }
  },
  "projects": {"/dev/app": {"mcpServers": {"local": {"command": "x"}}}}
}
`)
	write(filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-5\"\n\n[mcp_servers.passed]\ncommand = \"y\"\nenv_vars = [\"TOKEN\"]\n")
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH")}
	return home, env
}

func server(t *testing.T, env sysenv.Env, agent, name string) inventory.MCPServer {
	t.Helper()
	servers, _ := ai.MCP(env)
	s, ok := findServer(servers, agent, name, "")
	if !ok {
		i := slices.IndexFunc(servers, func(s inventory.MCPServer) bool { return s.Agent == agent && s.Name == name })
		if i < 0 {
			t.Fatalf("no %s of %s in %+v", name, agent, servers)
		}
		s = servers[i]
	}
	return s
}

func runEdits(t *testing.T, p Plan) {
	t.Helper()
	if s := Execute(context.Background(), &run.Fake{}, p, io.Discard, func(State) {}); s.Phase != Succeeded {
		t.Fatalf("%s: phase %v, %q", p.Title, s.Phase, s.Err)
	}
}

func TestMCPConnect(t *testing.T) {
	home, env := mcpHome(t)
	github := server(t, env, "claude", "github")

	// To Cursor: a file that is not there yet, the token in it, hidden.
	p, err := MCPConnectPlan(github, "cursor", env, title)
	if err != nil {
		t.Fatal(err)
	}
	e := p.Steps[0].Edit
	if e == nil || !e.New || !strings.Contains(e.After, token) || strings.Contains(e.Masked(e.After), token) {
		t.Fatalf("edit %+v", e)
	}
	runEdits(t, p)
	if _, err := MCPConnectPlan(github, "cursor", env, title); err == nil {
		t.Error("a plan for a server the agent has")
	}

	// To Codex: a table after the user's.
	p, err = MCPConnectPlan(github, "codex", env, title)
	if err != nil {
		t.Fatal(err)
	}
	after := p.Steps[0].Edit.After
	if !strings.HasPrefix(after, "model = \"gpt-5\"\n") || !strings.Contains(after, "[mcp_servers.github]") {
		t.Errorf("codex config:\n%s", after)
	}
	if _, err := toml.Decode(after, new(map[string]any)); err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)

	// To Claude Code: through claude mcp, the token hidden in what shows.
	codexGithub := server(t, env, "codex", "github")
	cursor := server(t, env, "cursor", "github")
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err = MCPConnectPlan(cursor, "claude", env, title)
	if err != nil {
		t.Fatal(err)
	}
	if argv := p.Steps[0].Argv; argv[2] != "add-json" || !strings.Contains(argv[6], token) || strings.Contains(strings.Join(p.Steps[0].Shown(), " "), token) {
		t.Errorf("step %+v", p.Steps[0])
	}
	if !codexGithub.Disabled && codexGithub.Command != "npx" {
		t.Errorf("codex's github: %+v", codexGithub)
	}

	// Codex's env_vars have no counterpart elsewhere.
	if _, err := MCPConnectPlan(server(t, env, "codex", "passed"), "cursor", env, title); err == nil {
		t.Error("a plan copying env_vars")
	}
}

func TestMCPRemoveAndToggle(t *testing.T) {
	_, env := mcpHome(t)
	github := server(t, env, "claude", "github")
	for _, id := range []string{"cursor", "codex"} {
		p, err := MCPConnectPlan(github, id, env, title)
		if err != nil {
			t.Fatal(err)
		}
		runEdits(t, p)
	}

	// Codex turns it off by its enabled, and on again.
	codex := server(t, env, "codex", "github")
	if !MCPToggles(codex) || MCPToggles(server(t, env, "cursor", "github")) {
		t.Fatal("toggles: Codex's alone")
	}
	p, err := MCPEnablePlan(codex, false, env, title)
	if err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)
	if !server(t, env, "codex", "github").Disabled {
		t.Fatal("still on")
	}

	// Cursor's goes from its file; Codex's table goes with its env.
	for _, id := range []string{"cursor", "codex"} {
		p, err := MCPRemovePlan(server(t, env, id, "github"), env, title)
		if err != nil {
			t.Fatal(err)
		}
		runEdits(t, p)
	}
	servers, _ := ai.MCP(env)
	if slices.ContainsFunc(servers, func(s inventory.MCPServer) bool { return s.Name == "github" && s.Agent != "claude" }) {
		t.Errorf("left: %+v", servers)
	}

	// Claude Code's own goes through claude mcp; its project's in
	// ~/.claude.json does not.
	p, err = MCPRemovePlan(github, env, title)
	if err != nil || !slices.Equal(p.Steps[0].Argv, []string{"claude", "mcp", "remove", "--scope", "user", "github"}) {
		t.Errorf("claude: %+v, %v", p.Steps, err)
	}
	local := slices.IndexFunc(servers, func(s inventory.MCPServer) bool { return s.Name == "local" })
	if local < 0 || MCPRemovable(servers[local]) {
		t.Error("Claude Code's project server is removable")
	}
}

func TestAntigravityMCP(t *testing.T) {
	home, env := mcpHome(t)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{
		"remote":{"type":"http","url":"https://example.test/mcp","headers":{"Authorization":"Bearer `+token+`"}},
		"events":{"type":"sse","url":"https://example.test/sse"}
	}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := MCPConnectPlan(server(t, env, "claude", "events"), "antigravity", env, title); err == nil {
		t.Fatal("Antigravity CLI does not support SSE")
	}
	p, err := MCPConnectPlan(server(t, env, "claude", "remote"), "antigravity", env, title)
	if err != nil {
		t.Fatal(err)
	}
	e := p.Steps[0].Edit
	if want := filepath.Join(home, ".gemini", "config", "mcp_config.json"); e.Path != want {
		t.Errorf("edit path = %s, want %s", e.Path, want)
	}
	if !strings.Contains(e.After, `"serverUrl"`) || strings.Contains(e.Masked(e.After), token) {
		t.Fatal("plan must use serverUrl and redact the token")
	}
	runEdits(t, p)
	for _, on := range []bool{false, true} {
		p, err := MCPEnablePlan(server(t, env, "antigravity", "remote"), on, env, title)
		if err != nil {
			t.Fatal(err)
		}
		runEdits(t, p)
		if s := server(t, env, "antigravity", "remote"); s.Disabled == on || s.Transport != "http" {
			t.Errorf("after enabled=%t: %+v", on, s)
		}
	}
	p, err = MCPRemovePlan(server(t, env, "antigravity", "remote"), env, title)
	if err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)
}

func TestMCPConnectBlankConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		before  string
		changed bool
	}{
		{name: "empty file"},
		{name: "whitespace", before: " \t\r\n"},
		{name: "file changed after preview", changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, env := mcpHome(t)
			path := filepath.Join(home, ".gemini", "config", "mcp_config.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.before), 0o600); err != nil {
				t.Fatal(err)
			}
			p, err := MCPConnectPlan(server(t, env, "claude", "github"), "antigravity", env, title)
			if err != nil {
				t.Fatal(err)
			}
			if tc.changed {
				const updated = "{}\n"
				if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
					t.Fatal(err)
				}
				s := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
				if s.Phase != Failed || !s.FileChanged {
					t.Fatalf("phase %v, file changed %t, error %q, want a stale plan failure", s.Phase, s.FileChanged, s.Err)
				}
				checkFile(t, path, updated, 0o600)
				return
			}
			runEdits(t, p)
			if s := server(t, env, "antigravity", "github"); s.Command != "npx" {
				t.Errorf("connected command = %q, want npx", s.Command)
			}
			backups, err := filepath.Glob(path + ".hearth-*.bak")
			if err != nil || len(backups) != 1 {
				t.Fatalf("backups = %v, err = %v, want one backup", backups, err)
			}
			checkFile(t, backups[0], tc.before, 0o600)
		})
	}
}

func TestMCPAddAndEditPlan(t *testing.T) {
	home, env := mcpHome(t)

	// Add stdio server to Cursor with secret env var masked in diff.
	p, err := MCPAddPlan("linear", "cursor", "", ai.MCPDef{
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "@linear/mcp"},
		Env:       map[string]string{"LINEAR_API_KEY": token},
	}, env, title)
	if err != nil {
		t.Fatal(err)
	}
	e := p.Steps[0].Edit
	if e == nil || !e.New || !strings.Contains(e.After, token) || strings.Contains(e.Masked(e.After), token) {
		t.Fatalf("add edit did not mask secret: %+v", e)
	}
	runEdits(t, p)

	// Edit Cursor server to HTTP with secret header; both old and new secrets masked.
	newToken := "sk-newsecret999"
	cur := server(t, env, "cursor", "linear")
	p, err = MCPEditPlan(cur, ai.MCPDef{
		Transport: "http",
		URL:       "https://mcp.linear.app/mcp",
		Headers:   map[string]string{"Authorization": "Bearer " + newToken},
	}, env, title)
	if err != nil {
		t.Fatal(err)
	}
	e = p.Steps[0].Edit
	if e == nil || strings.Contains(e.Masked(e.Before), token) || strings.Contains(e.Masked(e.After), newToken) {
		t.Fatalf("edit did not mask old/new secrets: %+v", e)
	}
	runEdits(t, p)
	updated := server(t, env, "cursor", "linear")
	if updated.Transport != "http" || updated.URL != "https://mcp.linear.app/mcp" {
		t.Fatalf("updated cursor server = %+v", updated)
	}

	// Add + Edit Codex TOML preserving disabled state.
	p, err = MCPAddPlan("docs", "codex", "", ai.MCPDef{Transport: "stdio", Command: "uvx", Args: []string{"docs-mcp"}}, env, title)
	if err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)
	codexSrv := server(t, env, "codex", "docs")
	p, err = MCPEnablePlan(codexSrv, false, env, title)
	if err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)
	codexSrv = server(t, env, "codex", "docs")
	p, err = MCPEditPlan(codexSrv, ai.MCPDef{Transport: "stdio", Command: "npx", Args: []string{"docs-v2"}}, env, title)
	if err != nil {
		t.Fatal(err)
	}
	runEdits(t, p)
	codexSrv = server(t, env, "codex", "docs")
	if codexSrv.Command != "npx" || !codexSrv.Disabled {
		t.Fatalf("edited codex server = %+v, want command=npx and Disabled=true", codexSrv)
	}

	// Edit Claude Code server uses claude mcp remove + add-json with Undo and masks secrets.
	claudeSrv := server(t, env, "claude", "github")
	p, err = MCPEditPlan(claudeSrv, ai.MCPDef{
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "@modelcontextprotocol/server-github@latest"},
		Env:       map[string]string{"GITHUB_TOKEN": newToken},
	}, env, title)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || p.Steps[0].Argv[2] != "remove" || p.Steps[0].Undo == nil || p.Steps[1].Argv[2] != "add-json" {
		t.Fatalf("unexpected claude edit steps: %+v", p.Steps)
	}
	if strings.Contains(strings.Join(p.Steps[1].Shown(), " "), newToken) {
		t.Fatalf("claude edit step leaked secret: %v", p.Steps[1].Shown())
	}

	// Validation & Antigravity SSE rejection.
	if _, err := MCPAddPlan("", "cursor", "", ai.MCPDef{Transport: "stdio", Command: "npx"}, env, title); err == nil {
		t.Error("empty name accepted")
	}
	if _, err := MCPAddPlan("-flag", "cursor", "", ai.MCPDef{Transport: "stdio", Command: "npx"}, env, title); err == nil {
		t.Error("leading dash server name accepted")
	}
	if _, err := MCPAddPlan("bad", "cursor", "", ai.MCPDef{Transport: "http", URL: "ftp://example.com"}, env, title); err == nil {
		t.Error("non-http URL accepted")
	}
	if _, err := MCPAddPlan("sse-test", "antigravity", "", ai.MCPDef{Transport: "sse", URL: "https://example.com/sse"}, env, title); err == nil {
		t.Error("antigravity SSE accepted")
	}

	// Codex edit preserves bearer_token_env_var / env_http_headers on HTTP and env_vars on stdio.
	codexCfg := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codexCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	initialTOML := "[mcp_servers.api]\nurl = \"https://old.example/mcp\"\nbearer_token_env_var = \"API_TOKEN\"\n[mcp_servers.api.env_http_headers]\nX-Org = \"ORG_ID\"\n\n[mcp_servers.local]\ncommand = \"npx\"\nargs = [\"-y\", \"demo\"]\nenv_vars = [\"EXTRA_KEY\"]\n"
	if err := os.WriteFile(codexCfg, []byte(initialTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	codexAPISrv := server(t, env, "codex", "api")
	pCodex, err := MCPEditPlan(codexAPISrv, ai.MCPDef{
		Transport: "sse",
		URL:       "https://new.example/mcp",
	}, env, title)
	if err != nil {
		t.Fatalf("codex http edit: %v", err)
	}
	codexAfter := pCodex.Steps[0].Edit.After
	for _, keep := range []string{`bearer_token_env_var = "API_TOKEN"`, `X-Org = "ORG_ID"`} {
		if !strings.Contains(codexAfter, keep) {
			t.Errorf("codex http edit dropped %s:\n%s", keep, codexAfter)
		}
	}
	if err := os.WriteFile(codexCfg, []byte(codexAfter), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pCodex.Verify(context.Background()); err != nil {
		t.Fatalf("codex edit verify failed for sse->http: %v", err)
	}

	codexLocalSrv := server(t, env, "codex", "local")
	pCodexStdio, err := MCPEditPlan(codexLocalSrv, ai.MCPDef{
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "demo@2"},
	}, env, title)
	if err != nil {
		t.Fatalf("codex stdio edit: %v", err)
	}
	if !strings.Contains(pCodexStdio.Steps[0].Edit.After, `"EXTRA_KEY"`) {
		t.Errorf("codex stdio edit dropped env_vars:\n%s", pCodexStdio.Steps[0].Edit.After)
	}
}
