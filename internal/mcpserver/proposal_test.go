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
