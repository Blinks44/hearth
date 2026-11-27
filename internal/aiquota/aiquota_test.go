package aiquota

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type fakeRunner struct {
	fn func(ctx context.Context, cmd run.Cmd) (run.Result, error)
}

func (f fakeRunner) Run(ctx context.Context, cmd run.Cmd) (run.Result, error) {
	if f.fn == nil {
		return run.Result{ExitCode: 1}, os.ErrNotExist
	}
	return f.fn(ctx, cmd)
}

func makeTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return hdr + "." + payload + ".sig"
}

func TestQuotaWindowPacingMath(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		win          QuotaWindow
		wantLeft     float64
		wantLow      bool
		wantExhaust  bool
		wantTimeLeft float64
		wantHasTime  bool
		wantPace     PaceKind
		wantRunsOut  bool
	}{
		{
			name: "spare pace",
			win: QuotaWindow{
				Kind:     WindowSession,
				Used:     28,
				ResetsAt: now.Add(2 * time.Hour),
				Duration: 5 * time.Hour,
			},
			wantLeft:     72,
			wantLow:      false,
			wantExhaust:  false,
			wantTimeLeft: 40,
			wantHasTime:  true,
			wantPace:     PaceSpare,
			wantRunsOut:  false,
		},
		{
			name: "normal pace",
			win: QuotaWindow{
				Kind:     WindowSession,
				Used:     62,
				ResetsAt: now.Add(2 * time.Hour),
				Duration: 5 * time.Hour,
			},
			wantLeft:     38,
			wantLow:      false,
			wantExhaust:  false,
			wantTimeLeft: 40,
			wantHasTime:  true,
			wantPace:     PaceNormal,
			wantRunsOut:  true,
		},
		{
			name: "fast pace projects runs out before reset",
			win: QuotaWindow{
				Kind:     WindowWeekly,
				Used:     62,
				ResetsAt: now.Add(4 * 24 * time.Hour),
				Duration: 7 * 24 * time.Hour,
			},
			wantLeft:     38,
			wantLow:      false,
			wantExhaust:  false,
			wantTimeLeft: 4.0 / 7.0 * 100.0,
			wantHasTime:  true,
			wantPace:     PaceFast,
			wantRunsOut:  true,
		},
		{
			name: "running low below 10%",
			win: QuotaWindow{
				Kind:     WindowSession,
				Used:     94,
				ResetsAt: now.Add(100 * time.Minute),
				Duration: 5 * time.Hour,
			},
			wantLeft:     6,
			wantLow:      true,
			wantExhaust:  false,
			wantTimeLeft: 100.0 / 300.0 * 100.0,
			wantHasTime:  true,
			wantPace:     PaceLow,
			wantRunsOut:  true,
		},
		{
			name: "exhausted at 100% used",
			win: QuotaWindow{
				Kind:     WindowSession,
				Used:     100,
				ResetsAt: now.Add(30 * time.Minute),
				Duration: 5 * time.Hour,
			},
			wantLeft:     0,
			wantLow:      true,
			wantExhaust:  true,
			wantTimeLeft: 10,
			wantHasTime:  true,
			wantPace:     PaceExhausted,
			wantRunsOut:  false,
		},
		{
			name: "already reset treats window as recovered",
			win: QuotaWindow{
				Kind:     WindowSession,
				Used:     99,
				ResetsAt: now.Add(-1 * time.Minute),
				Duration: 5 * time.Hour,
			},
			wantLeft:     1,
			wantLow:      false,
			wantExhaust:  false,
			wantTimeLeft: 0,
			wantHasTime:  true,
			wantPace:     PaceSpare,
			wantRunsOut:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.win.Left(); got != tc.wantLeft {
				t.Errorf("Left() = %.2f, want %.2f", got, tc.wantLeft)
			}
			if got := tc.win.IsRunningLow(now); got != tc.wantLow {
				t.Errorf("IsRunningLow() = %v, want %v", got, tc.wantLow)
			}
			if got := tc.win.IsExhausted(now); got != tc.wantExhaust {
				t.Errorf("IsExhausted() = %v, want %v", got, tc.wantExhaust)
			}
			tl, ok := tc.win.TimeLeft(now)
			if ok != tc.wantHasTime {
				t.Fatalf("TimeLeft ok = %v, want %v", ok, tc.wantHasTime)
			}
			if ok && (tl < tc.wantTimeLeft-0.5 || tl > tc.wantTimeLeft+0.5) {
				t.Errorf("TimeLeft() = %.2f, want ~%.2f", tl, tc.wantTimeLeft)
			}
			if got := tc.win.PaceStatus(now); got != tc.wantPace {
				t.Errorf("PaceStatus() = %q, want %q", got, tc.wantPace)
			}
			ro := tc.win.RunsOutAt(now)
			if !ro.IsZero() != tc.wantRunsOut {
				t.Errorf("RunsOutAt() = %v (non-zero=%v), want non-zero=%v", ro, !ro.IsZero(), tc.wantRunsOut)
			}
		})
	}
}

