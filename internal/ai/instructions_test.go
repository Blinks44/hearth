package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestCodexInstructions(t *testing.T) {
	for _, tc := range []struct{ name, shared, native, override, state, active string }{
		{name: "missing", state: "missing"},
		{name: "shared only", shared: "Common", state: "shared"},
		{name: "native only", native: "Native", state: "native", active: "AGENTS.md"},
		{name: "independent same", shared: "Common", native: "Common", state: "same", active: "AGENTS.md"},
		{name: "different", shared: "Common", native: "Native", state: "different", active: "AGENTS.md"},
		{name: "override wins", shared: "Common", native: "Native", override: "Override", state: "override", active: "AGENTS.override.md"},
		{name: "empty override falls back", native: "Native", override: " \r\n", state: "native", active: "AGENTS.md"},
		{name: "empty native protected", shared: "Common", native: " \n", state: "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			custom := filepath.Join(home, "custom-codex")
			env := sysenv.Env{"HOME=" + home, "CODEX_HOME=" + custom}
			files := map[string]string{filepath.Join(home, ".agents/AGENTS.md"): tc.shared, filepath.Join(custom, "AGENTS.md"): tc.native, filepath.Join(custom, "AGENTS.override.md"): tc.override}
			for p, txt := range files {
				if txt != "" {
					if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte(txt), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := CodexInstructions(env)
			if got.State != tc.state {
				t.Fatalf("state %s, want %s: %+v", got.State, tc.state, got)
			}
			want := ""
			if tc.active != "" {
				want = filepath.Join(custom, tc.active)
			}
			if got.Active != want {
				t.Fatalf("active %q, want %q", got.Active, want)
			}
			// No contents are persisted, even for instruction files that contain secrets.
			ag := byID(Detect(t.Context(), &run.Fake{}, env, nil))["codex"]
			if ag.Instructions == nil || *ag.Instructions != got {
				t.Fatal("scan lost global instructions")
			}
		})
	}
}

func TestCodexInstructionLinksAndReadErrors(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{"USERPROFILE=" + home}
	shared := filepath.Join(home, ".agents/AGENTS.md")
	native := filepath.Join(home, ".codex/AGENTS.md")
	for _, p := range []string{shared, native} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	const secret = "private-instruction-token-12345"
	if err := os.WriteFile(shared, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, native); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := CodexInstructions(env)
	if got.State != "linked" || got.Active != native {
		t.Fatalf("link: %+v", got)
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), secret) {
		t.Fatal("instruction text leaked into inventory")
	}
	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	got = CodexInstructions(env)
	if got.State != "unreadable" || !got.Native.Exists || !got.Native.Link {
		t.Fatalf("broken link treated as absent: %+v", got)
	}
}

func TestInstructionText(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"large", []byte(strings.Repeat("x", 256*1024+1))}, {"invalid UTF8", []byte{255, 254}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "AGENTS.md")
			if err := os.WriteFile(p, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := InstructionText(p); err == nil {
				t.Fatal("accepted unreadable instructions")
			}
		})
	}
	if _, err := InstructionText(t.TempDir()); err == nil {
		t.Fatal("read directory")
	}
}

