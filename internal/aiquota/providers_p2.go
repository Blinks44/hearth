package aiquota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

// --- 8. Grok CLI ---

func fetchGrok(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderGrok)
	grokDir := filepath.Join(env.Home, ".grok")
	authFile := filepath.Join(grokDir, "auth.json")
	detected := res.Installed || pathExists(grokDir)

	var auth struct {
		AccessToken string `json:"access_token"`
		Token       string `json:"token"`
	}
	if data, err := readCappedFile(authFile, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &auth)
	}
	tok := strings.TrimSpace(auth.AccessToken)
	if tok == "" {
		tok = strings.TrimSpace(auth.Token)
	}
	if tok == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	endpoint := env.baseURL(ProviderGrok, "https://cli-chat-proxy.grok.com") + "/v1/billing?format=credits"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("x-xai-token-auth", "xai-grok-cli")
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type grokMoney struct {
		Val any `json:"val"`
	}
	var payload struct {
		Plan             string `json:"plan"`
		SubscriptionTier string `json:"subscriptionTier"`
		Config           *struct {
			CreditUsagePercent any    `json:"creditUsagePercent"`
			SubscriptionTier   string `json:"subscriptionTier"`
			BillingPeriodStart any    `json:"billingPeriodStart"`
			BillingPeriodEnd   any    `json:"billingPeriodEnd"`
			CurrentPeriod      *struct {
				Start any `json:"start"`
				End   any `json:"end"`
			} `json:"currentPeriod"`
			OnDemandCap  *grokMoney `json:"onDemandCap"`
			OnDemandUsed *grokMoney `json:"onDemandUsed"`
		} `json:"config"`
		UsedCredits    *float64 `json:"used_credits"`
		TotalCredits   *float64 `json:"total_credits"`
		Balance        *float64 `json:"balance"`
		RemainingPct   *float64 `json:"remaining_percent"`
		ResetAt        any      `json:"reset_at"`
		BillingCycleEn any      `json:"billing_cycle_end"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode grok billing: %w", err).Error()
		return res
	}

	tier := payload.SubscriptionTier
	if payload.Config != nil && payload.Config.SubscriptionTier != "" {
		tier = payload.Config.SubscriptionTier
	}
	if tier == "" {
		tier = payload.Plan
	}
	res.Plan = formatPlanName(tier)

	var resetsAt, periodStart time.Time
	if payload.Config != nil {
		if payload.Config.CurrentPeriod != nil {
			periodStart = parseAnyTime(payload.Config.CurrentPeriod.Start)
			resetsAt = parseAnyTime(payload.Config.CurrentPeriod.End)
		}
		if periodStart.IsZero() {
			periodStart = parseAnyTime(payload.Config.BillingPeriodStart)
		}
		if resetsAt.IsZero() {
			resetsAt = parseAnyTime(payload.Config.BillingPeriodEnd)
		}
	}
	if resetsAt.IsZero() {
		resetsAt = parseAnyTime(payload.ResetAt)
	}
	if resetsAt.IsZero() {
		resetsAt = parseAnyTime(payload.BillingCycleEn)
	}
	dur := 30 * 24 * time.Hour
	if !periodStart.IsZero() && !resetsAt.IsZero() && resetsAt.After(periodStart) {
		dur = resetsAt.Sub(periodStart)
	}

	if payload.Config != nil {
		if pct, ok := parseAnyFloat(payload.Config.CreditUsagePercent); ok {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       "monthly",
				Kind:     WindowMonthly,
				Used:     clampPct(pct),
				ResetsAt: resetsAt,
				Duration: dur,
			})
		}
		if payload.Config.OnDemandCap != nil && payload.Config.OnDemandUsed != nil {
			capVal, okCap := parseAnyFloat(payload.Config.OnDemandCap.Val)
			usedVal, okUsed := parseAnyFloat(payload.Config.OnDemandUsed.Val)
			if okCap && okUsed && capVal > 0 {
				res.Windows = append(res.Windows, QuotaWindow{
					ID:           "on_demand",
					Kind:         WindowCredits,
					Model:        "On-demand",
					Used:         clampPct((usedVal / capVal) * 100.0),
					ResetsAt:     resetsAt,
					Duration:     dur,
					DollarsUsed:  new(usedVal),
					DollarsLimit: new(capVal),
				})
			}
		}
	}

	if len(res.Windows) == 0 {
		if payload.UsedCredits != nil && payload.TotalCredits != nil && *payload.TotalCredits > 0 {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       "monthly",
				Kind:     WindowMonthly,
				Used:     clampPct((*payload.UsedCredits / *payload.TotalCredits) * 100.0),
				ResetsAt: resetsAt,
				Duration: dur,
			})
		} else if payload.RemainingPct != nil {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       "monthly",
				Kind:     WindowMonthly,
				Used:     clampPct(100.0 - *payload.RemainingPct),
				ResetsAt: resetsAt,
				Duration: dur,
			})
		} else if payload.Balance != nil {
			res.Credits = &CreditsInfo{RemainingUSD: new(*payload.Balance)}
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

// --- 9. Windsurf ---

func fetchWindsurf(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderWindsurf)
	dbPath := windsurfStateDBPath(env.Home, env.Env)
	detected := res.Installed || pathExists(dbPath)

	raw, found, err := querySQLiteString(ctx, dbPath, "SELECT value FROM ItemTable WHERE key = 'windsurf.settings.cachedPlanInfo' LIMIT 1")
	if errors.Is(err, errWALWithoutSHM) {
		res.Installed = true
		res.Status = StatusUnavailable
		return res
	}
	if err != nil && !os.IsNotExist(err) {
		detected = true
	}
	if !found || strings.TrimSpace(raw) == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusNoData
		return res
	}

	res.Installed = true
	type wsQuota struct {
		Used        *float64 `json:"used"`
		Limit       *float64 `json:"limit"`
		PercentUsed *float64 `json:"percentUsed"`
		ResetAt     any      `json:"resetAt"`
	}
	var payload struct {
		PlanName string `json:"planName"`
		PlanInfo *struct {
			PlanName string `json:"planName"`
		} `json:"planInfo"`
		QuotaUsage *struct {
			DailyRemainingPercent  *float64 `json:"dailyRemainingPercent"`
			DailyResetAtUnix       any      `json:"dailyResetAtUnix"`
			WeeklyRemainingPercent *float64 `json:"weeklyRemainingPercent"`
			WeeklyResetAtUnix      any      `json:"weeklyResetAtUnix"`
			Daily                  *wsQuota `json:"daily"`
			Weekly                 *wsQuota `json:"weekly"`
		} `json:"quotaUsage"`
		Usage *struct {
			Messages        json.RawMessage `json:"messages"`
			UsedMessages    *float64        `json:"usedMessages"`
			FlowActions     json.RawMessage `json:"flowActions"`
			UsedFlowActions *float64        `json:"usedFlowActions"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		res.Status = StatusNoData
		return res
	}
	planName := payload.PlanName
	if planName == "" && payload.PlanInfo != nil {
		planName = payload.PlanInfo.PlanName
	}
	res.Plan = formatPlanName(planName)

	addWS := func(id, model string, kind WindowKind, dur time.Duration, q *wsQuota) {
		if q == nil {
			return
		}
		var used float64
		switch {
		case q.PercentUsed != nil:
			used = clampPct(*q.PercentUsed)
		case q.Used != nil && q.Limit != nil && *q.Limit > 0:
			used = clampPct((*q.Used / *q.Limit) * 100.0)
		default:
			return
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     kind,
			Model:    model,
			Used:     used,
			ResetsAt: parseAnyTime(q.ResetAt),
			Duration: dur,
		})
	}
	if payload.QuotaUsage != nil {
		if payload.QuotaUsage.DailyRemainingPercent != nil {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       "daily",
				Kind:     WindowDaily,
				Used:     clampPct(100.0 - *payload.QuotaUsage.DailyRemainingPercent),
				ResetsAt: parseAnyTime(payload.QuotaUsage.DailyResetAtUnix),
				Duration: 24 * time.Hour,
			})
		} else {
			addWS("daily", "", WindowDaily, 24*time.Hour, payload.QuotaUsage.Daily)
		}
		if payload.QuotaUsage.WeeklyRemainingPercent != nil {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       "weekly",
				Kind:     WindowWeekly,
				Used:     clampPct(100.0 - *payload.QuotaUsage.WeeklyRemainingPercent),
				ResetsAt: parseAnyTime(payload.QuotaUsage.WeeklyResetAtUnix),
				Duration: 7 * 24 * time.Hour,
			})
		} else {
			addWS("weekly", "", WindowWeekly, 7*24*time.Hour, payload.QuotaUsage.Weekly)
		}
	}
	if len(res.Windows) == 0 && payload.Usage != nil {
		addWSUsage := func(id, model string, rawTotal json.RawMessage, usedPtr *float64) {
			if len(rawTotal) == 0 {
				return
			}
			var total float64
			if json.Unmarshal(rawTotal, &total) == nil && total > 0 && usedPtr != nil {
				res.Windows = append(res.Windows, QuotaWindow{
					ID:       id,
					Kind:     WindowMonthly,
					Model:    model,
					Used:     clampPct((*usedPtr / total) * 100.0),
					Duration: 30 * 24 * time.Hour,
				})
				return
			}
			var nested wsQuota
			if json.Unmarshal(rawTotal, &nested) == nil {
				addWS(id, model, WindowMonthly, 30*24*time.Hour, &nested)
			}
		}
		addWSUsage("messages", "Messages", payload.Usage.Messages, payload.Usage.UsedMessages)
		addWSUsage("flows", "Flows", payload.Usage.FlowActions, payload.Usage.UsedFlowActions)
	}
	if len(res.Windows) == 0 {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.SourceDateUnknown = true
	res.UpdatedAt = env.now()
	return res
}