func TestFetchClaudeCodexGeminiAntigravityCursorCopilotJetBrains(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	home := t.TempDir()

	// 1. Claude credentials file
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	claudeCred := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":      "claude-secret-tok",
			"expiresAt":        now.Add(2 * time.Hour).UnixMilli(),
			"subscriptionType": "max",
		},
	}
	writeJSONFile(t, filepath.Join(home, ".claude", ".credentials.json"), claudeCred)

	// 2. Codex auth.json
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	codexJWT := makeTestJWT(t, map[string]any{"exp": now.Add(2 * time.Hour).Unix()})
	writeJSONFile(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"tokens": map[string]any{
			"access_token": codexJWT,
			"account_id":   "acct-123",
		},
	})

	// 3. Gemini oauth_creds.json
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(home, ".gemini", "oauth_creds.json"), map[string]any{
		"access_token": "gemini-tok",
		"expiry_date":  now.Add(2 * time.Hour).UnixMilli(),
	})

	// 4. Antigravity credentials fallback file
	if err := os.MkdirAll(filepath.Join(home, ".antigravity"), 0o700); err != nil {
		t.Fatal(err)
	}
	agRaw, _ := json.Marshal(map[string]any{
		"token": map[string]any{
			"access_token": "ag-tok",
			"expiry":       now.Add(2 * time.Hour).Format(time.RFC3339),
		},
	})
	if err := os.WriteFile(filepath.Join(home, ".antigravity", "credentials.json"),
		[]byte("go-keyring-base64:"+base64.StdEncoding.EncodeToString(agRaw)), 0o600); err != nil {
		t.Fatal(err)
	}

	// 5. Cursor SQLite state.vscdb
	cursorDB := cursorStateDBPath(home, nil)
	if err := os.MkdirAll(filepath.Dir(cursorDB), 0o700); err != nil {
		t.Fatal(err)
	}
	cursorJWT := makeTestJWT(t, map[string]any{
		"sub": "auth0|user_42",
		"exp": now.Add(2 * time.Hour).Unix(),
	})
	createTestItemTableDB(t, cursorDB, map[string]string{
		"cursorAuth/accessToken": cursorJWT,
	})

	// 6. JetBrains XML
	jbRoots := jetBrainsConfigRoots(home, nil)
	jbOpts := filepath.Join(jbRoots[0], "GoLand2026.1", "options")
	if err := os.MkdirAll(jbOpts, 0o700); err != nil {
		t.Fatal(err)
	}
	jbXML := `<application><component name="AIAssistantQuotaManager2">
<option name="quotaInfo" value="{&quot;type&quot;:&quot;pro&quot;,&quot;current&quot;:&quot;450&quot;,&quot;maximum&quot;:&quot;1000&quot;,&quot;until&quot;:&quot;2026-12-10T00:00:00Z&quot;}" />
<option name="nextRefill" value="{&quot;next&quot;:&quot;2026-12-10T00:00:00Z&quot;}" />
</component></application>`
	if err := os.WriteFile(filepath.Join(jbOpts, "AIAssistantQuotaManager2.xml"), []byte(jbXML), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/usage":
			if r.Header.Get("Authorization") != "Bearer claude-secret-tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{
				"five_hour": {"utilization": 28, "resets_at": "2026-11-27T14:15:00Z"},
				"seven_day": {"utilization": 59, "resets_at": "2026-12-03T10:00:00Z"}
			}`))
		case "/backend-api/wham/usage":
			if r.Header.Get("ChatGPT-Account-Id") != "acct-123" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{
				"plan_type": "pro",
				"rate_limit": {
					"primary_window": {"used_percent": 16, "limit_window_seconds": 18000, "reset_at": 1795709400},
					"secondary_window": {"used_percent": 38, "limit_window_seconds": 604800, "reset_at": 1796054400}
				}
			}`))
		case "/v1internal:loadCodeAssist":
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-1","currentTier":{"name":"pro"}}`))
		case "/v1internal:retrieveUserQuota":
			_, _ = w.Write([]byte(`{"buckets":[
				{"modelId":"gemini-2.5-pro","remainingFraction":0.65,"resetTime":"2026-11-28T00:00:00Z"},
				{"modelId":"gemini-2.5-flash","remainingFraction":0.92,"resetTime":"2026-11-28T00:00:00Z"}
			]}`))
		case "/v1internal:retrieveUserQuotaSummary":
			_, _ = w.Write([]byte(`{"tier":"pro","groups":[
				{"name":"Gemini","buckets":[
					{"bucketId":"gemini-5h","remainingFraction":0.52,"resetTime":"2026-11-27T15:10:00Z","windowDurationSeconds":18000},
					{"bucketId":"gemini-weekly","remainingFraction":0.74,"resetTime":"2026-12-02T12:00:00Z","windowDurationSeconds":604800}
				]},
				{"name":"Claude","buckets":[
					{"bucketId":"3p-5h","remainingFraction":0.06,"resetTime":"2026-11-27T13:40:00Z","windowDurationSeconds":18000},
					{"bucketId":"3p-weekly","remainingFraction":0.31,"resetTime":"2026-12-02T12:00:00Z","windowDurationSeconds":604800}
				]}
			]}`))
		case "/api/usage-summary":
			if !strings.Contains(r.Header.Get("Cookie"), "WorkosCursorSessionToken=user_42%3A%3A") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{
				"membershipType": "pro",
				"billingCycleStart": "2026-11-09T00:00:00Z",
				"billingCycleEnd": "2026-12-09T00:00:00Z",
				"individualUsage": {"plan": {"used": 1240, "limit": 2000, "totalPercentUsed": 62}}
			}`))
		case "/copilot_internal/user":
			if r.Header.Get("Authorization") != "token gho_test_token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{
				"copilot_plan": "individual_pro",
				"quota_reset_date_utc": "2026-12-01T00:00:00Z",
				"quota_snapshots": {
					"premium_interactions": {"percent_remaining": 82.5, "unlimited": false}
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	runner := fakeRunner{
		fn: func(_ context.Context, cmd run.Cmd) (run.Result, error) {
			if len(cmd.Args) >= 3 && cmd.Args[0] == "gh" && cmd.Args[1] == "auth" && cmd.Args[2] == "token" {
				// Verify GH_TOKEN override was stripped.
				if sysenv.Env(cmd.Env).Get("GH_TOKEN") != "" {
					t.Errorf("GH_TOKEN must be stripped before calling gh auth token")
				}
				return run.Result{Stdout: []byte("gho_test_token\n"), ExitCode: 0}, nil
			}
			return run.Result{ExitCode: 1}, os.ErrNotExist
		},
	}

	fenv := FetchEnv{
		Home:   home,
		Env:    sysenv.Env{"GH_TOKEN=should-be-stripped"},
		Runner: runner,
		HTTP:   srv.Client(),
		Now:    func() time.Time { return now },
		Endpoints: map[ProviderID]string{
			ProviderClaude:      srv.URL,
			ProviderCodex:       srv.URL,
			ProviderGemini:      srv.URL,
			ProviderAntigravity: srv.URL,
			ProviderCursor:      srv.URL,
			ProviderCopilot:     srv.URL,
		},
	}

	snap := FetchAll(context.Background(), fenv)
	byID := map[ProviderID]ProviderQuota{}
	for _, p := range snap.Providers {
		byID[p.Provider] = p
	}

	for _, id := range []ProviderID{
		ProviderClaude, ProviderCodex, ProviderAntigravity,
		ProviderGemini, ProviderCursor, ProviderCopilot, ProviderJetBrains,
	} {
		p := byID[id]
		if p.Status != StatusOK {
			t.Errorf("%s: status = %q (err=%q), want ok", id, p.Status, p.Err)
		}
		if len(p.Windows) == 0 {
			t.Errorf("%s: expected windows, got 0", id)
		}
	}

	if got := byID[ProviderAntigravity].HasRunningLow(now); !got {
		t.Errorf("expected Antigravity 3p-5h (6%% left) to be running low")
	}
	if cur := byID[ProviderCursor]; cur.Windows[0].DollarsUsed == nil || *cur.Windows[0].DollarsUsed != 12.40 {
		t.Errorf("cursor DollarsUsed = %v, want 12.40", cur.Windows[0].DollarsUsed)
	}
	if jb := byID[ProviderJetBrains]; jb.Windows[0].Used != 45 {
		t.Errorf("jetbrains Used = %.1f, want 45", jb.Windows[0].Used)
	}

	// Ensure cache serialization never contains raw tokens.
	cacheDir := t.TempDir()
	if err := SaveCache(cacheDir, snap); err != nil {
		t.Fatal(err)
	}
	rawCache, err := os.ReadFile(filepath.Join(cacheDir, cacheFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"claude-secret-tok", "gho_test_token", "ag-tok", "gemini-tok", codexJWT, cursorJWT} {
		if strings.Contains(string(rawCache), secret) {
			t.Errorf("cache leaked secret %q", secret)
		}
	}
}

func TestRateLimitBackoffAndStalePreservation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(home, ".claude", ".credentials.json"), map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken": "tok",
			"expiresAt":   now.Add(2 * time.Hour).UnixMilli(),
		},
	})

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":30,"resets_at":"2026-11-27T15:00:00Z"}}`))
			return
		}
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	mon := NewMonitor(t.TempDir(), func() FetchEnv {
		return FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderClaude: srv.URL},
		}
	}, nil)

	snap1 := mon.Refresh(context.Background(), true)
	if snap1.ConnectedCount() != 1 {
		t.Fatalf("snap1 connected = %d, want 1", snap1.ConnectedCount())
	}

	// Second call gets 429 -> should preserve last good window and set RetryAfter.
	snap2 := mon.Refresh(context.Background(), true)
	var claude ProviderQuota
	for _, p := range snap2.Providers {
		if p.Provider == ProviderClaude {
			claude = p
		}
	}
	if !claude.Connected() || len(claude.Windows) != 1 || claude.Windows[0].Used != 30 {
		t.Fatalf("expected stale window preserved on 429, got %+v", claude)
	}
	if claude.RetryAfter.IsZero() {
		t.Fatalf("expected RetryAfter set on 429")
	}

	// Third non-forced call before RetryAfter must skip HTTP call.
	prevCalls := calls
	_ = mon.Refresh(context.Background(), false)
	if calls != prevCalls {
		t.Errorf("expected backoff to skip HTTP call, calls went %d -> %d", prevCalls, calls)
	}
}

