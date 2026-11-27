package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/aiquota"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

type fakeSource struct {
	state   State
	err     error
	refresh bool
}

func (f *fakeSource) Get(_ context.Context, refresh bool) (State, error) {
	f.refresh = refresh
	return f.state, f.err
}

func clientFor(t *testing.T, src Source) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := New(src, "test").Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Close() })
	return cs
}

func fixture() State {
	copies := []inventory.Installation{
		{Tool: "node", Version: "22.1.0", Path: "/tools/node22/bin/node", OnPath: 1},
		{Tool: "node", Version: "20.1.0", Path: "/tools/node20/bin/node", OnPath: 2},
	}
	return State{Cached: true, Snapshot: inventory.Snapshot{Taken: time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), Tools: []inventory.ToolState{{Tool: "node", Installations: copies}}}, Projects: []projects.Project{{Dir: "/project", Requirements: []projects.Requirement{{Tool: "node", Want: "20", File: ".nvmrc"}}}}}
}

func richFixture() State {
	st := fixture()
	today := time.Now().Format("2006-01-02")
	st.Snapshot.Tools = append(st.Snapshot.Tools,
		inventory.ToolState{
			Tool: "docker",
			Installations: []inventory.Installation{
				{Tool: "docker", Version: "28.0.0", Path: "/usr/local/bin/docker", OnPath: 1},
			},
			Docker: &inventory.DockerState{
				Selected:   "colima",
				Running:    1,
				Containers: 2,
				Contexts:   []inventory.DockerContext{{Name: "colima", Engine: "colima", Status: "running", Version: "28.0.0", Local: true, Endpoint: "unix:///tmp/colima.sock"}},
				Engines:    []inventory.DockerEngine{{Name: "Colima", Path: "/opt/homebrew/bin/colima"}},
				KindClusters: []inventory.KindCluster{
					{Name: "dev", Nodes: 1, Running: 1, Version: "v1.31.0"},
				},
			},
		},
		inventory.ToolState{
			Tool: "databases",
			Databases: &inventory.DatabaseState{
				Servers: []inventory.Database{
					{Name: "PostgreSQL 16", Kind: "PostgreSQL", Version: "16.4", Service: "postgresql@16", Source: "brew", Status: "running", Ports: []string{"5432"}},
				},
				Listeners: []inventory.DatabaseListener{
					{Address: "127.0.0.1", Port: "3000", Process: "node", PID: "9001", Dirs: []string{"/project"}, Args: []string{"node", "server.js"}, Own: true},
				},
			},
		},
		inventory.ToolState{
			Tool: "cli",
			Cli: &inventory.CliState{
				Contexts:     []inventory.CliContext{{Tool: "kubectl", Name: "kind-dev"}},
				KubeContexts: []inventory.KubeContext{{Name: "kind-dev", Cluster: "kind-dev", Namespace: "default"}},
			},
		},
		inventory.ToolState{
			Tool: "models",
			Models: &inventory.ModelsState{
				Ollama: &inventory.ModelApp{
					Version: "0.6.0",
					Port:    "11434",
					Dir:     "/home/u/.ollama",
					Models:  []inventory.Model{{Name: "llama3.2:latest", Size: 2_000_000_000, Downloaded: time.Now()}},
				},
			},
		},
	)
	st.Snapshot.Agents = []inventory.Agent{
		{
			ID: "claude",
			Instructions: &inventory.GlobalInstructions{
				State:  "linked",
				Active: "/home/u/.claude/CLAUDE.md",
				Native: inventory.InstructionFile{Path: "/home/u/.claude/CLAUDE.md", Exists: true, Link: true},
			},
		},
	}
	st.Snapshot.Skills = []inventory.Skill{
		{Name: "lint-rules", Description: "Lint conventions", Dir: "/home/u/.agents/skills/lint-rules", Paths: []inventory.SkillPath{{Path: "/home/u/.claude/skills/lint-rules", Agents: []string{"claude"}, Link: true}}},
	}
	st.Snapshot.Sessions = []inventory.AgentSession{
		{ID: "sess-1", Agent: "claude", Usage: []inventory.TokenUsage{{Day: today, Model: "claude-sonnet-4", Input: 1000, Output: 500}}},
	}
	st.Sizes = &inventory.Sizes{
		Taken:  time.Now(),
		Paths:  map[string]inventory.Usage{"/home/u/.npm": {Bytes: 64_000_000}},
		Caches: []inventory.Cache{{ID: "npm", Path: "/home/u/.npm"}},
		Docker: []inventory.DockerUsage{{Type: "Images", Count: 3, Reclaimable: 512_000_000}},
	}
	st.Quotas = &aiquota.Snapshot{
		CheckedAt: time.Now(),
		Providers: []aiquota.ProviderQuota{
			{
				Provider:  aiquota.ProviderClaude,
				Name:      "Claude Code",
				Plan:      "Max",
				Status:    aiquota.StatusOK,
				UpdatedAt: time.Now(),
				Windows: []aiquota.QuotaWindow{{
					ID:       "five_hour",
					Kind:     aiquota.WindowSession,
					Used:     28,
					ResetsAt: time.Now().Add(2 * time.Hour),
					Duration: 5 * time.Hour,
				}},
			},
		},
	}
	return st
}

