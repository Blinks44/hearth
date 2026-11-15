package ops

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestHearthMCPPlan(t *testing.T) {
	for _, tc := range []struct{ name, agent string }{{"Codex config", "codex"}, {"Claude CLI", "claude"}, {"Cursor JSON", "cursor"}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env := sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir}
			exe := filepath.Join(dir, "Hearth binary")
			if err := os.WriteFile(exe, nil, 0755); err != nil {
				t.Fatal(err)
			}
			p, err := HearthMCPPlan(exe, tc.agent, env, func(s string) string { return s })
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Steps) != 1 {
				t.Fatalf("steps: %+v", p.Steps)
			}
			config := ai.MCPFile(tc.agent, env, "")
			if _, err := os.Stat(config); !os.IsNotExist(err) {
				t.Fatal("planning wrote config")
			}
			if tc.agent == "claude" {
				args := p.Steps[0].Argv
				if len(args) != 7 || args[0] != "claude" || args[2] != "add-json" || !strings.Contains(args[6], `"args":["mcp"]`) {
					t.Fatalf("not existing Claude add operation: %v", args)
				}
				return
			}
			state := Execute(t.Context(), nil, p, io.Discard, func(State) {})
			if state.Phase != Succeeded {
				t.Fatalf("state %+v", state)
			}
			servers, _ := ai.MCP(env)
			s, ok := findServer(servers, tc.agent, "hearth", "")
			if !ok {
				t.Fatal("server not discovered")
			}
			def, err := ai.ReadMCPDef(s)
			if err != nil || def.Command != exe || len(def.Args) != 1 || def.Args[0] != "mcp" {
				t.Fatalf("bad declaration %+v %v", def, err)
			}
			if _, err := HearthMCPPlan(exe, tc.agent, env, func(s string) string { return s }); err == nil {
				t.Fatal("overwrote existing Hearth server")
			}
		})
	}
}
