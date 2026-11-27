package ai

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/proposals"
)

func TestMatchAgentProcess(t *testing.T) {
	cases := []struct {
		comm string
		args []string
		want string
	}{
		{"claude", []string{"claude"}, "claude"},
		{"node", []string{"node", "/opt/homebrew/bin/codex"}, "codex"},
		{"node", []string{"node", "/usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js"}, "claude"},
		{"hearth", []string{"hearth", "notify", "--agent", "claude"}, ""},
		{"node", []string{"node", "mcp-server-claude.js"}, ""},
		{"python3", []string{"python3", "/usr/local/bin/aider"}, "aider"},
		{"vim", []string{"vim", "claude.md"}, ""},
		{"codex", []string{"codex", "app-server", "--listen", "stdio://"}, ""},
		{"claude", []string{"claude", "mcp", "serve"}, ""},
		{"node", []string{"node", "/opt/homebrew/bin/codex", "app-server"}, ""},
	}
	for _, tc := range cases {
		if got := matchAgentProcess(tc.comm, tc.args); got != tc.want {
			t.Errorf("matchAgentProcess(%q, %q) = %q, want %q", tc.comm, tc.args, got, tc.want)
		}
	}
}

func TestMergeRadarPrecedenceAndOrdering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hooks := map[string]proposals.AgentEvent{
		"claude:s1": {Agent: "claude", Event: "waiting", SessionID: "s1", Cwd: "/dev/hearth", Timestamp: now.Add(-5 * time.Second).Unix()},
		"codex:s2":  {Agent: "codex", Event: "finished", SessionID: "s2", Cwd: "/dev/api", Timestamp: now.Add(-20 * time.Minute).Unix()}, // expired >15m
	}
	live := []LiveAgentProcess{
		{Agent: "claude", PID: "101", Cwd: "/dev/hearth"},
		{Agent: "gemini", PID: "202", Cwd: "/dev/web"},
	}
	recent := []inventory.AgentSession{
		{
			Agent:    "claude",
			Dir:      "/dev/hearth",
			Modified: now.Add(-10 * time.Second),
			Usage:    []inventory.TokenUsage{{Model: "claude-sonnet-4-5", Input: 1000, Output: 500}},
		},
		{
			Agent:    "gemini",
			Dir:      "/dev/web",
			Modified: now.Add(-2 * time.Minute),
			Usage:    []inventory.TokenUsage{{Model: "gemini-2.5-pro", Input: 400, Output: 100}},
		},
	}

	got := MergeRadar(now, hooks, live, recent)
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %+v", got)
	}
	if got[0].Agent != "claude" || got[0].Status != inventory.AgentStatusWaiting || !got[0].Exact || got[0].PID != "101" || got[0].Tokens != 1500 {
		t.Fatalf("unexpected first session: %+v", got[0])
	}
	if got[1].Agent != "gemini" || got[1].Status != inventory.AgentStatusQuiet || got[1].Exact || got[1].PID != "202" || got[1].Tokens != 500 {
		t.Fatalf("unexpected second session: %+v", got[1])
	}
}

func TestMergeRadarEmptyCwdAndSinglePID(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	// Hook event has cwd, live process has empty cwd (Windows): should dedup into 1 session.
	hooks := map[string]proposals.AgentEvent{
		"claude:s1": {Agent: "claude", Event: "working", SessionID: "s1", Cwd: "/dev/app", Timestamp: now.Add(-2 * time.Second).Unix()},
		"claude:s2": {Agent: "claude", Event: "waiting", SessionID: "s2", Cwd: "", Timestamp: now.Add(-1 * time.Second).Unix()},
	}
	live := []LiveAgentProcess{
		{Agent: "claude", PID: "500", Cwd: ""},
	}
	got := MergeRadar(now, hooks, live, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 hook sessions (no duplicate passive), got %+v", got)
	}
	// Only the fresher hook event (s2) should receive PID 500, never both.
	pidCount := 0
	for _, s := range got {
		if s.PID == "500" {
			pidCount++
		}
	}
	if pidCount != 1 {
		t.Fatalf("expected PID 500 assigned once, got %d in %+v", pidCount, got)
	}
}

func TestMergeRadarFiltersStaleAndDedupsPassive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	live := []LiveAgentProcess{
		{Agent: "codex", PID: "301", Cwd: ""},
		{Agent: "codex", PID: "302", Cwd: ""},
		{Agent: "codex", PID: "303", Cwd: "/dev/hearth"},
		{Agent: "codex", PID: "304", Cwd: "/dev/hearth"},
	}
	recent := []inventory.AgentSession{
		{
			Agent:    "codex",
			Dir:      "",
			Modified: now.Add(-1 * time.Minute),
			Usage:    []inventory.TokenUsage{{Model: "gpt-5", Input: 1000, Output: 500}},
		},
		{
			Agent:    "codex",
			Dir:      "/dev/hearth",
			Modified: now.Add(-24 * time.Hour),
			Usage:    []inventory.TokenUsage{{Model: "gpt-5", Input: 2000, Output: 500}},
		},
	}

	got := MergeRadar(now, nil, live, recent)
	if len(got) != 2 {
		t.Fatalf("expected 2 deduplicated passive rows (empty cwd + /dev/hearth), got %+v", got)
	}
	for _, s := range got {
		if s.Tokens != 0 || s.CostUSD != 0 {
			t.Fatalf("stale or empty-dir usage should not attach to passive process: %+v", s)
		}
		if !s.UpdatedAt.Equal(now) {
			t.Fatalf("expected fresh now timestamp without recent usage, got %v in %+v", s.UpdatedAt, s)
		}
	}
}