func TestTools(t *testing.T) {
	src := &fakeSource{state: richFixture()}
	cs := clientFor(t, src)
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 20 {
		t.Fatalf("expected 20 tools, got %d: %+v", len(list.Tools), list.Tools)
	}
	readOnlyCount := 0
	proposalCount := 0
	for _, tool := range list.Tools {
		if strings.HasPrefix(tool.Name, "propose_") {
			proposalCount++
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
				t.Fatalf("proposal tool should not be read-only: %s", tool.Name)
			}
			continue
		}
		readOnlyCount++
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("not read-only: %s", tool.Name)
		}
	}
	if readOnlyCount != 13 || proposalCount != 7 {
		t.Fatalf("readOnly=%d proposal=%d, want 13 and 7", readOnlyCount, proposalCount)
	}
	for _, tc := range []struct {
		name, want string
		args       map[string]any
	}{
		{"get_tools", "22.1.0", map[string]any{"tool": "node"}},
		{"get_projects", "20.1.0", map[string]any{"path": "/project"}},
		{"get_issues", "Multiple installed copies", nil},
		{"get_agents", "mcp_servers", nil},
		{"get_docker", "colima", nil},
		{"get_ports_and_databases", "dev_server", map[string]any{"port": "3000"}},
		{"get_skills", "lint-rules", nil},
		{"get_kubernetes", "kind-dev", nil},
		{"get_cleanup", "reclaimable_bytes", nil},
		{"get_usage", "claude-sonnet-4", map[string]any{"days": 30}},
		{"get_models", "llama3.2:latest", nil},
		{"get_instructions", "CLAUDE.md", nil},
		{"get_ai_quotas", "Claude Code", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil || res.IsError {
				t.Fatalf("call: %v %+v", err, res)
			}
			data, err := json.Marshal(res)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.want) || !strings.Contains(string(data), "scanned_at") {
				t.Fatalf("missing result: %s", data)
			}
		})
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_tools", Arguments: map[string]any{"refresh": true}})
	if err != nil || res.IsError || !src.refresh {
		t.Fatalf("refresh not forwarded: %v %+v", err, res)
	}
	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_projects", Arguments: map[string]any{"path": "/not-registered"}})
	if err != nil || res.IsError {
		t.Fatal(err)
	}
	data, _ := json.Marshal(res.StructuredContent)
	if strings.Contains(string(data), "/project") {
		t.Fatalf("wrong project filter: %s", data)
	}
}

func TestResourcesAndPrompts(t *testing.T) {
	cs := clientFor(t, &fakeSource{state: richFixture()})

	rList, err := cs.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rList.Resources) != 4 {
		t.Fatalf("expected 4 resources, got %d", len(rList.Resources))
	}
	for _, tc := range []struct{ uri, want string }{
		{"hearth://summary", `"docker_context": "colima"`},
		{"hearth://projects", "/project"},
		{"hearth://issues", "Multiple installed copies"},
		{"hearth://ports", "postgresql@16"},
	} {
		t.Run(tc.uri, func(t *testing.T) {
			res, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: tc.uri})
			if err != nil || len(res.Contents) != 1 {
				t.Fatalf("read %s: %v %+v", tc.uri, err, res)
			}
			if !strings.Contains(res.Contents[0].Text, tc.want) {
				t.Fatalf("resource %s missing %q: %s", tc.uri, tc.want, res.Contents[0].Text)
			}
		})
	}

	pList, err := cs.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pList.Prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(pList.Prompts))
	}
	for _, tc := range []struct {
		name string
		args map[string]string
		want string
	}{
		{"diagnose-project", map[string]string{"path": "/project"}, "get_projects"},
		{"troubleshoot-port", map[string]string{"port": "3000"}, "get_ports_and_databases"},
		{"audit-ai-setup", nil, "get_instructions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: tc.name, Arguments: tc.args})
			if err != nil || len(res.Messages) == 0 {
				t.Fatalf("prompt %s: %v %+v", tc.name, err, res)
			}
			txt, ok := res.Messages[0].Content.(*mcp.TextContent)
			if !ok || !strings.Contains(txt.Text, tc.want) {
				t.Fatalf("prompt %s missing %q: %+v", tc.name, tc.want, res.Messages[0].Content)
			}
		})
	}
	if _, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "diagnose-project"}); err == nil {
		t.Fatal("diagnose-project accepted missing path")
	}
	if _, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "troubleshoot-port"}); err == nil {
		t.Fatal("troubleshoot-port accepted missing port")
	}
}

