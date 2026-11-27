package aiquota

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type geminiProjectEntry struct {
	projectID string
	plan      string
	expiresAt time.Time
}

type geminiProjectCache struct {
	mu      sync.Mutex
	entries map[string]geminiProjectEntry
}

func newGeminiProjectCache() *geminiProjectCache {
	return &geminiProjectCache{entries: map[string]geminiProjectEntry{}}
}

func geminiTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

func (c *geminiProjectCache) get(token string, now time.Time) (projectID, plan string, ok bool) {
	if c == nil || token == "" {
		return "", "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, found := c.entries[geminiTokenHash(token)]
	if !found || !ent.expiresAt.After(now) {
		return "", "", false
	}
	return ent.projectID, ent.plan, true
}

func (c *geminiProjectCache) set(token, projectID, plan string, now time.Time) {
	if c == nil || token == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[geminiTokenHash(token)] = geminiProjectEntry{
		projectID: projectID,
		plan:      plan,
		expiresAt: now.Add(2 * time.Hour),
	}
}

func (c *geminiProjectCache) invalidate(token string) {
	if c == nil || token == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, geminiTokenHash(token))
}

// FetchEnv supplies read-only dependencies for quota provider fetchers.
type FetchEnv struct {
	Home      string
	Env       sysenv.Env
	Runner    run.Runner
	HTTP      *http.Client
	Now       func() time.Time
	Installed map[ProviderID]bool
	// Endpoints optionally overrides a provider's base URL (used in tests).
	Endpoints map[ProviderID]string

	geminiCache *geminiProjectCache
}

func (f FetchEnv) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f FetchEnv) baseURL(id ProviderID, def string) string {
	if f.Endpoints != nil {
		if u := strings.TrimRight(f.Endpoints[id], "/"); u != "" {
			return u
		}
	}
	return strings.TrimRight(def, "/")
}

func (f FetchEnv) isInstalled(id ProviderID) bool {
	return f.Installed != nil && f.Installed[id]
}

func (f FetchEnv) hasExecutable(name string) bool {
	if name == "" {
		return false
	}
	pathVal := f.Env.Get("PATH")
	var dirs []string
	if pathVal != "" {
		dirs = append(dirs, filepath.SplitList(pathVal)...)
	}
	if f.Home != "" {
		dirs = append(dirs,
			filepath.Join(f.Home, ".local", "bin"),
			filepath.Join(f.Home, ".amp", "bin"),
			filepath.Join(f.Home, ".bun", "bin"),
		)
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return true
		}
		if info, err := os.Stat(cand + ".exe"); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func (f FetchEnv) newResult(id ProviderID) ProviderQuota {
	info := LookupProvider(id)
	return ProviderQuota{
		Provider:  id,
		Name:      info.Name,
		Installed: f.isInstalled(id),
	}
}

func mapHTTPError(res *ProviderQuota, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, errUnauthorized) {
		res.Status = StatusSignedOut
		return
	}
	if rl, ok := errors.AsType[*rateLimitError](err); ok {
		res.Status = StatusUnavailable
		res.RetryAfter = rl.retryAfter
		res.Err = "rate limited"
		return
	}
	res.Status = StatusUnavailable
	res.Err = err.Error()
}

func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func formatPlanName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	switch lower {
	case "max", "claude_max", "max_5x", "max_20x":
		if strings.Contains(lower, "20x") {
			return "Max 20x"
		}
		if strings.Contains(lower, "5x") {
			return "Max 5x"
		}
		return "Max"
	case "pro", "claude_pro", "pro_plus":
		if strings.Contains(lower, "plus") {
			return "Pro+"
		}
		return "Pro"
	case "plus":
		return "Plus"
	case "team":
		return "Team"
	case "enterprise":
		return "Enterprise"
	case "business":
		return "Business"
	case "individual":
		return "Individual"
	case "individual_pro":
		return "Pro"
	case "free":
		return "Free"
	default:
		s = strings.ReplaceAll(s, "_", " ")
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
}

// --- 1. Claude Code ---

