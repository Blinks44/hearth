package aiquota

import (
	"context"
	"database/sql"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestNormalizeWindowsOrdering(t *testing.T) {
	t.Parallel()

	t.Run("antigravity groups and 5h before weekly", func(t *testing.T) {
		t.Parallel()
		in := []QuotaWindow{
			{ID: "c-wk", Group: "Claude", Kind: WindowWeekly, Used: 20},
			{ID: "g-wk", Group: "Gemini", Kind: WindowWeekly, Used: 25},
			{ID: "c-5h", Group: "Claude", Kind: WindowSession, Used: 10},
			{ID: "g-5h", Group: "Gemini", Kind: WindowSession, Used: 5},
		}
		got := NormalizeWindows(in)
		want := []string{"g-5h", "g-wk", "c-5h", "c-wk"}
		for i, w := range want {
			if got[i].ID != w {
				t.Fatalf("got[%d].ID = %q, want %q (all=%+v)", i, got[i].ID, w, got)
			}
		}
	})

	t.Run("ungrouped shorter before longer and plan-wide before model", func(t *testing.T) {
		t.Parallel()
		in := []QuotaWindow{
			{ID: "sonnet-wk", Kind: WindowWeekly, Model: "Sonnet", Used: 10},
			{ID: "monthly", Kind: WindowMonthly, Used: 50},
			{ID: "weekly", Kind: WindowWeekly, Used: 30},
			{ID: "session", Kind: WindowSession, Used: 15},
		}
		got := NormalizeWindows(in)
		want := []string{"session", "weekly", "sonnet-wk", "monthly"}
		for i, w := range want {
			if got[i].ID != w {
				t.Fatalf("got[%d].ID = %q, want %q", i, got[i].ID, w)
			}
		}
	})
}

func TestP1BugFixes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)

	t.Run("codex numeric balance and canonical window order", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
			"tokens": map[string]any{"access_token": "tok"},
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"plan_type":"pro",
				"rate_limit":{
					"primary_window":{"used_percent":40,"reset_at":1795900000,"limit_window_seconds":604800},
					"secondary_window":{"used_percent":12,"reset_at":1795790000,"limit_window_seconds":18000}
				},
				"credits":{"balance":15.25,"unlimited":false}
			}`))
		}))
		defer srv.Close()

		got := fetchCodex(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderCodex: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 2 {
			t.Fatalf("codex = %+v", got)
		}
		if got.Windows[0].Kind != WindowSession || got.Windows[1].Kind != WindowWeekly {
			t.Fatalf("expected 5h before weekly, got %+v", got.Windows)
		}
		if got.Credits == nil || got.Credits.RemainingUSD == nil || *got.Credits.RemainingUSD != 15.25 {
			t.Fatalf("expected numeric balance 15.25, got %+v", got.Credits)
		}
	})

	t.Run("antigravity filters disabled buckets and orders 5h before weekly", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		agDir := filepath.Join(home, ".antigravity")
		if err := os.MkdirAll(agDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(agDir, "credentials.json"), map[string]any{
			"access_token": "ag-tok",
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"groups":[
					{"name":"Gemini","buckets":[
						{"bucketId":"gemini-weekly","remainingFraction":0.75,"resetTime":"2026-12-03T00:00:00Z","windowDurationSeconds":604800},
						{"bucketId":"gemini-5h","remainingFraction":0.90,"resetTime":"2026-11-27T19:00:00Z","windowDurationSeconds":18000}
					]},
					{"name":"Claude","buckets":[
						{"bucketId":"claude-weekly","remainingFraction":0.50,"resetTime":"2026-12-03T00:00:00Z","windowDurationSeconds":604800,"disabled":true},
						{"bucketId":"claude-5h","remainingFraction":0.60,"resetTime":"2026-11-27T18:00:00Z","windowDurationSeconds":18000}
					]}
				]
			}`))
		}))
		defer srv.Close()

		got := fetchAntigravity(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderAntigravity: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 3 {
			t.Fatalf("expected 3 enabled windows, got %+v", got)
		}
		if got.Windows[0].Group != "Gemini" || got.Windows[0].Kind != WindowSession {
			t.Errorf("win[0] = %+v, want Gemini 5h", got.Windows[0])
		}
		if got.Windows[1].Group != "Gemini" || got.Windows[1].Kind != WindowWeekly {
			t.Errorf("win[1] = %+v, want Gemini weekly", got.Windows[1])
		}
		if got.Windows[2].Group != "Claude" || got.Windows[2].Kind != WindowSession {
			t.Errorf("win[2] = %+v, want Claude 5h", got.Windows[2])
		}
	})

	t.Run("gemini caches projectID across refreshes", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		gemDir := filepath.Join(home, ".gemini")
		if err := os.MkdirAll(gemDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(gemDir, "oauth_creds.json"), map[string]any{
			"access_token": "gem-tok",
			"expiry_date":  now.Add(2 * time.Hour).UnixMilli(),
		})
		var loadCalls, quotaCalls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "loadCodeAssist") {
				loadCalls.Add(1)
				_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-cached","currentTier":{"name":"Standard"}}`))
				return
			}
			quotaCalls.Add(1)
			_, _ = w.Write([]byte(`{"buckets":[{"modelId":"gemini-2.5-pro","remainingFraction":0.8,"resetTime":"2026-11-28T15:00:00Z"}]}`))
		}))
		defer srv.Close()

		mon := NewMonitor(t.TempDir(), func() FetchEnv {
			return FetchEnv{
				Home:      home,
				HTTP:      srv.Client(),
				Now:       func() time.Time { return now },
				Endpoints: map[ProviderID]string{ProviderGemini: srv.URL},
			}
		}, nil)

		mon.Refresh(context.Background(), true)
		mon.Refresh(context.Background(), true)

		if loadCalls.Load() != 1 {
			t.Errorf("loadCodeAssist calls = %d, want 1 (cached projectID)", loadCalls.Load())
		}
		if quotaCalls.Load() != 2 {
			t.Errorf("retrieveUserQuota calls = %d, want 2", quotaCalls.Load())
		}
	})

	t.Run("cursor utf16le blob token, wal-without-shm safety, and autoPercentUsed fallback", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		dbPath := cursorStateDBPath(home, nil)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			t.Fatal(err)
		}
		jwtTok := makeTestJWT(t, map[string]any{"sub": "auth0|user_utf16", "exp": now.Add(2 * time.Hour).Unix()})
		u16 := utf16.Encode([]rune(`"` + jwtTok + `"`))
		blob := make([]byte, len(u16)*2)
		for i, c := range u16 {
			binary.LittleEndian.PutUint16(blob[2*i:2*i+2], c)
		}
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ItemTable (key, value) VALUES (?, ?)`, "cursorAuth/accessToken", blob); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"billingCycleEnd":"2026-12-10T00:00:00Z",
				"individualUsage":{"plan":{"used":0,"limit":0,"autoPercentUsed":42,"apiPercentUsed":15}}
			}`))
		}))
		defer srv.Close()

		got := fetchCursor(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderCursor: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 28.5 {
			t.Fatalf("cursor utf16/autoPercentUsed = %+v, want 1 window with 28.5%% used", got)
		}

		// Creating a lone -wal file without -shm must refuse opening rather than
		// creating a -shm sidecar in Cursor's directory.
		if err := os.WriteFile(dbPath+"-wal", []byte("wal"), 0o600); err != nil {
			t.Fatal(err)
		}
		blocked := fetchCursor(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderCursor: srv.URL},
		})
		if blocked.Status != StatusUnavailable {
			t.Fatalf("expected StatusUnavailable when -wal lacks -shm, got %+v", blocked)
		}
		if pathExists(dbPath + "-shm") {
			t.Fatalf("opening sqlite unexpectedly created -shm sidecar")
		}
	})

	t.Run("jetbrains idea.log overrides older XML", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		roots := jetBrainsConfigRoots(home, nil)
		optDir := filepath.Join(roots[0], "GoLand2025.3", "options")
		if err := os.MkdirAll(optDir, 0o700); err != nil {
			t.Fatal(err)
		}
		xmlPath := filepath.Join(optDir, "AIAssistantQuotaManager2.xml")
		xmlBody := `<application><component name="AIAssistantQuotaManager2">
			<option name="quotaInfo" value="{&quot;type&quot;:&quot;Pro&quot;,&quot;current&quot;:200,&quot;maximum&quot;:1000,&quot;until&quot;:&quot;2026-12-05T00:00:00Z&quot;}" />
		</component></application>`
		if err := os.WriteFile(xmlPath, []byte(xmlBody), 0o600); err != nil {
			t.Fatal(err)
		}
		oldMod := now.Add(-2 * time.Hour)
		_ = os.Chtimes(xmlPath, oldMod, oldMod)

		logRoots := jetBrainsLogRoots(home, nil)
		logDir := filepath.Join(logRoots[0], "GoLand2025.3")
		if err := os.MkdirAll(logDir, 0o700); err != nil {
			t.Fatal(err)
		}
		logTS := now.Add(-5 * time.Minute).Local().Format("2006-01-02 15:04:05,000")
		logLine := logTS + " [ 1234] INFO - #c.i.m.l.q.QuotaManager2Impl - New quota state is: Active(quota={current=750, maximum=1000, until=2026-12-05T00:00:00Z})\n"
		if err := os.WriteFile(filepath.Join(logDir, "idea.log"), []byte(logLine), 0o600); err != nil {
			t.Fatal(err)
		}

		got := fetchJetBrains(context.Background(), FetchEnv{
			Home: home,
			Now:  func() time.Time { return now },
		})
		if got.Status != StatusOK || len(got.Windows) != 1 {
			t.Fatalf("jetbrains = %+v", got)
		}
		if got.Windows[0].Used != 75 {
			t.Errorf("expected newer idea.log usage 75%%, got %.1f%%", got.Windows[0].Used)
		}
	})

	t.Run("claude API-key keychain check never passes -w", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		var apiKeySawDashW bool
		runner := fakeRunner{fn: func(_ context.Context, cmd run.Cmd) (run.Result, error) {
			if slices.Contains(cmd.Args, "Claude Code") {
				if slices.Contains(cmd.Args, "-w") {
					apiKeySawDashW = true
				}
				return run.Result{ExitCode: 0}, nil
			}
			return run.Result{ExitCode: 44}, nil
		}}
		got := fetchClaude(context.Background(), FetchEnv{
			Home:   home,
			Runner: runner,
			Now:    func() time.Time { return now },
		})
		if apiKeySawDashW {
			t.Fatalf("API-key keychain existence check must not pass -w")
		}
		if got.Status != StatusAPIKey && got.Status != StatusSignedOut {
			t.Fatalf("unexpected status %q", got.Status)
		}
	})
}

