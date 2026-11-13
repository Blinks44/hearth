package ops

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestAgentNpmUpdatePlan(t *testing.T) {
	linked := inventory.Installation{Path: "/home/ann/.local/share/fnm/aliases/default/bin/claude",
		RealPath: "/home/ann/.local/share/fnm/node-versions/v22/installation/lib/node_modules/@anthropic-ai/claude-code/cli.js"}
	shim := inventory.Installation{Path: `C:\Users\Ann\AppData\Roaming\npm\codex.cmd`, RealPath: `C:\Users\Ann\AppData\Roaming\npm\codex.cmd`,
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm"}}
	native := inventory.Installation{Path: "/home/ann/.local/bin/claude", RealPath: "/home/ann/.local/share/claude/versions/2.1.322"}
	p, ok := AgentNpmUpdatePlan("claude", "Claude Code", linked, nil)
	// The copy's own prefix: the terminal's npm may be another Node's.
	prefix := filepath.FromSlash("/home/ann/.local/share/fnm/node-versions/v22/installation")
	if !ok || !slices.Equal(p.Steps[0].Argv, []string{"npm", "install", "--global", "--prefix", prefix, "@anthropic-ai/claude-code@latest"}) {
		t.Errorf("linked copy: %+v, %v", p.Steps, ok)
	}
	p, ok = AgentNpmUpdatePlan("codex", "Codex", shim, nil)
	if !ok || p.Steps[0].Argv[4] != filepath.Dir(shim.Path) {
		t.Errorf("npm's shim on Windows: %+v, %v", p.Steps, ok)
	}
	if _, ok := AgentNpmUpdatePlan("claude", "Claude Code", native, nil); ok {
		t.Error("a plan for the native install")
	}
	formula := inventory.Installation{Path: "/opt/homebrew/bin/gemini",
		RealPath: "/opt/homebrew/Cellar/gemini-cli/0.67.0/libexec/lib/node_modules/@google/gemini-cli/bundle/gemini.js",
		Source:   inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
	if _, ok := AgentNpmUpdatePlan("gemini", "Gemini CLI", formula, nil); ok {
		t.Error("a plan through npm for Homebrew's formula")
	}
	if _, ok := AgentNpmUpdatePlan("kimi", "Kimi Code", shim, nil); ok {
		t.Error("a plan for an agent npm does not publish")
	}
}

func TestAgentFound(t *testing.T) {
	find := func(found bool) FindAgent {
		return func(context.Context) (inventory.Agent, error) {
			if !found {
				return inventory.Agent{ID: "codex"}, nil
			}
			return inventory.Agent{ID: "codex", Installations: []inventory.Installation{{Version: "0.167.1", OnPath: 1}}}, nil
		}
	}
	if found, err := AgentFound("Codex", find(true))(context.Background()); err != nil || found != "Codex 0.167.1 found" {
		t.Errorf("Verify = %q, %v", found, err)
	}
	if _, err := AgentFound("Codex", find(false))(context.Background()); err == nil {
		t.Error("Verify passed with the agent not found")
	}
}

func TestToolFound(t *testing.T) {
	detect := func(ins ...inventory.Installation) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{Tool: "pnpm", Installations: ins}, nil
		}
	}
	if found, err := ToolFound("pnpm", detect(inventory.Installation{Version: "10.20.0", OnPath: 1}))(context.Background()); err != nil || found != "pnpm 10.20.0 found" {
		t.Errorf("Verify = %q, %v", found, err)
	}
	// A copy off the terminal's PATH is not what a new terminal runs.
	if _, err := ToolFound("pnpm", detect(inventory.Installation{Version: "10.20.0"}))(context.Background()); err == nil {
		t.Error("Verify passed with no copy on PATH")
	}
}
