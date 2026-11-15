package issues

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestID(t *testing.T) {
	original := Issue{Kind: PlainSecret, MCP: []inventory.MCPServer{{Name: "local", Agent: "codex", File: "config"}}}
	id := ID(original)
	if len(id) != 64 || id != ID(original) {
		t.Fatal("unstable identity")
	}
	original.MCP[0].Args = []string{"a-secret"}
	if ID(original) != id {
		t.Fatal("secret used as target identity")
	}
	original.MCP[0].Name = "another"
	if ID(original) == id {
		t.Fatal("different server reused identity")
	}
}
