//go:build integration && unix

package ops

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// failAddRefs is the real runner, but claude mcp add-json of a server
// that takes a variable fails, as a claude that does not take it would.
type failAddRefs struct{ run.Exec }

func (f failAddRefs) Run(ctx context.Context, c run.Cmd) (run.Result, error) {
	if slices.Contains(c.Args, "add-json") && strings.Contains(c.Args[len(c.Args)-1], "${") {
		return run.Result{ExitCode: 1}, nil
	}
	return f.Exec.Run(ctx, c)
}

// The real claude in a temporary home: a server whose move does not get
// through comes back in ~/.claude.json as it was, through claude mcp.
func TestMCPSecretPlanClaudeIntegration(t *testing.T) {
	gui := sysenv.GUI()
	if len(sysenv.LookPathAll("claude", gui)) == 0 {
		t.Skip("claude not installed")
	}
	for _, tt := range []struct {
		name     string
		runner   run.Runner
		terminal Terminal
		phase    Phase
	}{
		{"add-json fails", failAddRefs{}, nil, Failed},
		{"check fails", run.Exec{}, func(context.Context) (sysenv.Env, error) { return nil, nil }, NotVerified},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			env := slices.DeleteFunc(slices.Clone(gui), func(kv string) bool {
				return strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=")
			})
			env = append(env, "HOME="+home, "SHELL=/bin/zsh")
			add := run.Cmd{Env: env, Args: []string{"claude", "mcp", "add-json", "--scope", "user", "github",
				`{"command":"npx","args":["-y","@modelcontextprotocol/server-github"],"env":{"GITHUB_TOKEN":"` + token + `"}}`}}
			add.Args = fullPath(add.Args, env)
			if res, err := (run.Exec{}).Run(t.Context(), add); err != nil || res.ExitCode != 0 {
				t.Fatalf("claude mcp add-json: %v %s%s", err, res.Stdout, res.Stderr)
			}
			path := filepath.Join(home, ".claude.json")
			before := inner(mustRead(t, path), []string{"mcpServers", "github"})

			p, err := MCPSecretPlan(server(t, env, "claude", "github"), env, tt.terminal, title)
			if err != nil {
				t.Fatal(err)
			}
			p.Env = env
			var journal strings.Builder
			s := Execute(t.Context(), tt.runner, p, &journal, func(State) {})
			if s.Phase != tt.phase {
				t.Fatalf("phase %v %s\n%s", s.Phase, s.Err, journal.String())
			}
			if after := inner(mustRead(t, path), []string{"mcpServers", "github"}); !jsonEqual(t, after, before) {
				t.Errorf("server not back:\n%s\nwas\n%s\n%s", after, before, journal.String())
			}
			if strings.Contains(journal.String(), token) {
				t.Errorf("the journal shows the token:\n%s", journal.String())
			}
			t.Logf("%s", journal.String())
		})
	}
}