func TestP2RealSchemasAndCLIPreFilter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)

	t.Run("grok nested config schema", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(home, ".grok", "auth.json"), map[string]any{"access_token": "gtok"})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"config":{
					"creditUsagePercent":37.5,
					"subscriptionTier":"SuperGrok",
					"currentPeriod":{"start":"2026-11-01T00:00:00Z","end":"2026-12-01T00:00:00Z"}
				}
			}`))
		}))
		defer srv.Close()

		got := fetchGrok(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderGrok: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 37.5 || got.Plan != "SuperGrok" {
			t.Fatalf("grok = %+v", got)
		}
	})

	t.Run("windsurf remaining percents schema", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		dbPath := windsurfStateDBPath(home, nil)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			t.Fatal(err)
		}
		createTestItemTableDB(t, dbPath, map[string]string{
			"windsurf.settings.cachedPlanInfo": `{"planName":"Pro","quotaUsage":{"dailyRemainingPercent":70,"dailyResetAtUnix":1795824000,"weeklyRemainingPercent":40,"weeklyResetAtUnix":1796256000}}`,
		})
		got := fetchWindsurf(context.Background(), FetchEnv{Home: home, Now: func() time.Time { return now }})
		if got.Status != StatusOK || len(got.Windows) != 2 || got.Windows[0].Used != 30 || got.Windows[1].Used != 60 {
			t.Fatalf("windsurf = %+v", got)
		}
	})

	t.Run("cline nested providers.json workos prefix and data.limits", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		pDir := filepath.Join(home, ".cline", "data", "settings")
		if err := os.MkdirAll(pDir, 0o700); err != nil {
			t.Fatal(err)
		}
		tok := makeTestJWT(t, map[string]any{"exp": now.Add(2 * time.Hour).Unix()})
		writeJSONFile(t, filepath.Join(pDir, "providers.json"), map[string]any{
			"providers": map[string]any{
				"cline": map[string]any{
					"settings": map[string]any{
						"auth": map[string]any{"accessToken": tok},
					},
				},
			},
		})
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"success":true,"data":{"limits":[{"type":"weekly","percentUsed":55,"resetsAt":"2026-12-03T00:00:00Z"},{"type":"five_hour","percentUsed":20,"resetsAt":"2026-11-27T19:00:00Z"}]}}`))
		}))
		defer srv.Close()

		got := fetchCline(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderCline: srv.URL},
		})
		if gotAuth != "Bearer workos:"+tok {
			t.Errorf("Authorization = %q, want Bearer workos:<jwt>", gotAuth)
		}
		if got.Status != StatusOK || len(got.Windows) != 2 || got.Windows[0].Kind != WindowSession || got.Windows[0].Used != 20 {
			t.Fatalf("cline = %+v", got)
		}
	})

	t.Run("kimi used_ratio fraction", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		kDir := filepath.Join(home, ".kimi-code", "credentials")
		if err := os.MkdirAll(kDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(kDir, "kimi-code.json"), map[string]any{
			"access_token": "ktok",
			"expires_at":   now.Add(2 * time.Hour).Unix(),
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"usages":{"limit_5h":{"used_ratio":0.25,"reset_time":"2026-11-27T18:00:00Z"}},"user":{"membership":{"level":"LEVEL_PRO"}}}`))
		}))
		defer srv.Close()

		got := fetchKimi(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderKimi: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 25 || got.Plan != "" {
			t.Fatalf("kimi = %+v", got)
		}
	})

	t.Run("opencode sqlite credential json and usage wrapper", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		ocDir := filepath.Join(home, ".local", "share", "opencode")
		if err := os.MkdirAll(ocDir, 0o700); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", filepath.Join(ocDir, "opencode.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE credential (active INTEGER, integration_id TEXT, value TEXT)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO credential VALUES (1, 'opencode-go', '{"type":"key","key":"oc-key"}')`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":18,"resetsAt":"2026-11-27T18:00:00Z"},"weekly":{"percent":44,"resetsAt":"2026-12-03T00:00:00Z"}}}`))
		}))
		defer srv.Close()

		got := fetchOpenCode(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderOpenCode: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 2 || got.Windows[0].Used != 18 {
			t.Fatalf("opencode = %+v", got)
		}
	})

	t.Run("kiro auth_kv + state profileArn + CREDIT filter", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		dbPath := kiroDBPath(home, nil)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE auth_kv (key TEXT, value TEXT); CREATE TABLE state (key TEXT, value TEXT);`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO auth_kv VALUES ('kirocli:odic:token', '{"access_token":"kiro-tok","expires_at":"2026-12-01T00:00:00Z"}')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO state VALUES ('api.codewhisperer.profile', '{"arn":"arn:aws:codewhisperer:eu-central-1:123456:profile/abc"}')`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"subscriptionInfo":{"subscriptionTitle":"Kiro Pro"},
				"usageBreakdownList":[
					{"resourceType":"OTHER","currentUsageWithPrecision":9,"usageLimitWithPrecision":10,"displayName":"Ignored"},
					{"resourceType":"CREDIT","currentUsageWithPrecision":25,"usageLimitWithPrecision":100,"nextDateReset":1796256000,"displayName":"Credits"}
				]
			}`))
		}))
		defer srv.Close()

		got := fetchKiro(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderKiro: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 25 || got.Windows[0].Model != "Credits" {
			t.Fatalf("kiro = %+v", got)
		}
	})

	t.Run("kilo trpc superjson microdollars and pass subscription", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		kDir := filepath.Join(home, ".local", "share", "kilo")
		if err := os.MkdirAll(kDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(kDir, "auth.json"), map[string]any{
			"kilo": map[string]any{"access": "kilo-tok"},
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[
				{"result":{"data":{"json":{"totalBalance_mUsd":12500000}}}},
				{"result":{"data":{"json":{"subscription":{"tier":"Pass","currentPeriodUsageUsd":5,"currentPeriodBaseCreditsUsd":20,"nextBillingAt":"2026-12-05T00:00:00Z"}}}}}
			]`))
		}))
		defer srv.Close()

		got := fetchKilo(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderKilo: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 25 {
			t.Fatalf("kilo windows = %+v", got)
		}
		if got.Credits == nil || got.Credits.RemainingUSD == nil || *got.Credits.RemainingUSD != 12.5 {
			t.Fatalf("kilo credits = %+v", got.Credits)
		}
	})

	t.Run("huggingface base and current remaining", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		hfDir := filepath.Join(home, ".cache", "huggingface")
		if err := os.MkdirAll(hfDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hfDir, "token"), []byte("hf_tok\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"base":1500,"current":900,"resetsAt":"2026-11-28T00:00:00Z"}`))
		}))
		defer srv.Close()

		got := fetchHuggingFace(context.Background(), FetchEnv{
			Home:      home,
			HTTP:      srv.Client(),
			Now:       func() time.Time { return now },
			Endpoints: map[ProviderID]string{ProviderHuggingFace: srv.URL},
		})
		if got.Status != StatusOK || len(got.Windows) != 1 || got.Windows[0].Used != 40 {
			t.Fatalf("huggingface = %+v", got)
		}
	})

	t.Run("cli providers skip spawning when executable absent", func(t *testing.T) {
		t.Parallel()
		var ran atomic.Int32
		runner := fakeRunner{fn: func(_ context.Context, _ run.Cmd) (run.Result, error) {
			ran.Add(1)
			return run.Result{ExitCode: 0}, nil
		}}
		env := FetchEnv{
			Home:   t.TempDir(),
			Env:    sysenv.Env{"PATH=" + t.TempDir()},
			Runner: runner,
		}
		if got := fetchAmp(context.Background(), env); got.Status != StatusNotFound {
			t.Errorf("amp = %q, want not_found", got.Status)
		}
		if got := fetchAugment(context.Background(), env); got.Status != StatusNotFound {
			t.Errorf("augment = %q, want not_found", got.Status)
		}
		if got := fetchCodeRabbit(context.Background(), env); got.Status != StatusNotFound {
			t.Errorf("coderabbit = %q, want not_found", got.Status)
		}
		if ran.Load() != 0 {
			t.Fatalf("expected 0 process spawns for missing CLI tools, got %d", ran.Load())
		}
	})
}

