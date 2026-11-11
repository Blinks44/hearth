package ops

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestAgentInstallPlan(t *testing.T) {
	brew := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	winget := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "winget"}
	apt := inventory.PackageManager{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}
	tests := []struct {
		name     string
		id       string
		managers []inventory.PackageManager
		npm      bool
		want     []string // nil: Hearth does not install it so
	}{
		{"cask first", "claude", []inventory.PackageManager{brew}, true, []string{"/opt/homebrew/bin/brew", "install", "--cask", "claude-code"}},
		{"formula", "gemini", []inventory.PackageManager{brew}, false, []string{"/opt/homebrew/bin/brew", "install", "--formula", "gemini-cli"}},
		{"winget before npm", "copilot", []inventory.PackageManager{winget}, true,
			[]string{"winget", "install", "--id", "GitHub.Copilot", "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}},
		{"npm when the system's manager has none", "codex", []inventory.PackageManager{apt}, true, []string{"npm", "install", "--global", "@openai/codex"}},
		{"nothing without npm", "codex", []inventory.PackageManager{apt}, false, nil},
		{"an agent Hearth does not install", "devin", []inventory.PackageManager{brew, winget}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := AgentInstallPlan(tt.id, "Agent", tt.managers, tt.npm, nil)
			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, want %v", ok, tt.want != nil)
			}
			if ok && (len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.want)) {
				t.Errorf("steps = %+v, want %q", p.Steps, tt.want)
			}
		})
	}
}

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
	p, _ := AgentInstallPlan("codex", "Codex", nil, true, find(true))
	if found, err := p.Verify(context.Background()); err != nil || found != "Codex 0.167.1 found" {
		t.Errorf("Verify = %q, %v", found, err)
	}
	p, _ = AgentInstallPlan("codex", "Codex", nil, true, find(false))
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("Verify passed with the agent not found")
	}
}