// --- 10. Cline ---

func fetchCline(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderCline)
	clineDir := filepath.Join(env.Home, ".cline")
	provFile := filepath.Join(clineDir, "data", "settings", "providers.json")
	detected := res.Installed || pathExists(clineDir)

	var cfg struct {
		ClineAPIKey string `json:"clineApiKey"`
		AccessToken string `json:"accessToken"`
		Cline       *struct {
			AccessToken string `json:"accessToken"`
			APIKey      string `json:"apiKey"`
		} `json:"cline"`
		Providers *struct {
			Cline *struct {
				Settings *struct {
					Auth *struct {
						AccessToken string `json:"accessToken"`
					} `json:"auth"`
				} `json:"settings"`
				Auth *struct {
					AccessToken string `json:"accessToken"`
				} `json:"auth"`
			} `json:"cline"`
		} `json:"providers"`
	}
	if data, err := readCappedFile(provFile, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &cfg)
	}

	var tok string
	fromWorkOS := false
	if cfg.Providers != nil && cfg.Providers.Cline != nil {
		if cfg.Providers.Cline.Settings != nil && cfg.Providers.Cline.Settings.Auth != nil {
			tok = strings.TrimSpace(cfg.Providers.Cline.Settings.Auth.AccessToken)
			fromWorkOS = tok != ""
		}
		if tok == "" && cfg.Providers.Cline.Auth != nil {
			tok = strings.TrimSpace(cfg.Providers.Cline.Auth.AccessToken)
			fromWorkOS = tok != ""
		}
	}
	if tok == "" {
		tok = strings.TrimSpace(cfg.AccessToken)
	}
	if tok == "" {
		tok = strings.TrimSpace(cfg.ClineAPIKey)
	}
	if tok == "" && cfg.Cline != nil {
		tok = strings.TrimSpace(cfg.Cline.AccessToken)
		if tok == "" {
			tok = strings.TrimSpace(cfg.Cline.APIKey)
		}
	}
	if tok == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	rawJWT := strings.TrimPrefix(tok, "workos:")
	if jwtExpired(parseJWTClaims(rawJWT), now) {
		res.Status = StatusExpired
		return res
	}
	authHeader := tok
	if (fromWorkOS || strings.Count(tok, ".") == 2) && !strings.HasPrefix(tok, "workos:") {
		authHeader = "workos:" + tok
	}

	endpoint := env.baseURL(ProviderCline, "https://api.cline.bot") + "/api/v1/users/me/plan/usage-limits"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+authHeader)
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type clineWin struct {
		UsedPercent *float64 `json:"used_percent"`
		Utilization *float64 `json:"utilization"`
		ResetsAt    any      `json:"resets_at"`
	}
	type clineLimitItem struct {
		Type        string   `json:"type"`
		PercentUsed *float64 `json:"percentUsed"`
		UsedPercent *float64 `json:"usedPercent"`
		ResetsAt    any      `json:"resetsAt"`
	}
	var payload struct {
		Success *bool  `json:"success"`
		Plan    string `json:"plan"`
		Data    *struct {
			Plan   string           `json:"plan"`
			Limits []clineLimitItem `json:"limits"`
		} `json:"data"`
		FiveHour *clineWin `json:"five_hour"`
		Weekly   *clineWin `json:"weekly"`
		Monthly  *clineWin `json:"monthly"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode cline limits: %w", err).Error()
		return res
	}
	plan := payload.Plan
	if payload.Data != nil && payload.Data.Plan != "" {
		plan = payload.Data.Plan
	}
	res.Plan = formatPlanName(plan)

	if payload.Data != nil && len(payload.Data.Limits) > 0 {
		for _, item := range payload.Data.Limits {
			pct := item.PercentUsed
			if pct == nil {
				pct = item.UsedPercent
			}
			if pct == nil {
				continue
			}
			kind, dur := clineWindowMeta(item.Type)
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       strings.ToLower(strings.TrimSpace(item.Type)),
				Kind:     kind,
				Used:     clampPct(*pct),
				ResetsAt: parseAnyTime(item.ResetsAt),
				Duration: dur,
			})
		}
	}

	if len(res.Windows) == 0 {
		addCW := func(id string, kind WindowKind, dur time.Duration, cw *clineWin) {
			if cw == nil {
				return
			}
			pct := cw.UsedPercent
			if pct == nil {
				pct = cw.Utilization
			}
			if pct == nil {
				return
			}
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       id,
				Kind:     kind,
				Used:     clampPct(*pct),
				ResetsAt: parseAnyTime(cw.ResetsAt),
				Duration: dur,
			})
		}
		addCW("five_hour", WindowSession, 5*time.Hour, payload.FiveHour)
		addCW("weekly", WindowWeekly, 7*24*time.Hour, payload.Weekly)
		addCW("monthly", WindowMonthly, 30*24*time.Hour, payload.Monthly)
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

func clineWindowMeta(typ string) (WindowKind, time.Duration) {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "five_hour", "5h", "session":
		return WindowSession, 5 * time.Hour
	case "daily", "day", "24h":
		return WindowDaily, 24 * time.Hour
	case "weekly", "week", "7d":
		return WindowWeekly, 7 * 24 * time.Hour
	case "monthly", "month", "30d":
		return WindowMonthly, 30 * 24 * time.Hour
	default:
		return WindowMonthly, 30 * 24 * time.Hour
	}
}

// --- 11. Kimi Code ---

func fetchKimi(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderKimi)
	candidates := []string{
		filepath.Join(env.Home, ".kimi-code", "credentials", "kimi-code.json"),
		filepath.Join(env.Home, ".kimi", "credentials", "kimi-code.json"),
	}
	detected := res.Installed || pathExists(filepath.Join(env.Home, ".kimi-code")) || pathExists(filepath.Join(env.Home, ".kimi"))

	var cred struct {
		AccessToken string `json:"access_token"`
		APIBase     string `json:"api_base"`
		ExpiresAt   any    `json:"expires_at"`
	}
	for _, p := range candidates {
		if data, err := readCappedFile(p, maxConfigFileBytes); err == nil {
			detected = true
			if json.Unmarshal(data, &cred) == nil && strings.TrimSpace(cred.AccessToken) != "" {
				break
			}
		}
	}
	tok := strings.TrimSpace(cred.AccessToken)
	if tok == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	if exp := parseAnyTime(cred.ExpiresAt); !exp.IsZero() && exp.Before(now) {
		res.Status = StatusExpired
		return res
	}
	if jwtExpired(parseJWTClaims(tok), now) {
		res.Status = StatusExpired
		return res
	}

	defBase := strings.TrimRight(cred.APIBase, "/")
	if defBase == "" {
		defBase = "https://api.kimi.com/coding/v1"
	}
	endpoint := env.baseURL(ProviderKimi, defBase) + "/usages"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type kimiWin struct {
		Used      *float64 `json:"used"`
		Limit     *float64 `json:"limit"`
		UsedRatio *float64 `json:"used_ratio"`
		ResetTime any      `json:"resetTime"`
		ResetAt   any      `json:"reset_time"`
	}
	var payload struct {
		Plan string `json:"plan"`
		User *struct {
			Membership *struct {
				Level string `json:"level"`
			} `json:"membership"`
		} `json:"user"`
		Usages *struct {
			Limit5h    *kimiWin `json:"limit_5h"`
			Limit7d    *kimiWin `json:"limit_7d"`
			LimitMonth *kimiWin `json:"limit_month_total"`
		} `json:"usages"`
		Usage  *kimiWin `json:"usage"`
		Limits []struct {
			Window *struct {
				Duration int    `json:"duration"`
				TimeUnit string `json:"timeUnit"`
			} `json:"window"`
			Detail *kimiWin `json:"detail"`
		} `json:"limits"`
		Limit5h    *kimiWin `json:"limit_5h"`
		Limit7d    *kimiWin `json:"limit_7d"`
		LimitMonth *kimiWin `json:"limit_month_total"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode kimi usage: %w", err).Error()
		return res
	}

	plan := payload.Plan
	if plan == "" && payload.User != nil && payload.User.Membership != nil {
		lvl := strings.TrimSpace(payload.User.Membership.Level)
		if lvl != "" && !strings.HasPrefix(strings.ToUpper(lvl), "LEVEL_") {
			plan = lvl
		}
	}
	res.Plan = formatPlanName(plan)

	addKW := func(id string, kind WindowKind, dur time.Duration, kw *kimiWin) {
		if kw == nil {
			return
		}
		var used float64
		switch {
		case kw.UsedRatio != nil:
			v := *kw.UsedRatio
			if v <= 1.0 {
				v *= 100.0
			}
			used = clampPct(v)
		case kw.Used != nil && kw.Limit != nil && *kw.Limit > 0:
			used = clampPct((*kw.Used / *kw.Limit) * 100.0)
		default:
			return
		}
		reset := parseAnyTime(kw.ResetTime)
		if reset.IsZero() {
			reset = parseAnyTime(kw.ResetAt)
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     kind,
			Used:     used,
			ResetsAt: reset,
			Duration: dur,
		})
	}

	if payload.Usages != nil {
		addKW("limit_5h", WindowSession, 5*time.Hour, payload.Usages.Limit5h)
		addKW("limit_7d", WindowWeekly, 7*24*time.Hour, payload.Usages.Limit7d)
		addKW("limit_month", WindowMonthly, 30*24*time.Hour, payload.Usages.LimitMonth)
	}
	if len(res.Windows) == 0 && len(payload.Limits) > 0 {
		for i, item := range payload.Limits {
			if item.Detail == nil {
				continue
			}
			kind, dur := WindowSession, 5*time.Hour
			if item.Window != nil && item.Window.Duration > 0 {
				switch strings.ToUpper(item.Window.TimeUnit) {
				case "TIME_UNIT_MINUTE":
					dur = time.Duration(item.Window.Duration) * time.Minute
					kind = WindowSession
				case "TIME_UNIT_HOUR":
					dur = time.Duration(item.Window.Duration) * time.Hour
					if dur >= 24*time.Hour {
						kind = WindowDaily
					} else {
						kind = WindowSession
					}
				case "TIME_UNIT_DAY":
					dur = time.Duration(item.Window.Duration) * 24 * time.Hour
					if item.Window.Duration >= 28 {
						kind = WindowMonthly
					} else if item.Window.Duration >= 7 {
						kind = WindowWeekly
					} else {
						kind = WindowDaily
					}
				}
			}
			addKW(fmt.Sprintf("limit_%d", i), kind, dur, item.Detail)
		}
	}
	if len(res.Windows) == 0 {
		addKW("limit_5h", WindowSession, 5*time.Hour, payload.Limit5h)
		addKW("limit_7d", WindowWeekly, 7*24*time.Hour, payload.Limit7d)
		addKW("limit_month", WindowMonthly, 30*24*time.Hour, payload.LimitMonth)
	}
	if len(res.Windows) == 0 && payload.Usage != nil {
		addKW("weekly", WindowWeekly, 7*24*time.Hour, payload.Usage)
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

// --- 12. OpenCode Go ---

func fetchOpenCode(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderOpenCode)
	ocDir := filepath.Join(env.Home, ".local", "share", "opencode")
	dbPath := filepath.Join(ocDir, "opencode.db")
	authPath := filepath.Join(ocDir, "auth.json")
	detected := res.Installed || pathExists(ocDir)
	now := env.now()

	var token string
	var expired bool
	rows, dbErr := querySQLiteRows(ctx, dbPath,
		"SELECT integration_id, value FROM credential WHERE active = 1 AND integration_id IN ('opencode-go', 'opencode') ORDER BY CASE integration_id WHEN 'opencode-go' THEN 0 ELSE 1 END LIMIT 2",
		2,
	)
	if errors.Is(dbErr, errWALWithoutSHM) {
		res.Installed = true
		res.Status = StatusUnavailable
		return res
	}
	if dbErr == nil {
		detected = true
		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			var val struct {
				Type    string `json:"type"`
				Key     string `json:"key"`
				Access  string `json:"access"`
				Expires any    `json:"expires"`
			}
			if json.Unmarshal([]byte(row[1]), &val) != nil {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(val.Type)) {
			case "key", "api":
				if k := strings.TrimSpace(val.Key); k != "" {
					token = k
				}
			case "oauth":
				if acc := strings.TrimSpace(val.Access); acc != "" {
					if exp := parseAnyTime(val.Expires); !exp.IsZero() && exp.Before(now) {
						expired = true
						continue
					}
					token = acc
				}
			default:
				if k := strings.TrimSpace(val.Key); k != "" {
					token = k
				} else if acc := strings.TrimSpace(val.Access); acc != "" {
					token = acc
				}
			}
			if token != "" {
				break
			}
		}
	} else if !os.IsNotExist(dbErr) {
		detected = true
		// Backward-compatible schema fallback if older credential table has access_token column.
		if val, ok, _ := querySQLiteString(ctx, dbPath, "SELECT access_token FROM credential WHERE access_token != '' LIMIT 1"); ok {
			token = trimSQLiteToken(val)
		}
	}

	if token == "" {
		if data, err := readCappedFile(authPath, maxConfigFileBytes); err == nil {
			detected = true
			var m map[string]struct {
				Type   string `json:"type"`
				Access string `json:"access"`
				Key    string `json:"key"`
			}
			if json.Unmarshal(data, &m) == nil {
				for _, k := range []string{"opencode-go", "opencode", "default"} {
					e, ok := m[k]
					if !ok {
						continue
					}
					if strings.EqualFold(e.Type, "api") && strings.TrimSpace(e.Key) != "" {
						token = strings.TrimSpace(e.Key)
						break
					}
					if strings.TrimSpace(e.Access) != "" {
						token = strings.TrimSpace(e.Access)
						break
					}
					if strings.TrimSpace(e.Key) != "" {
						token = strings.TrimSpace(e.Key)
						break
					}
				}
			}
		}
	}
	if token == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		if expired {
			res.Status = StatusExpired
		} else {
			res.Status = StatusSignedOut
		}
		return res
	}

	res.Installed = true
	endpoint := env.baseURL(ProviderOpenCode, "https://opencode.ai") + "/inference/go/v1/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	type ocWin struct {
		Percent     *float64 `json:"percent"`
		UsedPercent *float64 `json:"usedPercent"`
		ResetsAt    any      `json:"resetsAt"`
	}
	var payload struct {
		Plan  string `json:"plan"`
		Usage *struct {
			Rolling *ocWin `json:"rolling"`
			Weekly  *ocWin `json:"weekly"`
			Monthly *ocWin `json:"monthly"`
		} `json:"usage"`
		Rolling *ocWin `json:"rolling"`
		Weekly  *ocWin `json:"weekly"`
		Monthly *ocWin `json:"monthly"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode opencode usage: %w", err).Error()
		return res
	}
	res.Plan = formatPlanName(payload.Plan)
	addOC := func(id string, kind WindowKind, dur time.Duration, w *ocWin) {
		if w == nil {
			return
		}
		pct := w.Percent
		if pct == nil {
			pct = w.UsedPercent
		}
		if pct == nil {
			return
		}
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       id,
			Kind:     kind,
			Used:     clampPct(*pct),
			ResetsAt: parseAnyTime(w.ResetsAt),
			Duration: dur,
		})
	}
	if payload.Usage != nil {
		addOC("rolling", WindowSession, 5*time.Hour, payload.Usage.Rolling)
		addOC("weekly", WindowWeekly, 7*24*time.Hour, payload.Usage.Weekly)
		addOC("monthly", WindowMonthly, 30*24*time.Hour, payload.Usage.Monthly)
	}
	if len(res.Windows) == 0 {
		addOC("rolling", WindowSession, 5*time.Hour, payload.Rolling)
		addOC("weekly", WindowWeekly, 7*24*time.Hour, payload.Weekly)
		addOC("monthly", WindowMonthly, 30*24*time.Hour, payload.Monthly)
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

// --- 13. Kiro CLI ---

func fetchKiro(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderKiro)
	dbPath := kiroDBPath(env.Home, env.Env)
	detected := res.Installed || pathExists(dbPath)

	raw, found, err := querySQLiteString(ctx, dbPath, "SELECT value FROM auth_kv WHERE key = 'kirocli:odic:token' LIMIT 1")
	if errors.Is(err, errWALWithoutSHM) {
		res.Installed = true
		res.Status = StatusUnavailable
		return res
	}
	if !found {
		var err2 error
		raw, found, err2 = querySQLiteString(ctx, dbPath, "SELECT value FROM auth_kv WHERE key LIKE '%token%' LIMIT 1")
		if err == nil {
			err = err2
		}
	}
	if err != nil && !os.IsNotExist(err) {
		detected = true
	}
	if !found || strings.TrimSpace(raw) == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	var tok struct {
		AccessToken string `json:"access_token"`
		Region      string `json:"region"`
		ExpiresAt   any    `json:"expires_at"`
	}
	if err := json.Unmarshal([]byte(raw), &tok); err != nil || strings.TrimSpace(tok.AccessToken) == "" {
		res.Status = StatusSignedOut
		return res
	}
	now := env.now()
	if exp := parseAnyTime(tok.ExpiresAt); !exp.IsZero() && exp.Before(now) {
		res.Status = StatusExpired
		return res
	}

	var profileARN string
	if stateRaw, ok, _ := querySQLiteString(ctx, dbPath, "SELECT value FROM state WHERE key = 'api.codewhisperer.profile' LIMIT 1"); ok {
		var prof struct {
			ARN string `json:"arn"`
		}
		if json.Unmarshal([]byte(stateRaw), &prof) == nil {
			profileARN = strings.TrimSpace(prof.ARN)
		}
	}

	region := strings.TrimSpace(tok.Region)
	if arnRegion := kiroARNRegion(profileARN); arnRegion != "" {
		region = arnRegion
	}
	if region == "" {
		region = "us-east-1"
	}
	defaultHost := fmt.Sprintf("https://codewhisperer.%s.amazonaws.com", region)
	if region == "eu-central-1" {
		defaultHost = "https://q.eu-central-1.amazonaws.com"
	}

	reqBody := []byte(`{"resourceType":"AGENTIC_REQUEST"}`)
	if profileARN != "" {
		if b, err := json.Marshal(map[string]string{"profileArn": profileARN}); err == nil {
			reqBody = b
		}
	}

	endpoint := env.baseURL(ProviderKiro, defaultHost) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tok.AccessToken))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererService.GetUsageLimits")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	var payload struct {
		SubscriptionInfo *struct {
			SubscriptionTitle string `json:"subscriptionTitle"`
		} `json:"subscriptionInfo"`
		UsageBreakdownList []struct {
			ResourceType                 string   `json:"resourceType"`
			CurrentUsageWithPrecision    *float64 `json:"currentUsageWithPrecision"`
			UsageLimitWithPrecision      *float64 `json:"usageLimitWithPrecision"`
			CurrentOveragesWithPrecision *float64 `json:"currentOveragesWithPrecision"`
			OverageCapWithPrecision      *float64 `json:"overageCapWithPrecision"`
			NextDateReset                any      `json:"nextDateReset"`
			DisplayName                  string   `json:"displayName"`
		} `json:"usageBreakdownList"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode kiro usage: %w", err).Error()
		return res
	}
	if payload.SubscriptionInfo != nil {
		res.Plan = formatPlanName(payload.SubscriptionInfo.SubscriptionTitle)
	}

	hasCreditTyped := false
	for _, u := range payload.UsageBreakdownList {
		if strings.EqualFold(u.ResourceType, "CREDIT") {
			hasCreditTyped = true
			break
		}
	}
	for i, u := range payload.UsageBreakdownList {
		if hasCreditTyped && !strings.EqualFold(u.ResourceType, "CREDIT") {
			continue
		}
		resetsAt := parseAnyTime(u.NextDateReset)
		if u.CurrentUsageWithPrecision != nil && u.UsageLimitWithPrecision != nil && *u.UsageLimitWithPrecision > 0 {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       fmt.Sprintf("usage_%d", i),
				Kind:     WindowMonthly,
				Model:    strings.TrimSpace(u.DisplayName),
				Used:     clampPct((*u.CurrentUsageWithPrecision / *u.UsageLimitWithPrecision) * 100.0),
				ResetsAt: resetsAt,
				Duration: 30 * 24 * time.Hour,
			})
		}
		if u.CurrentOveragesWithPrecision != nil && u.OverageCapWithPrecision != nil && *u.OverageCapWithPrecision > 0 {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:       fmt.Sprintf("overage_%d", i),
				Kind:     WindowCredits,
				Model:    "Overages",
				Used:     clampPct((*u.CurrentOveragesWithPrecision / *u.OverageCapWithPrecision) * 100.0),
				ResetsAt: resetsAt,
				Duration: 30 * 24 * time.Hour,
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

func kiroARNRegion(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 && parts[0] == "arn" {
		return strings.TrimSpace(parts[3])
	}
	return ""
}

// --- 14. Kilo ---

func fetchKilo(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderKilo)
	authPath := filepath.Join(env.Home, ".local", "share", "kilo", "auth.json")
	detected := res.Installed || pathExists(filepath.Dir(authPath))

	var auth struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		Kilo        *struct {
			Access string `json:"access"`
			Token  string `json:"token"`
		} `json:"kilo"`
	}
	if data, err := readCappedFile(authPath, maxConfigFileBytes); err == nil {
		detected = true
		_ = json.Unmarshal(data, &auth)
	}
	var tok string
	if auth.Kilo != nil {
		tok = strings.TrimSpace(auth.Kilo.Access)
		if tok == "" {
			tok = strings.TrimSpace(auth.Kilo.Token)
		}
	}
	if tok == "" {
		tok = strings.TrimSpace(auth.Token)
	}
	if tok == "" {
		tok = strings.TrimSpace(auth.AccessToken)
	}
	if tok == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	endpoint := env.baseURL(ProviderKilo, "https://app.kilo.ai") + `/api/trpc/user.getCreditBlocks,kiloPass.getState?batch=1&input=%7B%220%22%3A%7B%22json%22%3Anull%7D%2C%221%22%3A%7B%22json%22%3Anull%7D%7D`
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	var batch []struct {
		Result *struct {
			Data json.RawMessage `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &batch); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode kilo response: %w", err).Error()
		return res
	}

	unwrapTRPC := func(raw json.RawMessage) json.RawMessage {
		var env struct {
			JSON json.RawMessage `json:"json"`
		}
		if json.Unmarshal(raw, &env) == nil && len(env.JSON) > 0 && string(env.JSON) != "null" {
			return env.JSON
		}
		return raw
	}

	if len(batch) > 0 && batch[0].Result != nil {
		data := unwrapTRPC(batch[0].Result.Data)
		var blocks struct {
			TotalBalanceMicroUSD *float64 `json:"totalBalance_mUsd"`
			CreditBlocks         []struct {
				BalanceMicroUSD *float64 `json:"balance_mUsd"`
			} `json:"creditBlocks"`
			TotalBalance *float64 `json:"totalBalance"`
			TotalLimit   *float64 `json:"totalLimit"`
		}
		if json.Unmarshal(data, &blocks) == nil {
			switch {
			case blocks.TotalBalanceMicroUSD != nil:
				rem := *blocks.TotalBalanceMicroUSD / 1_000_000.0
				res.Credits = &CreditsInfo{RemainingUSD: new(rem)}
			case len(blocks.CreditBlocks) > 0:
				var sum float64
				var anyVal bool
				for _, b := range blocks.CreditBlocks {
					if b.BalanceMicroUSD != nil {
						sum += *b.BalanceMicroUSD / 1_000_000.0
						anyVal = true
					}
				}
				if anyVal {
					res.Credits = &CreditsInfo{RemainingUSD: new(sum)}
				}
			case blocks.TotalBalance != nil:
				if blocks.TotalLimit != nil && *blocks.TotalLimit > 0 {
					used := clampPct((1.0 - (*blocks.TotalBalance / *blocks.TotalLimit)) * 100.0)
					res.Windows = append(res.Windows, QuotaWindow{
						ID:           "credits",
						Kind:         WindowCredits,
						Used:         used,
						DollarsUsed:  new(*blocks.TotalLimit - *blocks.TotalBalance),
						DollarsLimit: new(*blocks.TotalLimit),
					})
				} else {
					res.Credits = &CreditsInfo{RemainingUSD: new(*blocks.TotalBalance)}
				}
			}
		}
	}

	if len(batch) > 1 && batch[1].Result != nil {
		data := unwrapTRPC(batch[1].Result.Data)
		var state struct {
			Subscription *struct {
				PlanName                     string   `json:"planName"`
				Tier                         string   `json:"tier"`
				CurrentPeriodUsageUSD        *float64 `json:"currentPeriodUsageUsd"`
				CurrentPeriodBaseCreditsUSD  *float64 `json:"currentPeriodBaseCreditsUsd"`
				CurrentPeriodBonusCreditsUSD *float64 `json:"currentPeriodBonusCreditsUsd"`
				NextBillingAt                any      `json:"nextBillingAt"`
				NextRenewalAt                any      `json:"nextRenewalAt"`
			} `json:"subscription"`
		}
		if json.Unmarshal(data, &state) == nil && state.Subscription != nil {
			sub := state.Subscription
			if sub.PlanName != "" {
				res.Plan = formatPlanName(sub.PlanName)
			} else if sub.Tier != "" {
				res.Plan = formatPlanName(sub.Tier)
			}
			var limit float64
			if sub.CurrentPeriodBaseCreditsUSD != nil {
				limit += *sub.CurrentPeriodBaseCreditsUSD
			}
			if sub.CurrentPeriodBonusCreditsUSD != nil {
				limit += *sub.CurrentPeriodBonusCreditsUSD
			}
			if sub.CurrentPeriodUsageUSD != nil && limit > 0 {
				resetsAt := parseAnyTime(sub.NextBillingAt)
				if resetsAt.IsZero() {
					resetsAt = parseAnyTime(sub.NextRenewalAt)
				}
				res.Windows = append(res.Windows, QuotaWindow{
					ID:           "kilo_pass",
					Kind:         WindowMonthly,
					Used:         clampPct((*sub.CurrentPeriodUsageUSD / limit) * 100.0),
					ResetsAt:     resetsAt,
					Duration:     30 * 24 * time.Hour,
					DollarsUsed:  new(*sub.CurrentPeriodUsageUSD),
					DollarsLimit: new(limit),
				})
			}
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

// --- 15-17. CLI-based providers (Amp, Augment, CodeRabbit) ---

var (
	reDollarFraction = regexp.MustCompile(`\$([0-9]+(?:\.[0-9]+)?)\s*/\s*\$([0-9]+(?:\.[0-9]+)?)`)
	rePercentUsed    = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*%\s*(?:used|consumed)`)
	rePercentLeft    = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*%\s*(?:left|remaining)`)
	reReviewsCount   = regexp.MustCompile(`(?i)([0-9]+)\s+reviews?`)
)

func fetchAmp(ctx context.Context, env FetchEnv) ProviderQuota {
	return fetchCLIQuota(ctx, env, ProviderAmp, []string{"amp", "usage"}, WindowMonthly, 30*24*time.Hour)
}

func fetchAugment(ctx context.Context, env FetchEnv) ProviderQuota {
	return fetchCLIQuota(ctx, env, ProviderAugment, []string{"auggie", "account", "status"}, WindowMonthly, 30*24*time.Hour)
}

func fetchCodeRabbit(ctx context.Context, env FetchEnv) ProviderQuota {
	return fetchCLIQuota(ctx, env, ProviderCodeRabbit, []string{"coderabbit", "usage"}, WindowMonthly, 30*24*time.Hour)
}

func fetchCLIQuota(ctx context.Context, env FetchEnv, id ProviderID, argv []string, kind WindowKind, dur time.Duration) ProviderQuota {
	res := env.newResult(id)
	if len(argv) == 0 || (!res.Installed && !env.hasExecutable(argv[0])) {
		res.Status = StatusNotFound
		return res
	}
	if env.Runner == nil {
		res.Status = StatusUnavailable
		return res
	}
	cmdRes, err := env.Runner.Run(ctx, run.Cmd{
		Args:      argv,
		Env:       env.Env,
		Timeout:   5 * time.Second,
		MaxOutput: 64 << 10,
	})
	if err != nil {
		if !res.Installed {
			res.Status = StatusNotFound
		} else {
			res.Status = StatusUnavailable
			res.Err = err.Error()
		}
		return res
	}
	res.Installed = true
	out := strings.TrimSpace(string(cmdRes.Stdout) + "\n" + string(cmdRes.Stderr))
	lower := strings.ToLower(out)
	if cmdRes.ExitCode != 0 {
		if strings.Contains(lower, "login") || strings.Contains(lower, "not logged") || strings.Contains(lower, "unauthenticated") || strings.Contains(lower, "sign in") {
			res.Status = StatusSignedOut
			return res
		}
		res.Status = StatusNoData
		return res
	}

	if m := reDollarFraction.FindStringSubmatch(out); len(m) == 3 {
		u, _ := strconv.ParseFloat(m[1], 64)
		l, _ := strconv.ParseFloat(m[2], 64)
		if l > 0 {
			res.Windows = append(res.Windows, QuotaWindow{
				ID:           string(kind),
				Kind:         kind,
				Used:         clampPct((u / l) * 100.0),
				Duration:     dur,
				DollarsUsed:  new(u),
				DollarsLimit: new(l),
			})
		}
	} else if m := rePercentUsed.FindStringSubmatch(lower); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			res.Windows = append(res.Windows, QuotaWindow{ID: string(kind), Kind: kind, Used: clampPct(v), Duration: dur})
		}
	} else if m := rePercentLeft.FindStringSubmatch(lower); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			res.Windows = append(res.Windows, QuotaWindow{ID: string(kind), Kind: kind, Used: clampPct(100.0 - v), Duration: dur})
		}
	}
	if id == ProviderCodeRabbit {
		if m := reReviewsCount.FindStringSubmatch(out); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				res.ReviewCount = &n
			}
		}
	}

	if len(res.Windows) == 0 && res.ReviewCount == nil {
		res.Status = StatusNoData
		return res
	}
	res.Windows = NormalizeWindows(res.Windows)
	res.Status = StatusOK
	res.UpdatedAt = env.now()
	return res
}

// --- 18. HuggingFace ---

func fetchHuggingFace(ctx context.Context, env FetchEnv) ProviderQuota {
	res := env.newResult(ProviderHuggingFace)
	tokPath := filepath.Join(env.Home, ".cache", "huggingface", "token")
	detected := res.Installed || pathExists(filepath.Dir(tokPath))

	var tok string
	if data, err := readCappedFile(tokPath, 16<<10); err == nil {
		detected = true
		tok = strings.TrimSpace(string(data))
	}
	if tok == "" {
		if !detected {
			res.Status = StatusNotFound
			return res
		}
		res.Installed = true
		res.Status = StatusSignedOut
		return res
	}

	res.Installed = true
	now := env.now()
	endpoint := env.baseURL(ProviderHuggingFace, "https://huggingface.co") + "/api/spaces/zero-gpu/quota"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Status = StatusUnavailable
		res.Err = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	body, err := doJSON(ctx, env.HTTP, req)
	if err != nil {
		mapHTTPError(&res, err)
		return res
	}

	var payload struct {
		Plan      string   `json:"plan"`
		Base      *float64 `json:"base"`
		Current   *float64 `json:"current"`
		Used      *float64 `json:"used"`
		Limit     *float64 `json:"limit"`
		ResetsAt  any      `json:"resetsAt"`
		ResetsAtS any      `json:"resets_at"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("decode huggingface quota: %w", err).Error()
		return res
	}
	res.Plan = formatPlanName(payload.Plan)
	resetsAt := parseAnyTime(payload.ResetsAt)
	if resetsAt.IsZero() {
		resetsAt = parseAnyTime(payload.ResetsAtS)
	}
	switch {
	case payload.Base != nil && payload.Current != nil && *payload.Base > 0:
		used := clampPct(((*payload.Base - *payload.Current) / *payload.Base) * 100.0)
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       "daily",
			Kind:     WindowDaily,
			Used:     used,
			ResetsAt: resetsAt,
			Duration: 24 * time.Hour,
		})
	case payload.Used != nil && payload.Limit != nil && *payload.Limit > 0:
		res.Windows = append(res.Windows, QuotaWindow{
			ID:       "daily",
			Kind:     WindowDaily,
			Used:     clampPct((*payload.Used / *payload.Limit) * 100.0),
			ResetsAt: resetsAt,
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