func TestLowWindowTrackerDebounceAndRearm(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	tracker := NewLowWindowTracker()

	makeSnap := func(used float64, resetsAt time.Time) Snapshot {
		return Snapshot{
			Providers: []ProviderQuota{{
				Provider: ProviderClaude,
				Name:     "Claude Code",
				Status:   StatusOK,
				Windows: []QuotaWindow{{
					ID:       "five_hour",
					Kind:     WindowSession,
					Used:     used,
					ResetsAt: resetsAt,
					Duration: 5 * time.Hour,
				}},
			}},
		}
	}

	// 1. Healthy -> no alert.
	if evs := tracker.Evaluate(makeSnap(40, now.Add(2*time.Hour)), now); len(evs) != 0 {
		t.Fatalf("healthy snap emitted %d events, want 0", len(evs))
	}
	// 2. Drops to 7% left -> emits 1 low alert.
	evs := tracker.Evaluate(makeSnap(93, now.Add(2*time.Hour)), now)
	if len(evs) != 1 || evs[0].Reset {
		t.Fatalf("expected 1 low alert, got %+v", evs)
	}
	// 3. Stays low (4% left) -> debounced (0 events).
	if evs = tracker.Evaluate(makeSnap(96, now.Add(2*time.Hour)), now.Add(5*time.Minute)); len(evs) != 0 {
		t.Fatalf("expected debounced low episode, got %+v", evs)
	}
	// 4. Window resets -> emits 1 reset event and rearms.
	evs = tracker.Evaluate(makeSnap(5, now.Add(7*time.Hour)), now.Add(2*time.Hour+time.Minute))
	if len(evs) != 1 || !evs[0].Reset {
		t.Fatalf("expected 1 reset event, got %+v", evs)
	}
	// 5. Drops low in new cycle -> fires again.
	evs = tracker.Evaluate(makeSnap(92, now.Add(7*time.Hour)), now.Add(3*time.Hour))
	if len(evs) != 1 || evs[0].Reset {
		t.Fatalf("expected re-armed low alert, got %+v", evs)
	}
}

func TestClaudeAPIKeyDetection(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := fetchClaude(context.Background(), FetchEnv{
		Home: home,
		Env:  sysenv.Env{"ANTHROPIC_API_KEY=sk-ant-test"},
	})
	if got.Status != StatusAPIKey {
		t.Errorf("Status = %q, want %q", got.Status, StatusAPIKey)
	}
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func createTestItemTableDB(t *testing.T, dbPath string, rows map[string]string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`); err != nil {
		t.Fatal(err)
	}
	for k, v := range rows {
		if _, err := db.Exec(`INSERT INTO ItemTable (key, value) VALUES (?, ?)`, k, v); err != nil {
			t.Fatal(err)
		}
	}
}