func TestMonitorAdaptiveScheduleProgressAndTrackerFixes(t *testing.T) {
	t.Parallel()

	t.Run("tracker seed, stale skip, and resets_at jitter resistance", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
		tracker := NewLowWindowTracker()
		lowSnap := Snapshot{
			CheckedAt: now,
			Providers: []ProviderQuota{{
				Provider:  ProviderClaude,
				Name:      "Claude Code",
				Status:    StatusOK,
				UpdatedAt: now,
				Windows: []QuotaWindow{{
					ID:       "five_hour",
					Kind:     WindowSession,
					Used:     95,
					ResetsAt: now.Add(3 * time.Hour),
					Duration: 5 * time.Hour,
				}},
			}},
		}
		// Seeding from cache must suppress duplicate alert on first live poll.
		tracker.Seed(lowSnap, now)
		if evs := tracker.Evaluate(lowSnap, now.Add(time.Minute)); len(evs) != 0 {
			t.Fatalf("expected 0 alerts after Seed, got %+v", evs)
		}
		// Server jitter of +2m on ResetsAt in the same 5h period must NOT re-alert.
		jittered := lowSnap
		jittered.Providers = slices.Clone(lowSnap.Providers)
		jittered.Providers[0].Windows = []QuotaWindow{{
			ID:       "five_hour",
			Kind:     WindowSession,
			Used:     96,
			ResetsAt: now.Add(3*time.Hour + 2*time.Minute),
			Duration: 5 * time.Hour,
		}}
		jittered.Providers[0].UpdatedAt = now.Add(2 * time.Minute)
		if evs := tracker.Evaluate(jittered, now.Add(2*time.Minute)); len(evs) != 0 {
			t.Fatalf("resets_at jitter re-triggered alert: %+v", evs)
		}

		// Stale provider must not emit low alerts.
		tracker2 := NewLowWindowTracker()
		stale := lowSnap
		stale.Providers = slices.Clone(lowSnap.Providers)
		stale.Providers[0].UpdatedAt = now.Add(-20 * time.Minute)
		if evs := tracker2.Evaluate(stale, now); len(evs) != 0 {
			t.Fatalf("stale provider emitted alert: %+v", evs)
		}
	})

	t.Run("429 RetryAfter respected even on forced refresh and OnProgress fires", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(home, ".claude", ".credentials.json"), map[string]any{
			"claudeAiOauth": map[string]any{"accessToken": "tok", "expiresAt": now.Add(2 * time.Hour).UnixMilli()},
		})
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"five_hour":{"utilization":20,"resets_at":"2026-11-27T15:00:00Z"}}`))
				return
			}
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		var progressMu sync.Mutex
		progressCount := 0
		mon := NewMonitor(t.TempDir(), func() FetchEnv {
			return FetchEnv{
				Home:      home,
				HTTP:      srv.Client(),
				Now:       func() time.Time { return now },
				Endpoints: map[ProviderID]string{ProviderClaude: srv.URL},
			}
		}, nil)
		mon.OnProgress = func(s Snapshot) {
			progressMu.Lock()
			if s.ConnectedCount() > 0 {
				progressCount++
			}
			progressMu.Unlock()
		}

		mon.Refresh(context.Background(), true)
		progressMu.Lock()
		gotProg := progressCount
		progressMu.Unlock()
		if gotProg == 0 {
			t.Errorf("expected OnProgress to fire with connected snapshot")
		}

		// Trigger 429.
		mon.Refresh(context.Background(), true)
		beforeForce := calls.Load()
		// Even forced refresh during RetryAfter cooldown must NOT hit server.
		mon.Refresh(context.Background(), true)
		if calls.Load() != beforeForce {
			t.Fatalf("forced refresh bypassed 429 RetryAfter (%d -> %d)", beforeForce, calls.Load())
		}
	})

	t.Run("adaptive background intervals: 5m -> 15m idle -> sleep until reset", func(t *testing.T) {
		t.Parallel()
		clock := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
		var clockMu sync.Mutex
		nowFn := func() time.Time {
			clockMu.Lock()
			defer clockMu.Unlock()
			return clock
		}
		advance := func(d time.Duration) {
			clockMu.Lock()
			clock = clock.Add(d)
			clockMu.Unlock()
		}

		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, filepath.Join(home, ".claude", ".credentials.json"), map[string]any{
			"claudeAiOauth": map[string]any{"accessToken": "tok", "expiresAt": clock.Add(24 * time.Hour).UnixMilli()},
		})
		var util atomic.Int32
		util.Store(30)
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			u := util.Load()
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":` + strconvItoa(int(u)) + `,"resets_at":"2026-11-27T15:00:00Z"}}`))
		}))
		defer srv.Close()

		mon := NewMonitor(t.TempDir(), func() FetchEnv {
			return FetchEnv{
				Home:      home,
				HTTP:      srv.Client(),
				Now:       nowFn,
				Endpoints: map[ProviderID]string{ProviderClaude: srv.URL},
			}
		}, nil)

		// Initial active poll -> schedules nextPollAt = +5m.
		mon.RefreshMode(context.Background(), PollActive)
		if calls.Load() != 1 {
			t.Fatalf("calls = %d, want 1", calls.Load())
		}
		// At +2m, PollBackground must skip (not due).
		advance(2 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 1 {
			t.Fatalf("expected skip at +2m, got calls=%d", calls.Load())
		}
		// At +5m (unchanged #1) -> polls, still 5m cadence.
		advance(3 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 2 {
			t.Fatalf("expected poll at +5m, got calls=%d", calls.Load())
		}
		// At +10m (unchanged #2) -> polls and backs off to 15m idle!
		advance(5 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 3 {
			t.Fatalf("expected poll at +10m, got calls=%d", calls.Load())
		}
		// At +16m (only 6m since last poll) -> must NOT poll because backed off to 15m.
		advance(6 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 3 {
			t.Fatalf("expected 15m idle backoff to skip at +6m, got calls=%d", calls.Load())
		}
		// Advance another 10m (16m since last poll) and report 100% exhausted.
		util.Store(100)
		advance(10 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 4 {
			t.Fatalf("expected poll after 15m idle, got calls=%d", calls.Load())
		}
		// Now exhausted with ResetsAt = 15:00:00Z; clock is 12:26. Polling at 13:00 must skip!
		advance(34 * time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 4 {
			t.Fatalf("expected exhausted provider to sleep until ResetsAt, got calls=%d", calls.Load())
		}
		// Advance past ResetsAt + 15s (to 15:01) -> wakes up and polls!
		advance(2*time.Hour + time.Minute)
		mon.RefreshMode(context.Background(), PollBackground)
		if calls.Load() != 5 {
			t.Fatalf("expected exhausted provider to wake after ResetsAt, got calls=%d", calls.Load())
		}
	})
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
