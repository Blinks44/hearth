package ai

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// moscow is a zone ahead of UTC, so that a reply just before UTC's
// midnight is the next local day.
var moscow = time.FixedZone("MSK", 3*60*60)

func parseFile(t *testing.T, name string, parse func(*os.File) (transcript, error)) transcript {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "usage", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read only
	tr, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestParseClaude(t *testing.T) {
	tr := parseFile(t, "claude.jsonl", func(f *os.File) (transcript, error) { return parseClaude(f, moscow) })
	if tr.dir != "/Users/user/dev/shop" {
		t.Errorf("dir = %q", tr.dir)
	}
	want := []inventory.TokenUsage{
		// The reply streamed twice counts once, with its last count; its
		// cache write was for an hour.
		{Day: "2026-11-12", Model: "claude-opus-5-5", Input: 2, CacheWrite1h: 18178, CacheRead: 24742, Output: 132},
		// Past local midnight; the synthetic reply is left out.
		{Day: "2026-11-13", Model: "claude-haiku-5-5", Input: 120, Output: 8},
		{Day: "2026-11-13", Model: "claude-opus-5-5", Input: 5, CacheWrite: 300, CacheRead: 42920, Output: 40},
	}
	if got := merge(tr.usage); !reflect.DeepEqual(got, want) {
		t.Errorf("usage =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseCodex(t *testing.T) {
	tr := parseFile(t, "codex.jsonl", func(f *os.File) (transcript, error) { return parseCodex(f, time.UTC) })
	if tr.id != "01a14e51" || tr.dir != "/Users/user/dev/ui" {
		t.Errorf("id, dir = %q, %q", tr.id, tr.dir)
	}
	want := []inventory.TokenUsage{
		// The input Codex counts has the cached tokens in it; a count
		// repeated adds nothing.
		{Day: "2026-11-01", Model: "gpt-6.1-sol", Input: 8000, CacheRead: 2000, Output: 300},
		// The growth to the next count, then a count started over.
		{Day: "2026-11-02", Model: "codex-auto-review", Input: 4000 + 3000, CacheWrite: 1000, CacheRead: 10000 + 1000, Output: 200 + 50},
	}
	if got := merge(tr.usage); !reflect.DeepEqual(got, want) {
		t.Errorf("usage =\n%+v\nwant\n%+v", got, want)
	}
}

// copyFixture puts the fixture name at path under root.
func copyFixture(t *testing.T, root, name, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "usage", name))
	if err != nil {
		t.Fatal(err)
	}
	tree(t, root, map[string]string{path: string(data)}, nil)
}

func TestSessions(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(home, "codex-home")
	copyFixture(t, home, "claude.jsonl", ".claude/projects/-Users-user-dev-shop/0e7c497e.jsonl")
	// A subagent's transcript and a kept tool result beside the session.
	copyFixture(t, home, "claude.jsonl", ".claude/projects/-Users-user-dev-shop/0e7c497e/subagents/agent-1.jsonl")
	tree(t, home, map[string]string{
		".claude/projects/-Users-user-dev-shop/0e7c497e/tool-results/r.txt": "12345",
		".claude/projects/-Users-user-dev-shop/notes.txt":                   "not a session",
		".codex/sessions/2026/11/01/rollout-x.jsonl":                        "{}\n",
	}, nil)
	copyFixture(t, codexHome, "codex.jsonl", "sessions/2026/11/01/rollout-2026-11-01T09-21-54-01a14e51.jsonl")
	copyFixture(t, codexHome, "codex.jsonl", "archived_sessions/rollout-old.jsonl")

	got := Sessions(sysenv.Env{"HOME=" + home, "CODEX_HOME=" + codexHome})
	if len(got) != 3 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	claude := got[0]
	session := filepath.Join(home, ".claude", "projects", "-Users-user-dev-shop", "0e7c497e")
	if claude.Agent != "claude" || claude.ID != "0e7c497e" || claude.Dir != "/Users/user/dev/shop" {
		t.Errorf("claude = %+v", claude)
	}
	if want := []string{session + ".jsonl", session}; !reflect.DeepEqual(claude.Files, want) {
		t.Errorf("files = %q, want %q", claude.Files, want)
	}
	fi, err := os.Stat(session + ".jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := 2*fi.Size() + 5; claude.Size != want {
		t.Errorf("size = %d, want %d", claude.Size, want)
	}
	// The subagent's tokens are the session's.
	if u := claude.Usage[0]; u.Model != "claude-opus-5-5" || u.Output != 2*132 {
		t.Errorf("usage[0] = %+v", u)
	}
	// Codex's folder of CODEX_HOME, both its sessions and archived ones;
	// the home's .codex is not read.
	for _, s := range got[1:] {
		if s.Agent != "codex" || s.ID != "01a14e51" || len(s.Usage) != 2 || !strings.HasPrefix(s.Files[0], codexHome) {
			t.Errorf("codex = %+v", s)
		}
	}
}

func TestSessionsReadAgainWhenChanged(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, ".codex", "sessions", "rollout-a.jsonl")
	copyFixture(t, home, "codex.jsonl", ".codex/sessions/rollout-a.jsonl")
	env := sysenv.Env{"HOME=" + home}
	if got := Sessions(env); len(got) != 1 || len(got[0].Usage) != 2 {
		t.Fatalf("first = %+v", got)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.SplitAfterN(string(data), "\n", 5) // the first count only
	if err := os.WriteFile(file, []byte(strings.Join(cut[:4], "")), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Sessions(env); len(got) != 1 || len(got[0].Usage) != 1 {
		t.Errorf("after the change = %+v", got)
	}
}

func TestCost(t *testing.T) {
	tests := []struct {
		u    inventory.TokenUsage
		want float64
		ok   bool
	}{
		// $4 input, $5 and $8 the cache writes, $0.20 cache reads, $20 output.
		{inventory.TokenUsage{Model: "claude-opus-5-5", Input: 1e6, CacheWrite: 1e6, CacheWrite1h: 1e6, CacheRead: 1e6, Output: 1e6}, 4 + 5 + 8 + 0.2 + 20, true},
		{inventory.TokenUsage{Model: "claude-haiku-4-5-20251001", Input: 2e6}, 2, true},
		{inventory.TokenUsage{Model: "claude-opus-5-5[1m]", Output: 1e5}, 2, true},
		{inventory.TokenUsage{Model: "gpt-6.1-sol", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 2 + 0.1 + 10, true},
		{inventory.TokenUsage{Model: "codex-auto-review", Input: 1e6}, 0, false},
	}
	for _, tt := range tests {
		got, ok := Cost(tt.u)
		if ok != tt.ok || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Cost(%s) = %v, %v; want %v, %v", tt.u.Model, got, ok, tt.want, tt.ok)
		}
	}
}
