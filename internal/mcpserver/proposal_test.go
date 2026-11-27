package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/proposals"
)

func TestProposalTool(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed window", true: "open window"}[live], func(t *testing.T) {
			calls := 0
			server := New(&fakeSource{state: fixture()}, "test", func(_ context.Context, q proposals.Request) error {
				calls++
				if !live {
					return proposals.ErrNoWindow
				}
				return nil
			})
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(t.Context(), st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = ss.Close() }()
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(t.Context(), ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cs.Close() }()
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "propose_fix", Arguments: map[string]any{"issue_id": strings.Repeat("a", 64)}})
			if err != nil || res.IsError == live || calls != 1 {
				t.Fatalf("proposal: %v %+v calls %d", err, res, calls)
			}
			res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "propose_fix", Arguments: map[string]any{"issue_id": "invalid"}})
			if err != nil || !res.IsError || calls != 1 {
				t.Fatal("invalid request forwarded")
			}
		})
	}
}

func TestExtraProposalTools(t *testing.T) {
	var got []proposals.Request
	server := New(&fakeSource{state: fixture()}, "test", func(_ context.Context, q proposals.Request) error {
		got = append(got, q)
		return nil
	})
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()

	projDir := t.TempDir()
	cases := []struct {
		name     string
		args     map[string]any
		wantKind string
		wantErr  bool
	}{
		{"propose_project_prepare", map[string]any{"project": projDir}, proposals.KindProjectPrepare, false},
		{"propose_project_prepare", map[string]any{"project": "relative/dir"}, "", true},
		{"propose_stop_process", map[string]any{"port": "3000", "pid": "1234"}, proposals.KindStopProcess, false},
		{"propose_stop_process", map[string]any{"port": "bad", "pid": "1234"}, "", true},
		{"propose_service_action", map[string]any{"service": "postgresql@16", "action": "restart"}, proposals.KindServiceAction, false},
		{"propose_service_action", map[string]any{"service": "postgresql@16", "action": "nuke"}, "", true},
		{"propose_skill_action", map[string]any{"action": "link_all", "skill": "lint"}, proposals.KindSkillAction, false},
		{"propose_skill_action", map[string]any{"action": "link_all"}, "", true},
		{"propose_tool_install", map[string]any{"tool": "node", "version": "22.14.0"}, proposals.KindToolInstall, false},
		{"propose_tool_install", map[string]any{"tool": ""}, "", true},
		{"propose_cleanup", map[string]any{"kinds": []string{"cache"}}, proposals.KindCleanup, false},
		{"propose_cleanup", map[string]any{}, proposals.KindCleanup, false},
	}
	expectedCalls := 0
	for _, tc := range cases {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.IsError != tc.wantErr {
			t.Fatalf("%s(%v): IsError=%v want %v", tc.name, tc.args, res.IsError, tc.wantErr)
		}
		if !tc.wantErr {
			expectedCalls++
			if got[len(got)-1].EffectiveKind() != tc.wantKind {
				t.Fatalf("%s: got kind %q want %q", tc.name, got[len(got)-1].EffectiveKind(), tc.wantKind)
			}
		}
	}
	if len(got) != expectedCalls {
		t.Fatalf("forwarded %d requests, want %d", len(got), expectedCalls)
	}
}