func TestSecretsNeverReturned(t *testing.T) {
	const secret = "secret-do-not-return-123"
	st := richFixture()
	st.Snapshot.Tools[0].Err = secret
	st.Snapshot.Tools[0].Vars = map[string]string{"TOKEN": secret}
	st.Snapshot.Tools[0].Managers = []inventory.Manager{{Settings: map[string]string{"token": secret}}}
	st.Snapshot.Variables = []inventory.EnvironmentVariable{{Name: "TOKEN", Terminal: secret, GUI: secret}}
	st.Snapshot.MCPErrs = []string{secret}
	st.Snapshot.MCP = []inventory.MCPServer{{Name: "service", Agent: "claude", Transport: "http", Command: secret, Args: []string{secret}, URL: "https://user:" + secret + "@example.com/?token=" + secret, PlainArgs: true}}
	st.Snapshot.Agents[0].Rules = []inventory.AgentRule{{Rule: secret}}
	st.Snapshot.Agents[0].Hooks = []inventory.AgentHook{{Command: secret}}
	st.Projects[0].Texts = map[string]string{"package.json": secret}
	st.Projects[0].Errs = []error{errors.New(secret)}
	st.Projects[0].Instructions.Agents.Text = secret
	st.Projects[0].MCP = st.Snapshot.MCP
	st.Snapshot.Sessions = []inventory.AgentSession{{ID: secret, Agent: "claude", Usage: []inventory.TokenUsage{{Day: time.Now().Format("2006-01-02"), Model: "claude-sonnet-4", Input: 10, Output: 10}}}}
	for i := range st.Snapshot.Tools {
		if st.Snapshot.Tools[i].Databases != nil {
			st.Snapshot.Tools[i].Databases.Listeners[0].Args = []string{"node", "app.js", "--api-key=" + secret}
		}
		if st.Snapshot.Tools[i].Docker != nil {
			st.Snapshot.Tools[i].Docker.Contexts = append(st.Snapshot.Tools[i].Docker.Contexts, inventory.DockerContext{Name: "remote", Local: false, Endpoint: "tcp://user:" + secret + "@example.com:2375"})
		}
		if st.Snapshot.Tools[i].Models != nil && st.Snapshot.Tools[i].Models.Ollama != nil {
			st.Snapshot.Tools[i].Models.Ollama.Err = secret
		}
	}
	st.Snapshot.Skills[0].Err = secret
	st.Sizes.DockerErr = secret

	src := &fakeSource{state: st}
	cs := clientFor(t, src)
	readTools := []string{
		"get_tools", "get_projects", "get_issues", "get_agents",
		"get_docker", "get_ports_and_databases", "get_skills", "get_kubernetes",
		"get_cleanup", "get_usage", "get_models", "get_instructions",
	}
	for _, name := range readTools {
		t.Run("tool/"+name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(res)
			if strings.Contains(string(data), secret) {
				t.Fatalf("secret leaked by %s: %s", name, data)
			}
		})
	}
	for _, uri := range []string{"hearth://summary", "hearth://projects", "hearth://issues", "hearth://ports"} {
		t.Run("resource/"+uri, func(t *testing.T) {
			res, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(res)
			if strings.Contains(string(data), secret) {
				t.Fatalf("secret leaked by %s: %s", uri, data)
			}
		})
	}
	src.err = errors.New(secret)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_tools"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(res)
	if !res.IsError || strings.Contains(string(data), secret) {
		t.Fatalf("error leaked a secret: %s", data)
	}
}

func TestProjectSelection(t *testing.T) {
	st := fixture()
	st.Snapshot.Tools[0].Managers = []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".nvmrc"}}}
	for i := range st.Snapshot.Tools[0].Installations {
		st.Snapshot.Tools[0].Installations[i].Source.Manager = "fnm"
	}
	cs := clientFor(t, &fakeSource{state: st})
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_projects"})
	if err != nil || res.IsError {
		t.Fatalf("result %v %v", res, err)
	}
	data, _ := json.Marshal(res.StructuredContent)
	var result ProjectsResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	req := result.Projects[0].Requirements[0]
	if req.Terminal == nil || req.Terminal.Version != "20.1.0" || !req.Met {
		t.Fatalf("wrong project selection: %+v", req)
	}
}
