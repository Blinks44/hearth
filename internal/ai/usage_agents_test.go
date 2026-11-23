package ai

import (
	"database/sql"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestUsageSupport(t *testing.T) {
	supported := []string{"claude", "codex", "gemini", "copilot", "kimi", "grok", "opencode", "devin", "antigravity"}
	for _, id := range supported {
		if !UsageSupport(id) {
			t.Errorf("UsageSupport(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"cursor", "aider", "unknown"} {
		if UsageSupport(id) {
			t.Errorf("UsageSupport(%q) = true, want false", id)
		}
	}
}

func TestCostProvidersAndNormalization(t *testing.T) {
	tests := []struct {
		u    inventory.TokenUsage
		want float64
		ok   bool
	}{
		{inventory.TokenUsage{Model: "anthropic/claude-sonnet-4.6:thinking", Input: 1e6, Output: 1e6}, 3 + 15, true},
		{inventory.TokenUsage{Model: "openai/gpt-5.4", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 2.5 + 0.25 + 15, true},
		{inventory.TokenUsage{Model: "gpt-5-2025-08-07", Input: 1e6, Output: 1e6}, 1.25 + 10, true},
		{inventory.TokenUsage{Model: "models/gemini-2.5-pro", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 1.25 + 0.125 + 10, true},
		{inventory.TokenUsage{Model: "gemini-3-pro-preview", Input: 1e6, Output: 1e6}, 2 + 12, true},
		{inventory.TokenUsage{Model: "gemini-4-neon", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 3 + 0.15 + 15, true},
		{inventory.TokenUsage{Model: "claude-opus-5-5-medium", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 4 + 0.2 + 20, true},
		{inventory.TokenUsage{Model: "gpt-6.2-sol", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 2 + 0.1 + 10, true},
		{inventory.TokenUsage{Model: "gpt-6.2-astra", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 8 + 0.4 + 40, true},
		{inventory.TokenUsage{Model: "gpt-6-astra", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 10 + 1 + 50, true},
		{inventory.TokenUsage{Model: "xai/grok-4-0709", Input: 1e6, Output: 1e6}, 3 + 15, true},
		{inventory.TokenUsage{Model: "moonshot/kimi-k2.5", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 0.6 + 0.1 + 3, true},
		{inventory.TokenUsage{Model: "deepseek-chat", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 0.28 + 0.028 + 0.42, true},
		{inventory.TokenUsage{Model: "deepseek/deepseek-r1", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 0.55 + 0.14 + 2.19, true},
		{inventory.TokenUsage{Model: "qwen/qwen3-coder-plus", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 1 + 0.1 + 5, true},
		{inventory.TokenUsage{Model: "zhipu/glm-4.7", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 0.6 + 0.11 + 2.2, true},
		{inventory.TokenUsage{Model: "us.anthropic.claude-sonnet-4-5-20250929-v1:0", Input: 1e6, CacheRead: 1e6, Output: 1e6}, 3 + 0.3 + 15, true},
		{inventory.TokenUsage{Model: "eu.anthropic.claude-3-5-sonnet-20241022-v2:0", Input: 1e6, Output: 1e6}, 3 + 15, true},
		{inventory.TokenUsage{Model: "publishers/anthropic/models/claude-sonnet-4-5@20250929", Input: 1e6, Output: 1e6}, 3 + 15, true},
		{inventory.TokenUsage{Model: "claude-3-5-sonnet-v2@20241022", Input: 1e6, Output: 1e6}, 3 + 15, true},
		{inventory.TokenUsage{Model: "codex-mini-latest", Input: 1e6}, 1.5, true},
		{inventory.TokenUsage{Model: "codex-auto-review", Input: 1e6}, 0, false},
	}
	for _, tt := range tests {
		got, ok := Cost(tt.u)
		if ok != tt.ok || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Cost(%s) = %v, %v; want %v, %v", tt.u.Model, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSessionsGemini(t *testing.T) {
	home := t.TempDir()
	tree(t, home, map[string]string{
		".gemini/projects.json": `{"projects":{"/Users/dev/app":"app-slug"}}`,
		".gemini/tmp/app-slug/chats/session-1.json": `{
			"sessionId": "g-1",
			"startTime": "2026-11-10T10:00:00Z",
			"messages": [
				{"id":"m1","timestamp":"2026-11-10T10:01:00Z","type":"gemini","model":"gemini-2.5-pro",
				 "tokens":{"input":1000,"output":200,"cached":400,"thoughts":50}}
			]
		}`,
		".gemini/tmp/other/.project_root": "/Users/dev/other\n",
		".gemini/tmp/other/chats/session-2.jsonl": `{"sessionId":"g-2","startTime":"2026-11-11T10:00:00Z"}
{"id":"m1","timestamp":"2026-11-11T10:01:00Z","type":"gemini","model":"gemini-3-flash","tokens":{"input":500,"output":10,"cached":100}}
{"id":"m1","timestamp":"2026-11-11T10:01:05Z","type":"gemini","model":"gemini-3-flash","tokens":{"input":500,"output":80,"cached":100,"thoughts":20}}
`,
	}, nil)

	got := geminiSessions(filepath.Join(home, ".gemini"), time.UTC)
	if len(got) != 2 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	byID := map[string]inventory.AgentSession{}
	for _, s := range got {
		byID[s.ID] = s
	}
	s1 := byID["g-1"]
	if s1.Agent != "gemini" || s1.Dir != "/Users/dev/app" || len(s1.Files) != 1 {
		t.Errorf("g-1 = %+v", s1)
	}
	want1 := []inventory.TokenUsage{{Day: "2026-11-10", Model: "gemini-2.5-pro", Input: 600, CacheRead: 400, Output: 250}}
	if !reflect.DeepEqual(s1.Usage, want1) {
		t.Errorf("g-1 usage = %+v, want %+v", s1.Usage, want1)
	}
	s2 := byID["g-2"]
	if s2.Dir != "/Users/dev/other" {
		t.Errorf("g-2 dir = %q", s2.Dir)
	}
	// Streaming update with same id replaces the earlier entry.
	want2 := []inventory.TokenUsage{{Day: "2026-11-11", Model: "gemini-3-flash", Input: 400, CacheRead: 100, Output: 100}}
	if !reflect.DeepEqual(s2.Usage, want2) {
		t.Errorf("g-2 usage = %+v, want %+v", s2.Usage, want2)
	}
}

func TestSessionsCopilot(t *testing.T) {
	home := t.TempDir()
	tree(t, home, map[string]string{
		".copilot/session-state/sess-turns/workspace.yaml": "cwd: /Users/dev/api\n",
		".copilot/session-state/sess-turns/events.jsonl": `{"type":"session.start","timestamp":"2026-11-10T10:00:00Z","data":{"sessionId":"cp-1"}}
{"type":"session.usage_record","timestamp":"2026-11-10T10:01:00Z","data":{"model":"claude-sonnet-4.5","inputTokens":1200,"outputTokens":300,"cacheReadTokens":400,"cacheWriteTokens":100}}
{"type":"session.shutdown","timestamp":"2026-11-10T10:05:00Z","data":{"modelMetrics":{"claude-sonnet-4.5":{"usage":{"inputTokens":9999,"outputTokens":9999}}}}}
`,
		".copilot/session-state/sess-shutdown/events.jsonl": `{"type":"session.start","timestamp":"2026-11-10T11:00:00Z","data":{"sessionId":"cp-2","context":{"cwd":"/Users/dev/web"}}}
{"type":"session.shutdown","timestamp":"2026-11-10T11:10:00Z","data":{"modelMetrics":{"gpt-5":{"usage":{"inputTokens":500,"outputTokens":100}}}}}
{"type":"session.shutdown","timestamp":"2026-11-11T11:10:00Z","data":{"modelMetrics":{"gpt-5":{"usage":{"inputTokens":800,"outputTokens":160}}}}}
`,
	}, nil)

	got := copilotSessions(filepath.Join(home, ".copilot"), time.UTC)
	if len(got) != 2 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	byID := map[string]inventory.AgentSession{}
	for _, s := range got {
		byID[s.ID] = s
	}
	// Per-turn usage_record wins over shutdown summary.
	s1 := byID["cp-1"]
	want1 := []inventory.TokenUsage{{Day: "2026-11-10", Model: "claude-sonnet-4.5", Input: 1200, CacheRead: 400, CacheWrite: 100, Output: 300}}
	if s1.Dir != "/Users/dev/api" || !reflect.DeepEqual(s1.Usage, want1) {
		t.Errorf("cp-1 = %+v", s1)
	}
	// Shutdown fallback computes delta across resumes.
	s2 := byID["cp-2"]
	want2 := []inventory.TokenUsage{
		{Day: "2026-11-10", Model: "gpt-5", Input: 500, Output: 100},
		{Day: "2026-11-11", Model: "gpt-5", Input: 300, Output: 60},
	}
	if s2.Dir != "/Users/dev/web" || !reflect.DeepEqual(s2.Usage, want2) {
		t.Errorf("cp-2 = %+v, want %+v", s2, want2)
	}
}

func TestSessionsKimi(t *testing.T) {
	home := t.TempDir()
	ts := float64(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC).Unix())
	tree(t, home, map[string]string{
		".kimi-code/session_index.jsonl":            `{"session_id":"k-1","work_dir":"/Users/dev/kimi-app"}` + "\n",
		".kimi-code/sessions/ws1/k-1/metadata.json": `{"session_id":"k-1","model":"kimi-k2.5"}`,
		".kimi-code/sessions/ws1/k-1/wire.jsonl": `{"timestamp":` + formatFloat(ts) + `,"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":700,"input_cache_read":200,"input_cache_creation":50,"output":120}}}}
{"timestamp":` + formatFloat(ts) + `,"message":{"type":"StatusUpdate","payload":{"scope":"session","token_usage":{"input_other":700,"output":120}}}}
`,
		".kimi-code/sessions/ws1/k-1/agents/sub1/wire.jsonl": `{"timestamp":` + formatFloat(ts) + `,"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":300,"output":80}}}}` + "\n",
	}, nil)

	got := kimiSessions(filepath.Join(home, ".kimi-code"), time.UTC)
	if len(got) != 1 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	want := []inventory.TokenUsage{{Day: "2026-11-12", Model: "kimi-k2.5", Input: 1000, CacheRead: 200, CacheWrite: 50, Output: 200}}
	if got[0].Agent != "kimi" || got[0].Dir != "/Users/dev/kimi-app" || !reflect.DeepEqual(got[0].Usage, want) {
		t.Errorf("kimi = %+v, want %+v", got[0], want)
	}
}

func TestSessionsGrok(t *testing.T) {
	home := t.TempDir()
	tree(t, home, map[string]string{
		".grok/sessions/proj-a/.cwd": "/Users/dev/grok-a\n",
		".grok/sessions/proj-a/s1/usage.json": `{
			"turns": [
				{"timestamp":"2026-11-12T10:00:00Z","model":"grok-4","input_tokens":1000,"cached_tokens":300,"output_tokens":150,"reasoning_tokens":50}
			]
		}`,
		".grok/sessions/proj-b/s2/summary.json":  `{"working_dir":"/Users/dev/grok-b"}`,
		".grok/sessions/proj-b/s2/updates.jsonl": `{"type":"turn_completed","timestamp":"2026-11-13T10:00:00Z","model":"grok-code-fast-1","usage":{"input_tokens":400,"cached_tokens":100,"output_tokens":90}}` + "\n",
	}, nil)

	got := grokSessions(filepath.Join(home, ".grok"), time.UTC)
	if len(got) != 2 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	byID := map[string]inventory.AgentSession{}
	for _, s := range got {
		byID[s.ID] = s
	}
	want1 := []inventory.TokenUsage{{Day: "2026-11-12", Model: "grok-4", Input: 700, CacheRead: 300, Output: 200}}
	if byID["s1"].Dir != "/Users/dev/grok-a" || !reflect.DeepEqual(byID["s1"].Usage, want1) {
		t.Errorf("s1 = %+v", byID["s1"])
	}
	want2 := []inventory.TokenUsage{{Day: "2026-11-13", Model: "grok-code-fast-1", Input: 300, CacheRead: 100, Output: 90}}
	if byID["s2"].Dir != "/Users/dev/grok-b" || !reflect.DeepEqual(byID["s2"].Usage, want2) {
		t.Errorf("s2 = %+v", byID["s2"])
	}
}

func TestSessionsOpenCodeSQLiteAndLegacy(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, "opencode-data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC).UnixMilli()
	t2 := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC).UnixMilli()
	schema := `
		CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, time_updated INTEGER);
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, time_created INTEGER, data TEXT);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO session VALUES ('s1', '/Users/dev/oc1', ?), ('s2', '/Users/dev/oc2', ?)`, t1, t2)
	if err != nil {
		t.Fatal(err)
	}
	msg1 := `{"role":"assistant","modelID":"claude-sonnet-4-6","tokens":{"input":500,"output":100,"reasoning":40,"cache":{"read":200,"write":50}}}`
	msg2 := `{"role":"assistant","modelID":"gpt-5","tokens":{"input":300,"output":80}}`
	msgUser := `{"role":"user","modelID":"gpt-5","tokens":{"input":999,"output":999}}`
	_, err = db.Exec(`INSERT INTO message VALUES ('m1','s1',?,?), ('m2','s2',?,?), ('m3','s2',?,?)`, t1, msg1, t2, msg2, t2, msgUser)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	got := opencodeSessions(sysenv.Env{"OPENCODE_DATA=" + dataDir}, home, time.UTC)
	if len(got) != 2 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	var nonZeroSize int
	for _, s := range got {
		if len(s.Files) != 0 {
			t.Errorf("SQLite session %s must have empty Files, got %v", s.ID, s.Files)
		}
		if s.Size > 0 {
			nonZeroSize++
		}
	}
	if nonZeroSize != 1 {
		t.Errorf("shared DB size attached to %d sessions, want 1", nonZeroSize)
	}
	if got[0].ID != "s1" || got[0].Dir != "/Users/dev/oc1" || got[0].StoreDir != dataDir || got[0].Usage[0].Output != 140 || got[0].Usage[0].CacheRead != 200 {
		t.Errorf("s1 = %+v (want StoreDir %s)", got[0], dataDir)
	}

	// Legacy JSON storage fallback when opencode.db is absent.
	legacyHome := t.TempDir()
	tree(t, legacyHome, map[string]string{
		".local/share/opencode/storage/session/p1/leg1.json": `{"id":"leg1","directory":"/Users/dev/legacy","time":{"updated":` + formatFloat(float64(t2)) + `}}`,
		".local/share/opencode/storage/message/leg1/m1.json": `{"role":"assistant","modelID":"gemini-2.5-pro","time":{"created":` + formatFloat(float64(t2)) + `},"tokens":{"input":600,"output":90}}`,
	}, nil)
	leg := opencodeSessions(sysenv.Env{"HOME=" + legacyHome}, legacyHome, time.UTC)
	wantLegacyStore := filepath.Join(legacyHome, ".local", "share", "opencode", "storage")
	if len(leg) != 1 || leg[0].Dir != "/Users/dev/legacy" || leg[0].StoreDir != wantLegacyStore || leg[0].Usage[0].Input != 600 {
		t.Errorf("legacy = %+v (want StoreDir %s)", leg, wantLegacyStore)
	}
}

func TestSessionsDevinSQLite(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "share", "devin", "cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "sessions.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT, updated_at TEXT);
		CREATE TABLE message_nodes (id TEXT PRIMARY KEY, session_id TEXT, created_at TEXT, chat_message TEXT);
		INSERT INTO sessions VALUES ('d1', '/Users/dev/devin-proj', '2026-11-12T10:00:00Z');
		INSERT INTO message_nodes VALUES
			('n1', 'd1', '2026-11-12T10:01:00Z', '{"request_id":"req-1","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":20}}'),
			('n2', 'd1', '2026-11-12T10:01:02Z', '{"request_id":"req-1","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":90,"cache_read_input_tokens":50}}'),
			('n3', 'd1', '2026-11-12T10:02:00Z', '{"request_id":"req-2","model":"gpt-5.4","usage":{"prompt_tokens":1000,"completion_tokens":200,"prompt_tokens_details":{"cached_tokens":400}}}'),
			('n4', 'd1', '2026-11-12T10:03:00Z', '{"request_id":"req-3","model":"gpt-5.4","usage":{"input_tokens":500,"output_tokens":100,"input_tokens_details":{"cached_tokens":200}}}'),
			('n5', 'd1', '2026-11-12T10:04:00Z', '{"request_id":"req-4","model":"gpt-5.4","usage":{"prompt_tokens":300,"completion_tokens":50,"cache_read_tokens":100}}');
	`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	got := devinSessions(sysenv.Env{"HOME=" + home}, home, time.UTC)
	if len(got) != 1 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	want := []inventory.TokenUsage{
		{Day: "2026-11-12", Model: "claude-opus-4-6", Input: 100, CacheRead: 50, Output: 90},
		// n3: 600+400+200, n4: 300+200+100, n5: 200+100+50 => Input=1100, CacheRead=700, Output=350
		{Day: "2026-11-12", Model: "gpt-5.4", Input: 1100, CacheRead: 700, Output: 350},
	}
	if got[0].Agent != "devin" || got[0].Dir != "/Users/dev/devin-proj" || got[0].StoreDir != dir || len(got[0].Files) != 0 || got[0].Size == 0 || !reflect.DeepEqual(got[0].Usage, want) {
		t.Errorf("devin = %+v, want %+v (StoreDir %s)", got[0], want, dir)
	}
}

func TestSessionsAntigravityProtobufSQLite(t *testing.T) {
	home := t.TempDir()
	convDir := filepath.Join(home, ".gemini", "antigravity-cli", "conversations")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(convDir, "conv-42.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Build trajectory_metadata_blob: field 7 = "/Users/dev/agy-proj"
	trajBlob := protoBytes(7, []byte("/Users/dev/agy-proj"))
	// Build gen_metadata:
	//   f1 (chat_model):
	//     f4 (usage): f2=1200 (in), f3=300 (out), f4=50 (cw), f5=400 (cr), f11="resp-1"
	//     f9 (timeWrap): f4=unix(2026-11-13)
	//     f19="gemini-2.5-pro"
	ts := uint64(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC).Unix())
	usage1 := append(append(append(append(protoVarint(2, 1200), protoVarint(3, 100)...), protoVarint(4, 50)...), protoVarint(5, 400)...), protoBytes(11, []byte("resp-1"))...)
	usage2 := append(append(append(append(protoVarint(2, 1200), protoVarint(3, 300)...), protoVarint(4, 50)...), protoVarint(5, 400)...), protoBytes(11, []byte("resp-1"))...)
	timeWrap := protoVarint(4, ts)
	chat1 := append(append(protoBytes(4, usage1), protoBytes(9, timeWrap)...), protoBytes(19, []byte("gemini-2.5-pro"))...)
	chat2 := append(append(protoBytes(4, usage2), protoBytes(9, timeWrap)...), protoBytes(19, []byte("gemini-2.5-pro"))...)
	gen1 := protoBytes(1, chat1)
	gen2 := protoBytes(1, chat2)

	_, err = db.Exec(`
		CREATE TABLE metadata (trajectory_metadata_blob BLOB);
		CREATE TABLE steps (id INTEGER PRIMARY KEY, gen_metadata BLOB);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO metadata VALUES (?)`, trajBlob); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO steps (gen_metadata) VALUES (?), (?)`, gen1, gen2); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	got := antigravitySessions(home, time.UTC)
	if len(got) != 1 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	want := []inventory.TokenUsage{{Day: "2026-11-13", Model: "gemini-2.5-pro", Input: 1200, CacheWrite: 50, CacheRead: 400, Output: 300}}
	if got[0].Agent != "antigravity" || got[0].ID != "conv-42" || got[0].Dir != "/Users/dev/agy-proj" || got[0].StoreDir != convDir ||
		!reflect.DeepEqual(got[0].Files, []string{dbPath}) || got[0].Size == 0 || !reflect.DeepEqual(got[0].Usage, want) {
		t.Errorf("antigravity = %+v, want %+v (files [%s], StoreDir %s)", got[0], want, dbPath, convDir)
	}
}

func TestSessionsAntigravityACPTableWAL(t *testing.T) {
	home := t.TempDir()
	convDir := filepath.Join(home, ".gemini", "antigravity-acp", "conversations")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(convDir, "acp-session.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Real Antigravity DBs use WAL mode and separate tables trajectory_metadata_blob
	// and gen_metadata with a data BLOB column; cleanly closed WAL DBs have no -shm.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE trajectory_metadata_blob (id INTEGER PRIMARY KEY, data BLOB);
		CREATE TABLE gen_metadata (idx INTEGER PRIMARY KEY, data BLOB, size INTEGER);
	`)
	if err != nil {
		t.Fatal(err)
	}

	trajBlob := protoBytes(7, []byte("file:///Users/dev/acp-proj"))
	ts := uint64(time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC).Unix())
	tsMsg := protoVarint(1, ts)
	timeWrap := protoBytes(4, tsMsg)

	// Two turns in the same execution share top-level field 4 (execution_id) and have
	// no usage.f11 response_id; both turns must be summed rather than overwriting.
	usage1 := append(protoVarint(2, 1000), protoVarint(3, 200)...)
	usage2 := append(append(protoVarint(2, 500), protoVarint(3, 150)...), protoVarint(5, 900)...)
	chat1 := append(append(protoBytes(4, usage1), protoBytes(9, timeWrap)...), protoBytes(19, []byte("gemini-4-neon"))...)
	chat2 := append(append(protoBytes(4, usage2), protoBytes(9, timeWrap)...), protoBytes(19, []byte("gemini-4-neon"))...)
	gen1 := append(protoBytes(1, chat1), protoBytes(4, []byte("exec-shared"))...)
	gen2 := append(protoBytes(1, chat2), protoBytes(4, []byte("exec-shared"))...)

	if _, err := db.Exec(`INSERT INTO trajectory_metadata_blob (id, data) VALUES (1, ?)`, trajBlob); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO gen_metadata (idx, data, size) VALUES (0, ?, ?), (1, ?, ?)`, gen1, len(gen1), gen2, len(gen2)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Ensure no WAL/SHM sidecars exist (cleanly closed WAL DB).
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	got := antigravitySessions(home, time.UTC)
	if len(got) != 1 {
		t.Fatalf("got %d sessions: %+v", len(got), got)
	}
	want := []inventory.TokenUsage{{Day: "2026-11-21", Model: "gemini-4-neon", Input: 1500, CacheRead: 900, Output: 350}}
	if got[0].Agent != "antigravity" || got[0].ID != "acp-session" || got[0].Dir != "/Users/dev/acp-proj" ||
		!reflect.DeepEqual(got[0].Files, []string{dbPath}) || !reflect.DeepEqual(got[0].Usage, want) {
		t.Errorf("antigravity acp = %+v, want %+v", got[0], want)
	}
	// When -wal and -shm sidecars exist alongside the DB, Files includes them too.
	if err := os.WriteFile(dbPath+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-shm", []byte("shm"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotWAL := antigravitySessions(home, time.UTC)
	wantFiles := []string{dbPath, dbPath + "-wal", dbPath + "-shm"}
	if len(gotWAL) != 1 || !reflect.DeepEqual(gotWAL[0].Files, wantFiles) {
		t.Errorf("antigravity acp with sidecars = %+v, want files %q", gotWAL, wantFiles)
	}
}

func TestExtractPathStringWindowsURI(t *testing.T) {
	if got := extractPathString([]byte("file:///C:/Users/dev/app")); got != "C:/Users/dev/app" {
		t.Errorf("got %q, want C:/Users/dev/app", got)
	}
	if got := extractPathString([]byte("file:///Users/dev/app")); got != "/Users/dev/app" {
		t.Errorf("got %q, want /Users/dev/app", got)
	}
}

func TestUsageDiskCacheRoundTrip(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "usage-cache.json")
	if err := ClearUsageCache(cachePath); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, home, "codex.jsonl", ".codex/sessions/rollout-cache.jsonl")
	filePath := filepath.Join(home, ".codex", "sessions", "rollout-cache.jsonl")
	env := sysenv.Env{"HOME=" + home}

	first := Sessions(env)
	if len(first) != 1 {
		t.Fatalf("first = %+v", first)
	}
	if err := SaveUsageCache(cachePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	// Wipe in-memory cache and reload from disk.
	transcripts.Lock()
	clear(transcripts.files)
	clear(transcripts.dbs)
	transcripts.dirty = false
	transcripts.Unlock()

	if err := LoadUsageCache(cachePath); err != nil {
		t.Fatal(err)
	}
	transcripts.Lock()
	_, hit := transcripts.files[filePath]
	transcripts.Unlock()
	if !hit {
		t.Fatalf("expected %s in reloaded disk cache", filePath)
	}

	// Reading again should hit cache without marking dirty.
	second := Sessions(env)
	if !reflect.DeepEqual(first[0].Usage, second[0].Usage) {
		t.Errorf("cached usage mismatch:\n%+v\nwant\n%+v", second[0].Usage, first[0].Usage)
	}
	transcripts.Lock()
	dirty := transcripts.dirty
	transcripts.Unlock()
	if dirty {
		t.Errorf("expected cache hit without marking dirty")
	}

	if err := ClearUsageCache(cachePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cache file still exists after ClearUsageCache: %v", err)
	}
}

func protoVarint(field uint64, val uint64) []byte {
	var b []byte
	b = binary.AppendUvarint(b, field<<3)
	b = binary.AppendUvarint(b, val)
	return b
}

func protoBytes(field uint64, payload []byte) []byte {
	var b []byte
	b = binary.AppendUvarint(b, (field<<3)|2)
	b = binary.AppendUvarint(b, uint64(len(payload)))
	return append(b, payload...)
}

func formatFloat(v float64) string {
	return time.Unix(0, 0).Format("") + func() string {
		// Simple integer formatting for whole unix timestamps.
		n := int64(v)
		if n == 0 {
			return "0"
		}
		var buf [32]byte
		i := len(buf)
		for n > 0 {
			i--
			buf[i] = byte('0' + n%10)
			n /= 10
		}
		return string(buf[i:])
	}()
}