func fetchClaude(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderClaude)
	claudeDir := filepath.Join(env.Home, ".claude")
	credFile := filepath.Join(claudeDir, ".credentials.json")
	claudeJSON := filepath.Join(env.Home, ".claude.json")
	detected := res.Installed || pathExists(claudeDir) || pathExists(claudeJSON) || env.hasExecutable("claude")

	type claudeOAuth struct {
		AccessToken      string `json:"accessToken"`
		ExpiresAt        int64  `json:"expiresAt"`
		SubscriptionType string `json:"subscriptionType"`
		RateLimitTier    string `json:"rateLimitTier"`
	}
	type claudeCreds struct {
		ClaudeAiOauth *claudeOAuth `json:"claudeAiOauth"`
	}

	var creds claudeCreds
	if data, err := readCappedFile(credFile, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &creds)
	}
	if (creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "") && detected {
		if raw, ok := readKeychainSecret(ctx, env.Runner, "Claude Code-credentials", ""); ok {
			_ = json.Unmarshal([]byte(raw), &creds)
		}
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		// Check whether Claude Code is configured via API key instead of OAuth.
		hasAPIKey := env.Env.Get("ANTHROPIC_API_KEY") != ""
		if !hasAPIKey {
			if data, err := readCappedFile(claudeJSON, maxConfigFileBytes); err == nil {
				var cfg struct {
					PrimaryAPIKey string `json:"primaryApiKey"`
				}
				if json.Unmarshal(data, &cfg) == nil && cfg.PrimaryAPIKey != "" {
					hasAPIKey = true
				}
			}
		}
		if !hasAPIKey && detected {
			// Existence check only (without -w), never reading the API key value.
			hasAPIKey = hasKeychainSecret(ctx, env.Runner, "Claude Code", "")
		}
		if !detected && !hasAPIKey {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		if hasAPIKey {
			res.Status = StatusAPIKey
		} else {
			res.Status = StatusSignedOut
		}
		return res
	}

	res.Installed = true
	now := env.now()
	if creds.ClaudeAiOauth.ExpiresAt > 0 && time.UnixMilli(creds.ClaudeAiOauth.ExpiresAt).Before(now) {
		res.Status = StatusExpired
		return res
	}
	if creds.ClaudeAiOauth.SubscriptionType != "" {
		res.Plan = formatPlanName(creds.ClaudeAiOauth.SubscriptionType)
	}

	endpoint := env.baseURL(ProviderClaude, "https://api.anthropic.com") + "/api/oauth/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+creds.ClaudeAiOauth.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hearth")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type windowPayload struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	}
	type extraPayload struct {
		IsEnabled    bool     `json:"is_enabled"`
		MonthlyLimit *float64 `json:"monthly_limit"`
		UsedCredits  *float64 `json:"used_credits"`
		Utilization  *float64 `json:"utilization"`
	}
	var payload struct {
		FiveHour        *windowPayload `json:"five_hour"`
		SevenDay        *windowPayload `json:"seven_day"`
		SevenDaySonnet  *windowPayload `json:"seven_day_sonnet"`
		SevenDayOpus    *windowPayload `json:"seven_day_opus"`
		ExtraUsage      *extraPayload  `json:"extra_usage"`
		SubscriptionTyp string         `json:"subscription_type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode claude usage: %w", err).Error()
		return res
	}
	if res.Plan == "" && payload.SubscriptionTyp != "" {
		res.Plan = formatPlanName(payload.SubscriptionTyp)
	}

	addWin := func(id string, kind WindowKind, model string, dur time.Duration, wp *windowPayload) {
		if wp == nil || wp.Utilization == nil {
			return
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     kind,
			Model:    model,
			Used:     clampPct(*wp.Utilization),
			ResetsAt: parseISO8601(wp.ResetsAt),
			Duration: dur,
		})
	}
	addWin("five_hour", WindowSession, "", 5*time.Hour, payload.FiveHour)
	addWin("seven_day", WindowWeekly, "", 7*24*time.Hour, payload.SevenDay)
	addWin("seven_day_sonnet", WindowWeekly, "Sonnet", 7*24*time.Hour, payload.SevenDaySonnet)
	addWin("seven_day_opus", WindowWeekly, "Opus", 7*24*time.Hour, payload.SevenDayOpus)

	if payload.ExtraUsage != nil && payload.ExtraUsage.IsEnabled {
		ci := &CreditsInfo{}
		if payload.ExtraUsage.UsedCredits != nil {
			ci.UsedUSD = new(*payload.ExtraUsage.UsedCredits / 100.0)
		}
		if payload.ExtraUsage.MonthlyLimit != nil && *payload.ExtraUsage.MonthlyLimit > 0 {
			ci.LimitUSD = new(*payload.ExtraUsage.MonthlyLimit / 100.0)
		}
		if ci.UsedUSD != nil || ci.LimitUSD != nil {
			res.Credits = ci
		}
	}

	if len(res.Windows) == 0 && res.Credits == nil {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 2. Codex ---

func fetchCodex(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderCodex)
	codexDir := filepath.Join(env.Home, ".codex")
	authFile := filepath.Join(codexDir, "auth.json")
	detected := res.Installed || pathExists(codexDir)

	var auth struct {
		AuthMode     string `json:"auth_mode"`
		OpenAIAPIKey string `json:"OPENAI_API_KEY"`
		Tokens       *struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if data, err := readCappedFile(authFile, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &auth)
	}

	token := ""
	accountID := ""
	if auth.Tokens != nil {
		token = strings.TrimSpace(auth.Tokens.AccessToken)
		accountID = strings.TrimSpace(auth.Tokens.AccountID)
	}
	if token == "" {
		hasAPIKey := strings.EqualFold(auth.AuthMode, "apikey") ||
			strings.TrimSpace(auth.OpenAIAPIKey) != "" ||
			env.Env.Get("OPENAI_API_KEY") != ""
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		if hasAPIKey {
			res.Status = StatusAPIKey
		} else {
			res.Status = StatusSignedOut
		}
		return res
	}

	res.Installed = true
	now := env.now()
	if jwtExpired(parseJWTClaims(token), now) {
		res.Status = StatusExpired
		return res
	}

	endpoint := env.baseURL(ProviderCodex, "https://chatgpt.com") + "/backend-api/wham/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hearth")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type codexWindow struct {
		UsedPercent        *float64 `json:"used_percent"`
		LimitWindowSeconds int64    `json:"limit_window_seconds"`
		ResetAt            int64    `json:"reset_at"`
	}
	type codexRateLimit struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	}
	var payload struct {
		PlanType             string          `json:"plan_type"`
		RateLimit            *codexRateLimit `json:"rate_limit"`
		AdditionalRateLimits []struct {
			LimitName string          `json:"limit_name"`
			RateLimit *codexRateLimit `json:"rate_limit"`
		} `json:"additional_rate_limits"`
		Credits *struct {
			HasCredits bool `json:"has_credits"`
			Unlimited  bool `json:"unlimited"`
			Balance    any  `json:"balance"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode codex usage: %w", err).Error()
		return res
	}

	res.Plan = formatPlanName(payload.PlanType)

	windowKindForDuration := func(d time.Duration, fallback WindowKind) WindowKind {
		switch {
		case d > 0 && d <= 12*time.Hour:
			return WindowSession
		case d > 12*time.Hour && d <= 36*time.Hour:
			return WindowDaily
		case d > 36*time.Hour && d <= 14*24*time.Hour:
			return WindowWeekly
		case d > 14*24*time.Hour:
			return WindowMonthly
		default:
			return fallback
		}
	}

	addCodexWin := func(id, model string, fallback WindowKind, cw *codexWindow) {
		if cw == nil || cw.UsedPercent == nil {
			return
		}
		dur := time.Duration(cw.LimitWindowSeconds) * time.Second
		if dur <= 0 {
			if fallback == WindowSession {
				dur = 5 * time.Hour
			} else {
				dur = 7 * 24 * time.Hour
			}
		}
		var resetsAt time.Time
		if cw.ResetAt > 0 {
			resetsAt = time.Unix(cw.ResetAt, 0).UTC()
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     windowKindForDuration(dur, fallback),
			Model:    model,
			Used:     clampPct(*cw.UsedPercent),
			ResetsAt: resetsAt,
			Duration: dur,
		})
	}

	if payload.RateLimit != nil {
		addCodexWin("primary", "", WindowSession, payload.RateLimit.PrimaryWindow)
		addCodexWin("secondary", "", WindowWeekly, payload.RateLimit.SecondaryWindow)
	}
	for i, extra := range payload.AdditionalRateLimits {
		if extra.RateLimit == nil {
			continue
		}
		model := strings.TrimSpace(extra.LimitName)
		addCodexWin(fmt.Sprintf("extra_%d_primary", i), model, WindowSession, extra.RateLimit.PrimaryWindow)
		addCodexWin(fmt.Sprintf("extra_%d_secondary", i), model, WindowWeekly, extra.RateLimit.SecondaryWindow)
	}
	if payload.Credits != nil {
		bal, hasBal := parseAnyFloat(payload.Credits.Balance)
		if payload.Credits.HasCredits || payload.Credits.Unlimited || (hasBal && bal > 0) {
			ci := &CreditsInfo{Unlimited: payload.Credits.Unlimited}
			if hasBal {
				ci.RemainingUSD = new(bal)
			}
			res.Credits = ci
		}
	}

	if len(res.Windows) == 0 && res.Credits == nil {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 3. Antigravity ---

func fetchAntigravity(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderAntigravity)
	detected := res.Installed ||
		pathExists(filepath.Join(env.Home, ".gemini", "antigravity")) ||
		pathExists(filepath.Join(env.Home, ".gemini", "antigravity-cli")) ||
		pathExists(filepath.Join(env.Home, ".antigravity")) ||
		pathExists(filepath.Join(env.Home, ".config", "antigravity")) ||
		env.hasExecutable("agy")

	type agTokenPayload struct {
		AccessToken string `json:"access_token"`
		Expiry      string `json:"expiry"`
		Token       *struct {
			AccessToken string `json:"access_token"`
			Expiry      string `json:"expiry"`
		} `json:"token"`
	}

	var rawSecret string
	if detected {
		if val, ok := readKeychainSecret(ctx, env.Runner, "gemini", "antigravity"); ok {
			rawSecret = val
		}
	}
	if rawSecret == "" {
		for _, candidate := range []string{
			filepath.Join(env.Home, ".antigravity", "credentials.json"),
			filepath.Join(env.Home, ".config", "antigravity", "credentials.json"),
		} {
			if data, err := readCappedFile(candidate, maxConfigFileBytes); err == nil {
				detected = true
				rawSecret = strings.TrimSpace(string(data))
				break
			}
		}
	}

	if rawSecret == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	decoded := []byte(rawSecret)
	if b64, ok := strings.CutPrefix(rawSecret, "go-keyring-base64:"); ok {
		if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64)); err == nil {
			decoded = b
		}
	}
	var tok agTokenPayload
	if err := json.Unmarshal(decoded, &tok); err != nil {
		res.Status = StatusSignedOut
		return res
	}
	accessToken := tok.AccessToken
	expiry := tok.Expiry
	if tok.Token != nil {
		if accessToken == "" {
			accessToken = tok.Token.AccessToken
		}
		if expiry == "" {
			expiry = tok.Token.Expiry
		}
	}
	if accessToken == "" {
		res.Status = StatusSignedOut
		return res
	}

	now := env.now()
	if exp := parseISO8601(expiry); !exp.IsZero() && exp.Before(now) {
		res.Status = StatusExpired
		return res
	}

	endpoint := env.baseURL(ProviderAntigravity, "https://daily-cloudcode-pa.googleapis.com") + "/v1internal:retrieveUserQuotaSummary"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "antigravity/hub/2.9.1 (hearth)")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type agBucket struct {
		ID                    string   `json:"id"`
		BucketID              string   `json:"bucketId"`
		RemainingFraction     *float64 `json:"remainingFraction"`
		ResetTime             string   `json:"resetTime"`
		WindowDurationSeconds int64    `json:"windowDurationSeconds"`
		Disabled              bool     `json:"disabled"`
	}
	type agGroup struct {
		Name    string     `json:"name"`
		Buckets []agBucket `json:"buckets"`
	}
	var payload struct {
		Tier   string    `json:"tier"`
		Groups []agGroup `json:"groups"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode antigravity quota: %w", err).Error()
		return res
	}
	if payload.Tier != "" {
		res.Plan = formatPlanName(payload.Tier)
	}

	seenBuckets := map[string]bool{}
	for _, g := range payload.Groups {
		for _, b := range g.Buckets {
			if b.Disabled || b.RemainingFraction == nil {
				continue
			}
			rem := *b.RemainingFraction
			if math.IsNaN(rem) || math.IsInf(rem, 0) || rem < 0 || rem > 1 {
				continue
			}
			bid := b.BucketID
			if bid == "" {
				bid = b.ID
			}
			if bid != "" && seenBuckets[bid] {
				continue
			}
			if bid != "" {
				seenBuckets[bid] = true
			}
			lower := strings.ToLower(bid)
			group := strings.TrimSpace(g.Name)
			if group == "" {
				switch {
				case strings.Contains(lower, "argon"):
					group = "Argon"
				case strings.Contains(lower, "3p") || strings.Contains(lower, "claude"):
					group = "Claude"
				case strings.Contains(lower, "gemini"):
					group = "Gemini"
				default:
					group = "Quota"
				}
			}
			kind := WindowSession
			dur := time.Duration(b.WindowDurationSeconds) * time.Second
			if strings.Contains(lower, "weekly") || strings.Contains(lower, "week") || dur >= 3*24*time.Hour {
				kind = WindowWeekly
				if dur <= 0 {
					dur = 7 * 24 * time.Hour
				}
			} else if dur <= 0 {
				dur = 5 * time.Hour
			}
			used := clampPct((1.0 - rem) * 100.0)
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       bid,
				Kind:     kind,
				Group:    group,
				Used:     used,
				ResetsAt: parseISO8601(b.ResetTime),
				Duration: dur,
			})
		}
	}

	if len(res.Windows) == 0 {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 4. Gemini CLI ---

func fetchGemini(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderGemini)
	geminiDir := filepath.Join(env.Home, ".gemini")
	credFile := filepath.Join(geminiDir, "oauth_creds.json")
	detected := res.Installed || pathExists(geminiDir)

	var creds struct {
		AccessToken string `json:"access_token"`
		ExpiryDate  int64  `json:"expiry_date"`
	}
	if data, err := readCappedFile(credFile, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &creds)
	}

	token := strings.TrimSpace(creds.AccessToken)
	if token == "" {
		hasAPIKey := env.Env.Get("GEMINI_API_KEY") != "" || env.Env.Get("GOOGLE_API_KEY") != ""
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		if hasAPIKey {
			res.Status = StatusAPIKey
		} else {
			res.Status = StatusSignedOut
		}
		return res
	}

	res.Installed = true
	now := env.now()
	if creds.ExpiryDate > 0 && time.UnixMilli(creds.ExpiryDate).Before(now) {
		res.Status = StatusExpired
		return res
	}

	base := env.baseURL(ProviderGemini, "https://cloudcode-pa.googleapis.com")
	projectID, cachedPlan, hasCached := env.geminiCache.get(token, now)
	if hasCached {
		res.Plan = cachedPlan
	} else {
		loadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1internal:loadCodeAssist",
			bytes.NewReader([]byte(`{"metadata":{"ideType":"GEMINI_CLI","pluginType":"GEMINI"}}`)))
		if err != nil {
			res.Status = StatusUnavailable
			res.Err = err.Error()
			return res
		}
		loadReq.Header.Set("Authorization", "Bearer "+token)
		loadReq.Header.Set("Content-Type", "application/json")
		loadReq.Header.Set("Accept", "application/json")

		loadBody, err := doJSON(ctx, env.HTTP, loadReq)
		if err != nil {
			mapHTTPError(&res, err)
			return res
		}

		var loadResp struct {
			Project     json.RawMessage `json:"cloudaicompanionProject"`
			CurrentTier *struct {
				Name string `json:"name"`
			} `json:"currentTier"`
		}
		if err := json.Unmarshal(loadBody, &loadResp); err != nil {
			res.Status = StatusUnavailable
			res.Err = fmt.Errorf("decode gemini code assist: %w", err).Error()
			return res
		}
		if loadResp.CurrentTier != nil && loadResp.CurrentTier.Name != "" {
			res.Plan = formatPlanName(loadResp.CurrentTier.Name)
		}

		if len(loadResp.Project) > 0 {
			if err := json.Unmarshal(loadResp.Project, &projectID); err != nil {
				var obj struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(loadResp.Project, &obj) == nil {
					projectID = obj.ID
				}
			}
		}
		env.geminiCache.set(token, projectID, res.Plan, now)
	}

	quotaReqBody := []byte("{}")
	if projectID != "" {
		if b, err := json.Marshal(map[string]string{"project": projectID}); err == nil {
			quotaReqBody = b
		}
	}
	quotaReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1internal:retrieveUserQuota", bytes.NewReader(quotaReqBody))
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	quotaReq.Header.Set("Authorization", "Bearer "+token)
	quotaReq.Header.Set("Content-Type", "application/json")
	quotaReq.Header.Set("Accept", "application/json")

	quotaBody, err := doJSON(ctx, env.HTTP, quotaReq)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			env.geminiCache.invalidate(token)
		}
		mapHTTPError(&res, err)
		return res
	}

	var quotaResp struct {
		Buckets []struct {
			ModelID           string   `json:"modelId"`
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(quotaBody, &quotaResp); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode gemini quota: %w", err).Error()
		return res
	}

	type famAgg struct {
		family   string
		minRem   float64
		resetsAt time.Time
		seen     bool
	}
	order := []string{"Pro", "Flash", "Flash Lite"}
	byFamily := map[string]*famAgg{}
	for _, b := range quotaResp.Buckets {
		if b.RemainingFraction == nil {
			continue
		}
		ml := strings.ToLower(b.ModelID)
		var family string
		switch {
		case strings.Contains(ml, "flash-lite") || strings.Contains(ml, "lite"):
			family = "Flash Lite"
		case strings.Contains(ml, "flash"):
			family = "Flash"
		case strings.Contains(ml, "pro"):
			family = "Pro"
		default:
			family = b.ModelID
		}
		agg, ok := byFamily[family]
		if !ok {
			agg = &famAgg{family: family, minRem: 1.0}
			byFamily[family] = agg
		}
		if !agg.seen || *b.RemainingFraction < agg.minRem {
			agg.minRem = *b.RemainingFraction
			agg.resetsAt = parseISO8601(b.ResetTime)
			agg.seen = true
		}
	}

	emitted := map[string]bool{}
	for _, fam := range order {
		if agg, ok := byFamily[fam]; ok && agg.seen {
			emitted[fam] = true
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       strings.ToLower(strings.ReplaceAll(fam, " ", "_")),
				Kind:     WindowDaily,
				Model:    fam,
				Used:     clampPct((1.0 - agg.minRem) * 100.0),
				ResetsAt: agg.resetsAt,
				Duration: 24 * time.Hour,
			})
		}
	}
	for fam, agg := range byFamily {
		if emitted[fam] || !agg.seen {
			continue
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       strings.ToLower(strings.ReplaceAll(fam, " ", "_")),
			Kind:     WindowDaily,
			Model:    fam,
			Used:     clampPct((1.0 - agg.minRem) * 100.0),
			ResetsAt: agg.resetsAt,
			Duration: 24 * time.Hour,
		})
	}

	if len(res.Windows) == 0 {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 5. Cursor ---

func fetchCursor(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderCursor)
	dbPath := cursorStateDBPath(env.Home, env.Env)
	detected := res.Installed || pathExists(dbPath)

	token, found, err := querySQLiteString(ctx, dbPath, "SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken' LIMIT 1")
	if err != nil && !os.IsNotExist(err) {
		detected = true
	}
	token = trimSQLiteToken(token)
	if !found || token == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		if errors.Is(err, errWALWithoutSHM) {
			res.Status = StatusUnavailable
			res.Err = err.Error()
			return res
		}
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	claims := parseJWTClaims(token)
	if jwtExpired(claims, now) {
		res.Status = StatusExpired
		return res
	}
	userID := ""
	if sub, ok := claims["sub"].(string); ok {
		if _, after, hasSep := strings.Cut(sub, "|"); hasSep {
			userID = after
		} else {
			userID = sub
		}
	}
	if userID == "" {
		res.Status = StatusSignedOut
		return res
	}

	endpoint := env.baseURL(ProviderCursor, "https://cursor.com") + "/api/usage-summary"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	cookieVal := url.QueryEscape(userID) + "%3A%3A" + url.QueryEscape(token)
	req.Header.Set("Cookie", "WorkosCursorSessionToken="+cookieVal)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hearth")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type cursorPlanUsage struct {
		Used             *float64 `json:"used"`
		Limit            *float64 `json:"limit"`
		TotalPercentUsed *float64 `json:"totalPercentUsed"`
		AutoPercentUsed  *float64 `json:"autoPercentUsed"`
		APIPercentUsed   *float64 `json:"apiPercentUsed"`
		Enabled          bool     `json:"enabled"`
	}
	var payload struct {
		MembershipType    string `json:"membershipType"`
		BillingCycleStart string `json:"billingCycleStart"`
		BillingCycleEnd   string `json:"billingCycleEnd"`
		IndividualUsage   *struct {
			Plan     *cursorPlanUsage `json:"plan"`
			Overall  *cursorPlanUsage `json:"overall"`
			OnDemand *cursorPlanUsage `json:"onDemand"`
		} `json:"individualUsage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode cursor usage: %w", err).Error()
		return res
	}

	res.Plan = formatPlanName(payload.MembershipType)
	cycleStart := parseISO8601(payload.BillingCycleStart)
	cycleEnd := parseISO8601(payload.BillingCycleEnd)
	dur := 30 * 24 * time.Hour
	if !cycleStart.IsZero() && !cycleEnd.IsZero() && cycleEnd.After(cycleStart) {
		dur = max(cycleEnd.Sub(cycleStart), 24*time.Hour)
	}

	var plan, planUsage *cursorPlanUsage
	if payload.IndividualUsage != nil {
		plan = payload.IndividualUsage.Plan
		if plan != nil {
			planUsage = plan
		} else if payload.IndividualUsage.Overall != nil {
			planUsage = payload.IndividualUsage.Overall
		}
	}
	if planUsage != nil {
		var usedPct float64
		hasPct := false
		switch {
		case plan != nil && plan.TotalPercentUsed != nil:
			usedPct = clampPct(*plan.TotalPercentUsed)
			hasPct = true
		case plan != nil && plan.AutoPercentUsed != nil && plan.APIPercentUsed != nil:
			usedPct = clampPct((*plan.AutoPercentUsed + *plan.APIPercentUsed) / 2.0)
			hasPct = true
		case planUsage.TotalPercentUsed != nil:
			usedPct = clampPct(*planUsage.TotalPercentUsed)
			hasPct = true
		case planUsage.Used != nil && planUsage.Limit != nil && *planUsage.Limit > 0:
			usedPct = clampPct((*planUsage.Used / *planUsage.Limit) * 100.0)
			hasPct = true
		case plan != nil && plan.APIPercentUsed != nil:
			usedPct = clampPct(*plan.APIPercentUsed)
			hasPct = true
		case plan != nil && plan.AutoPercentUsed != nil:
			usedPct = clampPct(*plan.AutoPercentUsed)
			hasPct = true
		}
		if hasPct {
			win := QuotaWindow{
				ID:       "monthly_plan",
				Kind:     WindowMonthly,
				Used:     usedPct,
				ResetsAt: cycleEnd,
				Duration: dur,
			}
			if planUsage.Used != nil && planUsage.Limit != nil && *planUsage.Limit > 0 {
				win.DollarsUsed = new(*planUsage.Used / 100.0)
				win.DollarsLimit = new(*planUsage.Limit / 100.0)
			}
			res.Windows = append(res.Windows, win)
		}
	}
	if payload.IndividualUsage != nil && payload.IndividualUsage.OnDemand != nil && payload.IndividualUsage.OnDemand.Enabled {
		od := payload.IndividualUsage.OnDemand
		ci := &CreditsInfo{}
		if od.Used != nil {
			ci.UsedUSD = new(*od.Used / 100.0)
		}
		if od.Limit != nil && *od.Limit > 0 {
			ci.LimitUSD = new(*od.Limit / 100.0)
		}
		if ci.UsedUSD != nil || ci.LimitUSD != nil {
			res.Credits = ci
		}
	}

	if len(res.Windows) == 0 && res.Credits == nil {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 6. GitHub Copilot ---

func fetchCopilot(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderCopilot)
	hasHosts := pathExists(filepath.Join(env.Home, ".config", "gh", "hosts.yml"))
	hasTestEndpoint := env.Endpoints != nil && env.Endpoints[ProviderCopilot] != ""
	detected := res.Installed || hasHosts || env.hasExecutable("gh")

	if env.Runner == nil || (!detected && !hasTestEndpoint) {
		if !detected {
			res.Status = StatusNotFound
		} else {
			res.Status = StatusSignedOut
		}
		return res
	}

	// Strip token and config overrides so gh reads the user's ordinary saved account.
	cleanEnv := env.Env.Without(
		"GH_TOKEN", "GITHUB_TOKEN",
		"GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN",
		"GH_HOST", "GH_CONFIG_DIR", "XDG_CONFIG_HOME",
	).With("GH_PROMPT_DISABLED", "1")

	cmdRes, err := env.Runner.Run(ctx, run.Cmd{
		Args:      []string{"gh", "auth", "token", "--hostname", "github.com", "--no-refresh"},
		Env:       cleanEnv,
		Timeout:   3 * time.Second,
		MaxOutput: 16 << 10,
	})
	token := ""
	if err == nil && cmdRes.ExitCode == 0 {
		token = strings.TrimSpace(string(cmdRes.Stdout))
	}
	if token == "" {
		if !res.Installed && !hasHosts {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	endpoint := env.baseURL(ProviderCopilot, "https://api.github.com") + "/copilot_internal/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", "vscode/1.99.0")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")
	req.Header.Set("User-Agent", "GitHubCopilotChat/0.26.7")
	req.Header.Set("X-Github-Api-Version", "2025-04-01")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type copilotSnap struct {
		PercentRemaining any  `json:"percent_remaining"`
		Unlimited        bool `json:"unlimited"`
		Entitlement      any  `json:"entitlement"`
		Remaining        any  `json:"remaining"`
		CreditsUsed      any  `json:"credits_used"`
	}
	var payload struct {
		CopilotPlan       string             `json:"copilot_plan"`
		QuotaResetDate    string             `json:"quota_reset_date"`
		QuotaResetDateUTC string             `json:"quota_reset_date_utc"`
		TokenBasedBilling bool               `json:"token_based_billing"`
		MonthlyQuotas     map[string]float64 `json:"monthly_quotas"`
		LimitedUserQuotas map[string]float64 `json:"limited_user_quotas"`
		QuotaSnapshots    *struct {
			PremiumInteractions *copilotSnap `json:"premium_interactions"`
			Chat                *copilotSnap `json:"chat"`
			Completions         *copilotSnap `json:"completions"`
		} `json:"quota_snapshots"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode copilot usage: %w", err).Error()
		return res
	}

	res.Plan = formatPlanName(payload.CopilotPlan)
	resetStr := payload.QuotaResetDateUTC
	if resetStr == "" {
		resetStr = payload.QuotaResetDate
	}
	resetsAt := parseISO8601(resetStr)

	hasUnlimited := false
	addSnap := func(key, id, model string, s *copilotSnap) {
		if s != nil && s.Unlimited {
			hasUnlimited = true
			return
		}
		var rem float64
		hasVal := false
		if s != nil {
			ent, hasEnt := parseAnyFloat(s.Entitlement)
			remVal, hasRem := parseAnyFloat(s.Remaining)
			if !(hasEnt && hasRem && ent == 0 && remVal == 0) {
				if pr, ok := parseAnyFloat(s.PercentRemaining); ok {
					rem = pr
					hasVal = true
				} else if hasEnt && ent > 0 && hasRem {
					rem = (remVal / ent) * 100.0
					hasVal = true
				}
			}
		}
		// Fallback for older free plans that publish monthly_quotas + limited_user_quotas.
		if !hasVal && key != "premium_interactions" {
			if total, okT := payload.MonthlyQuotas[key]; okT && total > 0 {
				if left, okL := payload.LimitedUserQuotas[key]; okL && left >= 0 {
					rem = (left / total) * 100.0
					hasVal = true
				}
			}
		}
		if !hasVal {
			return
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     WindowMonthly,
			Model:    model,
			Used:     clampPct(100.0 - rem),
			ResetsAt: resetsAt,
			Duration: 30 * 24 * time.Hour,
		})
	}
	var premSnap, chatSnap, compSnap *copilotSnap
	if payload.QuotaSnapshots != nil {
		premSnap = payload.QuotaSnapshots.PremiumInteractions
		chatSnap = payload.QuotaSnapshots.Chat
		compSnap = payload.QuotaSnapshots.Completions
	}
	addSnap("premium_interactions", "premium", "Premium", premSnap)
	addSnap("chat", "chat", "Chat", chatSnap)
	addSnap("completions", "completions", "Completions", compSnap)

	var creditsUsed *float64
	if premSnap != nil {
		if v, ok := parseAnyFloat(premSnap.CreditsUsed); ok && v >= 0 {
			creditsUsed = new(v)
		}
	}
	if creditsUsed == nil && chatSnap != nil {
		if v, ok := parseAnyFloat(chatSnap.CreditsUsed); ok && v >= 0 {
			creditsUsed = new(v)
		}
	}
	if hasUnlimited || payload.TokenBasedBilling || (creditsUsed != nil && *creditsUsed > 0) {
		ci := &CreditsInfo{Unlimited: hasUnlimited && len(res.Windows) == 0}
		if creditsUsed != nil && (*creditsUsed > 0 || payload.TokenBasedBilling || hasUnlimited) {
			ci.UsedUSD = creditsUsed
		}
		res.Credits = ci
	}

	if len(res.Windows) == 0 && res.Credits == nil {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = now
	return res
}

// --- 7. JetBrains AI ---

type jetBrainsReading struct {
	plan     string
	usedPct  float64
	resetsAt time.Time
	duration time.Duration
	date     time.Time
}

func fetchJetBrains(_ context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderJetBrains)
	cfgRoots := jetBrainsConfigRoots(env.Home, env.Env)
	logRoots := jetBrainsLogRoots(env.Home, env.Env)

	type jbCandidate struct {
		name    string
		xmlPath string
		xmlMod  time.Time
		logPath string
		logMod  time.Time
	}
	byKey := map[string]*jbCandidate{}

	for ri, root := range cfgRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			cand := filepath.Join(root, ent.Name(), "options", "AIAssistantQuotaManager2.xml")
			info, err := os.Stat(cand)
			if err != nil || info.IsDir() {
				continue
			}
			k := fmt.Sprintf("%d:%s", ri, ent.Name())
			c := byKey[k]
			if c == nil {
				c = &jbCandidate{name: ent.Name()}
				byKey[k] = c
			}
			c.xmlPath = cand
			c.xmlMod = info.ModTime()
		}
	}
	for ri, root := range logRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			cand := filepath.Join(root, ent.Name(), "idea.log")
			info, err := os.Stat(cand)
			if err != nil || info.IsDir() {
				continue
			}
			k := fmt.Sprintf("%d:%s", ri, ent.Name())
			c := byKey[k]
			if c == nil {
				c = &jbCandidate{name: ent.Name()}
				byKey[k] = c
			}
			c.logPath = cand
			c.logMod = info.ModTime()
		}
	}

	// Prefer the IDE installation with the newest XML, falling back to the newest idea.log.
	var chosen *jbCandidate
	for _, c := range byKey {
		if c.xmlPath != "" {
			if chosen == nil || chosen.xmlPath == "" || c.xmlMod.After(chosen.xmlMod) {
				chosen = c
			}
		} else if chosen == nil || (chosen.xmlPath == "" && c.logMod.After(chosen.logMod)) {
			chosen = c
		}
	}

	if chosen == nil {
		if !res.Installed {
			res.Status = StatusNotFound
			return res
		}
		res.Status = StatusNoData
		return res
	}

	res.Installed = true
	var xmlReading *jetBrainsReading
	if chosen.xmlPath != "" {
		if data, err := readCappedFile(chosen.xmlPath, maxConfigFileBytes); err == nil {
			if r, ok := parseJetBrainsXMLReading(data, chosen.xmlMod); ok {
				xmlReading = &r
			}
		}
	}
	var logReading *jetBrainsReading
	if chosen.logPath != "" {
		if tail, err := readFileTail(chosen.logPath, maxConfigFileBytes); err == nil && tail != "" {
			if r, ok := parseJetBrainsLogReading(tail); ok {
				logReading = &r
			}
		}
	}

	selected := xmlReading
	if logReading != nil && (selected == nil || logReading.date.After(selected.date)) {
		selected = logReading
	}
	if selected == nil {
		res.Status = StatusNoData
		return res
	}

	dur := selected.duration
	if dur <= 0 {
		dur = 30 * 24 * time.Hour
	}
	res.Plan = formatPlanName(selected.plan)
	res.Windows = NormalizeWindows([]QuotaWindow{{
		ID:       "monthly",
		Kind:     WindowMonthly,
		Used:     clampPct(selected.usedPct),
		ResetsAt: selected.resetsAt,
		Duration: dur,
	}})
	res.Status = StatusOK
	res.UpdatedAt = selected.date
	return res
}

func parseJetBrainsXMLReading(data []byte, modTime time.Time) (jetBrainsReading, bool) {
	quotaJSON, refillJSON := parseJetBrainsQuotaXML(data)
	if quotaJSON == "" {
		return jetBrainsReading{}, false
	}
	var qi struct {
		Type        string `json:"type"`
		Current     any    `json:"current"`
		Maximum     any    `json:"maximum"`
		Until       string `json:"until"`
		TariffQuota *struct {
			Current any `json:"current"`
			Maximum any `json:"maximum"`
		} `json:"tariffQuota"`
	}
	if err := json.Unmarshal([]byte(quotaJSON), &qi); err != nil {
		return jetBrainsReading{}, false
	}
	cur, okCur := parseAnyFloat(qi.Current)
	maxVal, okMax := parseAnyFloat(qi.Maximum)
	if (!okCur || !okMax) && qi.TariffQuota != nil {
		cur, okCur = parseAnyFloat(qi.TariffQuota.Current)
		maxVal, okMax = parseAnyFloat(qi.TariffQuota.Maximum)
	}
	if !okCur || !okMax || maxVal <= 0 || math.IsNaN(cur) || math.IsNaN(maxVal) {
		return jetBrainsReading{}, false
	}
	resetsAt := parseISO8601(qi.Until)
	if refillJSON != "" {
		var nr struct {
			Next string `json:"next"`
		}
		if json.Unmarshal([]byte(refillJSON), &nr) == nil {
			if t := parseISO8601(nr.Next); !t.IsZero() {
				resetsAt = t
			}
		}
	}
	return jetBrainsReading{
		plan:     qi.Type,
		usedPct:  (cur / maxVal) * 100.0,
		resetsAt: resetsAt,
		duration: 30 * 24 * time.Hour,
		date:     modTime,
	}, true
}

const (
	jbQuotaMarker  = "QuotaManager2Impl - New quota state is: "
	jbRefillMarker = "QuotaManager2Impl - New quota refill state is: "
	jbJunieMarker  = "AiaActivationTransmitter - Received message from junie: quota_update:"
)

func parseJetBrainsLogReading(tail string) (jetBrainsReading, bool) {
	lines := strings.Split(tail, "\n")
	var reading *jetBrainsReading
	var refillReset time.Time
	hasRefill := false

	for _, rawLine := range slices.Backward(lines) {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" {
			continue
		}
		if _, after, ok := strings.Cut(line, jbQuotaMarker); ok {
			if strings.HasPrefix(after, "Error(") {
				continue
			}
			if reading != nil {
				break
			}
			ts, okTS := parseJetBrainsLogTimestamp(line)
			if !okTS {
				return jetBrainsReading{}, false
			}
			cur, maxVal, okVals := parseJetBrainsKotlinQuota(after)
			if !okVals || maxVal <= 0 {
				return jetBrainsReading{}, false
			}
			reading = &jetBrainsReading{
				usedPct:  (cur / maxVal) * 100.0,
				resetsAt: refillReset,
				duration: 30 * 24 * time.Hour,
				date:     ts,
			}
		} else if _, after, ok := strings.Cut(line, jbJunieMarker); ok {
			if reading != nil {
				break
			}
			ts, okTS := parseJetBrainsLogTimestamp(line)
			if !okTS {
				return jetBrainsReading{}, false
			}
			var q struct {
				Type        string `json:"type"`
				Current     any    `json:"current"`
				Maximum     any    `json:"maximum"`
				TariffQuota *struct {
					Current any `json:"current"`
					Maximum any `json:"maximum"`
				} `json:"tariffQuota"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(after)), &q) != nil {
				return jetBrainsReading{}, false
			}
			cur, okCur := parseAnyFloat(q.Current)
			maxVal, okMax := parseAnyFloat(q.Maximum)
			if (!okCur || !okMax) && q.TariffQuota != nil {
				cur, okCur = parseAnyFloat(q.TariffQuota.Current)
				maxVal, okMax = parseAnyFloat(q.TariffQuota.Maximum)
			}
			if !okCur || !okMax || maxVal <= 0 {
				return jetBrainsReading{}, false
			}
			return jetBrainsReading{
				plan:     q.Type,
				usedPct:  (cur / maxVal) * 100.0,
				duration: 30 * 24 * time.Hour,
				date:     ts,
			}, true
		} else if !hasRefill {
			if _, after, ok := strings.Cut(line, jbRefillMarker); ok {
				if next := extractKotlinField(after, "next"); next != "" {
					if t := parseISO8601(next); !t.IsZero() {
						refillReset = t
						hasRefill = true
					}
				}
			}
		}
		if reading != nil && hasRefill {
			reading.resetsAt = refillReset
			break
		}
	}
	if reading == nil {
		return jetBrainsReading{}, false
	}
	return *reading, true
}

