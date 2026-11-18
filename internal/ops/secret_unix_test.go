//go:build unix

package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// secretTerminal is a terminal that reads the exports of Hearth's block in
// rc, as a new zsh would.
func secretTerminal(env sysenv.Env, rc string) Terminal {
	return func(context.Context) (sysenv.Env, error) {
		data, _ := os.ReadFile(rc)
		out := env
		for _, l := range splitLines(string(data)) {
			if rest, ok := strings.CutPrefix(l, "export "); ok {
				name, value, _ := strings.Cut(rest, "=")
				out = append(out, name+"="+strings.Trim(value, "'"))
			}
		}
		return out, nil
	}
}

// Cursor's server: the env's token and the header's go to .zshrc, the file
// takes them by ${env:...}, the scheme kept; nothing secret shows in the
// diff, and the scan finds no secret after.
func TestMCPSecretPlan(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "mcpServers": {
    "linear": {
      "command": "npx",
      "args": ["-y", "linear-mcp"],
      "env": {"LINEAR_API_KEY": "lin_api_s3cr3t", "LOG": "debug"},
      "headers": {"Authorization": "Bearer s3cr3t-bearer"}
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("alias ll='ls -l'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh", "PATH=" + os.Getenv("PATH")}
	s := server(t, env, "cursor", "linear")
	if !SecretMovable(s) {
		t.Fatalf("not movable: %+v", s)
	}
	p, err := MCPSecretPlan(s, env, secretTerminal(env, rc), title)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || p.Steps[0].Edit == nil || p.Steps[0].Edit.Path != rc {
		t.Fatalf("steps %+v", p.Steps)
	}
	for _, step := range p.Steps {
		e := step.Edit
		if shown := e.Masked(e.Before) + e.Masked(e.After); strings.Contains(shown, "s3cr3t") {
			t.Errorf("%s shows a secret:\n%s", e.Path, shown)
		}
	}
	for _, want := range []string{"export LINEAR_API_KEY='lin_api_s3cr3t'", "export LINEAR_TOKEN='s3cr3t-bearer'", "alias ll='ls -l'"} {
		if !strings.Contains(p.Steps[0].Edit.After, want) {
			t.Errorf("no %q in .zshrc:\n%s", want, p.Steps[0].Edit.After)
		}
	}
	after := p.Steps[1].Edit.After
	for _, want := range []string{`"LINEAR_API_KEY": "${env:LINEAR_API_KEY}"`, `"Authorization": "Bearer ${env:LINEAR_TOKEN}"`, `"LOG": "debug"`} {
		if !strings.Contains(after, want) {
			t.Errorf("no %s in mcp.json:\n%s", want, after)
		}
	}
	runEdits(t, p)
	if s := server(t, env, "cursor", "linear"); s.PlainSecret() {
		t.Errorf("a secret after the plan: %+v", s)
	}
	// The copies would keep the secret: they go once the plan got through.
	for _, dir := range []string{home, filepath.Dir(path)} {
		if copies, _ := filepath.Glob(filepath.Join(dir, "*.bak")); len(copies) > 0 {
			t.Errorf("copies left: %q", copies)
		}
	}
}

// A plan whose check fails puts the files back from their copies.
func TestMCPSecretPlanRestores(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	before := `{"mcpServers": {"gh": {"command": "gh-mcp", "env": {"GITHUB_TOKEN": "ghp_s3cr3t"}}}}`
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}
	// A terminal that does not read .zshrc, as a broken one.
	terminal := func(context.Context) (sysenv.Env, error) { return env, nil }
	p, err := MCPSecretPlan(server(t, env, "cursor", "gh"), env, terminal, title)
	if err != nil {
		t.Fatal(err)
	}
	if s := Execute(context.Background(), &run.Fake{}, p, io.Discard, func(State) {}); s.Phase != NotVerified {
		t.Fatalf("phase %v", s.Phase)
	}
	if data, _ := os.ReadFile(path); string(data) != before {
		t.Errorf("mcp.json not put back:\n%s", data)
	}
}

// A variable the terminal has at another value would win over the one
// moved: no plan. One at the same value is not set again.
func TestMCPSecretPlanTaken(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers": {"gh": {"command": "gh-mcp", "env": {"GITHUB_TOKEN": "ghp_s3cr3t"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/bash", "GITHUB_TOKEN=ghp_other"}
	s := server(t, env, "gemini", "gh")
	if _, err := MCPSecretPlan(s, env, nil, title); err == nil {
		t.Error("a plan over a variable at another value")
	}
	env[2] = "GITHUB_TOKEN=ghp_s3cr3t"
	p, err := MCPSecretPlan(s, env, nil, title)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Edit.Path != path || !strings.Contains(p.Steps[0].Edit.After, `"${GITHUB_TOKEN}"`) {
		t.Errorf("steps %+v", p.Steps)
	}
}

// Claude Code's user server goes and comes back through claude mcp, the
// variable in place of the token, which is not in the command.
func TestMCPSecretPlanClaude(t *testing.T) {
	home, env := mcpHome(t)
	env = append(env, "SHELL=/bin/zsh")
	p, err := MCPSecretPlan(server(t, env, "claude", "github"), env, nil, title)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 3 || p.Steps[0].Edit == nil || p.Steps[0].Edit.Path != filepath.Join(home, ".zshrc") {
		t.Fatalf("steps %+v", p.Steps)
	}
	remove, add := Command(p.Steps[1].Argv), strings.Join(p.Steps[2].Argv, " ")
	if remove != "claude mcp remove --scope user github" {
		t.Errorf("remove: %s", remove)
	}
	if !strings.Contains(add, `"GITHUB_TOKEN":"${GITHUB_TOKEN}"`) || strings.Contains(add, token) || !strings.Contains(add, "server-github") {
		t.Errorf("add: %s", add)
	}
}