func TestAgentInstructionsAllAgents(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{
		"HOME=" + home,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(home, "claude-cfg"),
		"CODEX_HOME=" + filepath.Join(home, "codex-cfg"),
		"COPILOT_HOME=" + filepath.Join(home, "copilot-cfg"),
		"OPENCODE_CONFIG_DIR=" + filepath.Join(home, "opencode-cfg"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"GROK_HOME=" + filepath.Join(home, "grok-cfg"),
		"KIMI_CODE_HOME=" + filepath.Join(home, "kimi-cfg"),
	}
	shared := filepath.Join(home, ".agents", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(shared), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("Common rules\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wantNative := map[string]string{
		"claude":      filepath.Join(home, "claude-cfg", "CLAUDE.md"),
		"codex":       filepath.Join(home, "codex-cfg", "AGENTS.md"),
		"copilot":     filepath.Join(home, "copilot-cfg", "copilot-instructions.md"),
		"antigravity": filepath.Join(home, ".gemini", "config", "AGENTS.md"),
		"gemini":      filepath.Join(home, ".gemini", "GEMINI.md"),
		"opencode":    filepath.Join(home, "opencode-cfg", "AGENTS.md"),
		"devin":       filepath.Join(home, "xdg", "devin", "AGENTS.md"),
		"grok":        filepath.Join(home, "grok-cfg", "AGENTS.md"),
		"kimi":        filepath.Join(home, "kimi-cfg", "AGENTS.md"),
	}
	for id, wantPath := range wantNative {
		got, ok := AgentInstructions(id, env)
		if !ok {
			t.Fatalf("%s: expected global instructions support", id)
		}
		if got.State != "shared" || got.Native.Path != wantPath {
			t.Errorf("%s: state=%q path=%q, want shared / %q", id, got.State, got.Native.Path, wantPath)
		}
	}
	for _, id := range []string{"cursor", "aider", "unknown"} {
		if _, ok := AgentInstructions(id, env); ok {
			t.Errorf("%s: unexpected global instructions support", id)
		}
	}

	detected := byID(Detect(t.Context(), &run.Fake{}, env, nil))
	for id := range wantNative {
		if detected[id].Instructions == nil || detected[id].Instructions.State != "shared" {
			t.Errorf("Detect(%s) missing shared instructions: %+v", id, detected[id].Instructions)
		}
	}
	if detected["cursor"].Instructions != nil || detected["aider"].Instructions != nil {
		t.Error("cursor/aider should not report file-based global instructions")
	}
}

func TestAgentInstructionsImportsAndAntigravityPaths(t *testing.T) {
	write := func(t *testing.T, p, txt string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(txt), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("claude and gemini @ import", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		write(t, filepath.Join(home, ".agents/AGENTS.md"), "Shared\n")
		write(t, filepath.Join(home, ".claude/CLAUDE.md"), "@~/.agents/AGENTS.md\n")
		write(t, filepath.Join(home, ".gemini/GEMINI.md"), "@"+filepath.Join(home, ".agents/AGENTS.md")+"\n")
		for _, id := range []string{"claude", "gemini"} {
			got, ok := AgentInstructions(id, env)
			if !ok || got.State != "linked" || got.Native.Link {
				t.Errorf("%s: got %+v (%v), want linked non-symlink import", id, got, ok)
			}
		}
	})

	t.Run("fenced or commented import is not linked", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		write(t, filepath.Join(home, ".agents/AGENTS.md"), "Shared\n")
		write(t, filepath.Join(home, ".claude/CLAUDE.md"), "```md\n@~/.agents/AGENTS.md\n```\n<!-- @~/.agents/AGENTS.md -->\n")
		got, _ := AgentInstructions("claude", env)
		if got.State != "different" {
			t.Fatalf("got state %q, want different", got.State)
		}
	})

	t.Run("antigravity markdown include and alternate path", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		write(t, filepath.Join(home, ".agents/AGENTS.md"), "Shared\n")
		// Bare @path is not an Antigravity inline include.
		write(t, filepath.Join(home, ".gemini/GEMINI.md"), "@~/.agents/AGENTS.md\n")
		got, _ := AgentInstructions("antigravity", env)
		if got.State != "different" || got.Native.Path != filepath.Join(home, ".gemini/GEMINI.md") {
			t.Fatalf("bare @ in antigravity: %+v", got)
		}
		// @[label](path) is an inline include and takes precedence over a non-linked sibling.
		write(t, filepath.Join(home, ".gemini/config/AGENTS.md"), "@[Global](~/.agents/AGENTS.md)\n")
		got, _ = AgentInstructions("antigravity", env)
		if got.State != "linked" || got.Native.Path != filepath.Join(home, ".gemini/config/AGENTS.md") {
			t.Fatalf("markdown include in antigravity: %+v", got)
		}
	})

	t.Run("antigravity broken link in alternate path is surfaced", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		write(t, filepath.Join(home, ".agents/AGENTS.md"), "Shared\n")
		alt := filepath.Join(home, ".gemini/AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(alt), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, "missing"), alt); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		got, _ := AgentInstructions("antigravity", env)
		if got.State != "unreadable" || got.Native.Path != alt {
			t.Fatalf("broken alternate link: %+v", got)
		}
	})
}