func parseJetBrainsLogTimestamp(line string) (time.Time, bool) {
	if len(line) < 23 {
		return time.Time{}, false
	}
	raw := strings.Replace(line[:23], ",", ".", 1)
	t, err := time.ParseInLocation("2006-01-02 15:04:05.000", raw, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseJetBrainsKotlinQuota(state string) (cur, maxVal float64, ok bool) {
	// Extract tariffQuota=(QuotaDetails|Details)(... current=..., maximum=...)
	sub := state
	if _, after, found := strings.Cut(sub, "tariffQuota="); found {
		sub = after
	}
	curStr := extractKotlinField(sub, "current")
	maxStr := extractKotlinField(sub, "maximum")
	if curStr == "" || maxStr == "" {
		return 0, 0, false
	}
	c, errC := strconv.ParseFloat(curStr, 64)
	m, errM := strconv.ParseFloat(maxStr, 64)
	if errC != nil || errM != nil || m <= 0 || math.IsNaN(c) || math.IsNaN(m) {
		return 0, 0, false
	}
	return c, m, true
}

func extractKotlinField(s, key string) string {
	_, rest, ok := strings.Cut(s, key+"=")
	if !ok {
		return ""
	}
	end := strings.IndexAny(rest, ",)")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func parseJetBrainsQuotaXML(data []byte) (quotaJSON, refillJSON string) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Entity = map[string]string{} // disable custom XML entity expansion
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "option" {
			continue
		}
		var name, val string
		for _, attr := range se.Attr {
			switch attr.Name.Local {
			case "name":
				name = attr.Value
			case "value":
				val = attr.Value
			}
		}
		switch name {
		case "quotaInfo":
			quotaJSON = html.UnescapeString(val)
		case "nextRefill":
			refillJSON = html.UnescapeString(val)
		}
	}
	return quotaJSON, refillJSON
}
