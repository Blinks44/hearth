package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

func TestTools(t *testing.T) {
	src := &fakeSource{state: fixture()}
	cs := clientFor(t, src)
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 4 {
		t.Fatalf("tools: %+v", list.Tools)
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("not read-only: %s", tool.Name)
		}
	}
	for _, tc := range []struct {
		name, want string
		args       map[string]any
	}{
		{"get_tools", "22.1.0", map[string]any{"tool": "node"}},
		{"get_projects", "20.1.0", map[string]any{"path": "/project"}},
		{"get_issues", "Multiple installed copies", nil},
		{"get_agents", "mcp_servers", nil},
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

func TestSecretsNeverReturned(t *testing.T) {
	const secret = "secret-do-not-return-123"
	st := fixture()
	st.Snapshot.Tools[0].Err = secret
	st.Snapshot.Tools[0].Vars = map[string]string{"TOKEN": secret}
	st.Snapshot.Tools[0].Managers = []inventory.Manager{{Settings: map[string]string{"token": secret}}}
	st.Snapshot.Variables = []inventory.EnvironmentVariable{{Name: "TOKEN", Terminal: secret, GUI: secret}}
	st.Snapshot.MCPErrs = []string{secret}
	st.Snapshot.MCP = []inventory.MCPServer{{Name: "service", Agent: "claude", Transport: "http", Command: secret, Args: []string{secret}, URL: "https://user:" + secret + "@example.com/?token=" + secret, PlainArgs: true}}
	st.Snapshot.Agents = []inventory.Agent{{ID: "claude", Rules: []inventory.AgentRule{{Rule: secret}}, Hooks: []inventory.AgentHook{{Command: secret}}}}
	st.Projects[0].Texts = map[string]string{"package.json": secret}
	st.Projects[0].Errs = []error{errors.New(secret)}
	st.Projects[0].Instructions.Agents.Text = secret
	st.Projects[0].MCP = st.Snapshot.MCP
	st.Snapshot.Sessions = []inventory.AgentSession{{ID: secret}}
	src := &fakeSource{state: st}
	cs := clientFor(t, src)
	for _, name := range []string{"get_tools", "get_projects", "get_issues", "get_agents"} {
		t.Run(name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(res)
			if strings.Contains(string(data), secret) {
				t.Fatalf("secret leaked: %s", data)
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