// claudeCLI is claude mcp over a ~/.claude.json: remove and add-json of a
// user server, add-json failing with failAdd.
type claudeCLI struct {
	path    string
	failAdd bool
}

func (c claudeCLI) Run(_ context.Context, cmd run.Cmd) (run.Result, error) {
	a := cmd.Args
	if len(a) < 6 || a[0] != "claude" || a[1] != "mcp" || a[3] != "--scope" || a[4] != "user" {
		return run.Result{}, fmt.Errorf("unexpected %q", a)
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return run.Result{}, err
	}
	text := string(data)
	switch {
	case a[2] == "remove":
		text, err = JSONDelete(text, []string{"mcpServers", a[5]})
	case a[2] == "add-json" && len(a) == 7 && (!c.failAdd || !strings.Contains(a[6], "${")):
		text, err = JSONSet(text, []string{"mcpServers", a[5]}, json.RawMessage(a[6]))
	default:
		return run.Result{ExitCode: 1, Stderr: []byte("Invalid configuration")}, nil
	}
	if err != nil {
		return run.Result{}, err
	}
	return run.Result{}, os.WriteFile(c.path, []byte(text), 0o644)
}

// A server that does not come back with the variable, as add-json fails
// or the check does not pass, comes back as it was, the token with it,
// and .zshrc loses the variable.
func TestMCPSecretPlanClaudeRestores(t *testing.T) {
	for _, tt := range []struct {
		name     string
		failAdd  bool
		terminal Terminal
		phase    Phase
	}{
		{"add-json fails", true, nil, Failed},
		{"check fails", false, func(context.Context) (sysenv.Env, error) { return nil, nil }, NotVerified},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home, env := mcpHome(t)
			env = append(env, "SHELL=/bin/zsh")
			path := filepath.Join(home, ".claude.json")
			before := inner(mustRead(t, path), []string{"mcpServers", "github"})
			p, err := MCPSecretPlan(server(t, env, "claude", "github"), env, tt.terminal, title)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range p.Steps {
				if s.Undo != nil && strings.Contains(Command(s.Undo.Shown()), token) {
					t.Errorf("undo shows the token: %s", Command(s.Undo.Shown()))
				}
			}
			s := Execute(context.Background(), claudeCLI{path, tt.failAdd}, p, io.Discard, func(State) {})
			if s.Phase != tt.phase {
				t.Fatalf("phase %v, %s", s.Phase, s.Err)
			}
			after := mustRead(t, path)
			if !jsonEqual(t, inner(after, []string{"mcpServers", "github"}), before) || !strings.Contains(after, `"numStartups": 3`) {
				t.Errorf("~/.claude.json not back:\n%s", after)
			}
			if rc, _ := os.ReadFile(filepath.Join(home, ".zshrc")); strings.Contains(string(rc), "GITHUB_TOKEN") {
				t.Errorf(".zshrc keeps the variable:\n%s", rc)
			}
			if !slices.Contains(s.Restored, path) {
				t.Errorf("restored %q", s.Restored)
			}
		})
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal([]byte(a), &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(b), &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}

// The API key leaves Claude Code's settings for .bashrc; the settings that
// are no secret stay.
func TestSettingsSecretPlan(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{
  "model": "opus",
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.example.com",
    "ANTHROPIC_AUTH_TOKEN": "sk-s3cr3t"
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/usr/bin/bash"}
	names, err := ai.SettingsSecrets(settings)
	if err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".bashrc")
	a := inventory.Agent{ID: "claude", Settings: settings, PlainEnv: names}
	p, err := SettingsSecretPlan(a, env, secretTerminal(env, rc), title)
	if err != nil {
		t.Fatal(err)
	}
	if e := p.Steps[0].Edit; e.Path != rc || !e.New || !strings.Contains(e.After, "export ANTHROPIC_AUTH_TOKEN='sk-s3cr3t'") {
		t.Errorf("rc edit %+v", e)
	}
	if after := p.Steps[1].Edit.After; strings.Contains(after, "ANTHROPIC_AUTH_TOKEN") || !strings.Contains(after, "ANTHROPIC_BASE_URL") {
		t.Errorf("settings:\n%s", after)
	}
	runEdits(t, p)

	// The last key takes "env" with it.
	if err := os.WriteFile(settings, []byte(`{"model": "opus", "env": {"ANTHROPIC_API_KEY": "sk-s3cr3t"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.PlainEnv = []string{"ANTHROPIC_API_KEY"}
	p, err = SettingsSecretPlan(a, env, nil, title)
	if err != nil {
		t.Fatal(err)
	}
	if after := p.Steps[len(p.Steps)-1].Edit.After; strings.Contains(after, "env") || !strings.Contains(after, "opus") {
		t.Errorf("settings:\n%s", after)
	}
}

func TestShQuote(t *testing.T) {
	if got := shQuote(`a'b$c`); got != `'a'\''b$c'` {
		t.Errorf("shQuote: %s", got)
	}
	if got := fishQuote(`a'b\c`); got != `'a\'b\\c'` {
		t.Errorf("fishQuote: %s", got)
	}
}
