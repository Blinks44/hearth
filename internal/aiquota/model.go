// Package aiquota reads AI subscription quotas and rate-limit windows across
// locally installed coding agents and IDEs. Every reader is strictly read-only
// and never logs, persists, or returns access tokens, cookies, API keys, or
// account emails.
package aiquota

import (
	"cmp"
	"slices"
	"time"
)

// ProviderID identifies an AI quota provider.
type ProviderID string

const (
	ProviderClaude      ProviderID = "claude"
	ProviderCodex       ProviderID = "codex"
	ProviderAntigravity ProviderID = "antigravity"
	ProviderGemini      ProviderID = "gemini"
	ProviderCursor      ProviderID = "cursor"
	ProviderCopilot     ProviderID = "copilot"
	ProviderJetBrains   ProviderID = "jetbrains"
	ProviderGrok        ProviderID = "grok"
	ProviderWindsurf    ProviderID = "windsurf"
	ProviderCline       ProviderID = "cline"
	ProviderKimi        ProviderID = "kimi"
	ProviderOpenCode    ProviderID = "opencode"
	ProviderKiro        ProviderID = "kiro"
	ProviderKilo        ProviderID = "kilo"
	ProviderAmp         ProviderID = "amp"
	ProviderAugment     ProviderID = "augment"
	ProviderCodeRabbit  ProviderID = "coderabbit"
	ProviderHuggingFace ProviderID = "huggingface"
)

// DefaultOrder is the canonical display order for providers before user
// customization. Priority 1 Hearth agents come first, followed by Priority 2.
var DefaultOrder = []ProviderID{
	ProviderClaude,
	ProviderCodex,
	ProviderAntigravity,
	ProviderGemini,
	ProviderCursor,
	ProviderCopilot,
	ProviderJetBrains,
	ProviderGrok,
	ProviderWindsurf,
	ProviderCline,
	ProviderKimi,
	ProviderOpenCode,
	ProviderKiro,
	ProviderKilo,
	ProviderAmp,
	ProviderAugment,
	ProviderCodeRabbit,
	ProviderHuggingFace,
}

// ProviderInfo describes static presentation metadata for a provider.
type ProviderInfo struct {
	ID        ProviderID
	Name      string
	ShortName string
}

var providerRegistry = []ProviderInfo{
	{ID: ProviderClaude, Name: "Claude Code", ShortName: "Claude Code"},
	{ID: ProviderCodex, Name: "Codex", ShortName: "Codex"},
	{ID: ProviderAntigravity, Name: "Antigravity", ShortName: "Antigravity"},
	{ID: ProviderGemini, Name: "Gemini CLI", ShortName: "Gemini"},
	{ID: ProviderCursor, Name: "Cursor", ShortName: "Cursor"},
	{ID: ProviderCopilot, Name: "GitHub Copilot", ShortName: "Copilot"},
	{ID: ProviderJetBrains, Name: "JetBrains AI", ShortName: "JetBrains"},
	{ID: ProviderGrok, Name: "Grok CLI", ShortName: "Grok"},
	{ID: ProviderWindsurf, Name: "Windsurf", ShortName: "Windsurf"},
	{ID: ProviderCline, Name: "Cline", ShortName: "Cline"},
	{ID: ProviderKimi, Name: "Kimi Code", ShortName: "Kimi"},
	{ID: ProviderOpenCode, Name: "OpenCode Go", ShortName: "OpenCode"},
	{ID: ProviderKiro, Name: "Kiro", ShortName: "Kiro"},
	{ID: ProviderKilo, Name: "Kilo", ShortName: "Kilo"},
	{ID: ProviderAmp, Name: "Amp", ShortName: "Amp"},
	{ID: ProviderAugment, Name: "Augment", ShortName: "Augment"},
	{ID: ProviderCodeRabbit, Name: "CodeRabbit", ShortName: "CodeRabbit"},
	{ID: ProviderHuggingFace, Name: "HuggingFace", ShortName: "HuggingFace"},
}

// LookupProvider returns static metadata for id.
func LookupProvider(id ProviderID) ProviderInfo {
	for _, p := range providerRegistry {
		if p.ID == id {
			return p
		}
	}
	return ProviderInfo{ID: id, Name: string(id), ShortName: string(id)}
}

// Status describes the read outcome for a provider.
type Status string

const (
	StatusOK          Status = "ok"
	StatusNotFound    Status = "not_found"
	StatusSignedOut   Status = "signed_out"
	StatusAPIKey      Status = "api_key"
	StatusExpired     Status = "expired"
	StatusNoData      Status = "no_data"
	StatusUnavailable Status = "unavailable"
)

// WindowKind categorizes a quota window's cadence.
type WindowKind string

const (
	WindowSession WindowKind = "session"
	WindowDaily   WindowKind = "daily"
	WindowWeekly  WindowKind = "weekly"
	WindowMonthly WindowKind = "monthly"
	WindowCredits WindowKind = "credits"
)

// PaceKind summarizes consumption rate compared to elapsed window time.
type PaceKind string

const (
	PaceUnknown   PaceKind = ""
	PaceSpare     PaceKind = "spare"
	PaceNormal    PaceKind = "normal"
	PaceFast      PaceKind = "fast"
	PaceLow       PaceKind = "low"
	PaceExhausted PaceKind = "exhausted"
)

// Thresholds matching Hearth's quota warning semantics.
const (
	LowThresholdPct     = 10.0
	WarningThresholdPct = 25.0
	StaleAfter          = 15 * time.Minute
)

// QuotaWindow represents one rate-limit or budget window (e.g. 5h session,
// 7d weekly, monthly dollar budget).
type QuotaWindow struct {
	ID           string        `json:"id,omitempty"`
	Kind         WindowKind    `json:"kind"`
	Group        string        `json:"group,omitempty"`
	Model        string        `json:"model,omitempty"`
	Label        string        `json:"label,omitempty"`
	Used         float64       `json:"used"` // percentage [0..100]
	ResetsAt     time.Time     `json:"resetsAt,omitzero"`
	Duration     time.Duration `json:"duration,omitempty"`
	DollarsUsed  *float64      `json:"dollarsUsed,omitempty"`
	DollarsLimit *float64      `json:"dollarsLimit,omitempty"`
}

// Key returns a stable identifier for this window within a provider.
func (w QuotaWindow) Key() string {
	if w.ID != "" {
		return w.ID
	}
	if w.Group != "" && w.Kind != "" {
		return w.Group + ":" + string(w.Kind)
	}
	if w.Model != "" && w.Kind != "" {
		return w.Model + ":" + string(w.Kind)
	}
	return string(w.Kind)
}

func windowKindRank(k WindowKind) int {
	switch k {
	case WindowSession:
		return 0
	case WindowDaily:
		return 1
	case WindowWeekly:
		return 2
	case WindowMonthly:
		return 3
	case WindowCredits:
		return 4
	default:
		return 5
	}
}

func windowGroupRank(g string) int {
	switch g {
	case "":
		return 0
	case "Gemini":
		return 1
	case "Claude":
		return 2
	default:
		return 3
	}
}

// NormalizeWindows returns a copy of windows sorted in a consistent order:
// ungrouped windows first, then named groups (Gemini, Claude, others), and
// within each group shorter cadences first (session 5h -> daily -> weekly ->
// monthly -> credits), with plan-wide limits before model-specific limits.
func NormalizeWindows(windows []QuotaWindow) []QuotaWindow {
	if len(windows) <= 1 {
		return slices.Clone(windows)
	}
	out := slices.Clone(windows)
	slices.SortStableFunc(out, func(a, b QuotaWindow) int {
		if c := cmp.Compare(windowGroupRank(a.Group), windowGroupRank(b.Group)); c != 0 {
			return c
		}
		if a.Group != b.Group {
			return cmp.Compare(a.Group, b.Group)
		}
		if c := cmp.Compare(windowKindRank(a.Kind), windowKindRank(b.Kind)); c != 0 {
			return c
		}
		aHasModel := a.Model != ""
		bHasModel := b.Model != ""
		if aHasModel != bHasModel {
			if !aHasModel {
				return -1
			}
			return 1
		}
		if a.Duration != b.Duration && a.Duration > 0 && b.Duration > 0 {
			return cmp.Compare(a.Duration, b.Duration)
		}
		return 0
	})
	return out
}

// Left returns the remaining percentage clamped to [0, 100].
func (w QuotaWindow) Left() float64 {
	left := 100.0 - w.Used
	if left < 0 {
		return 0
	}
	if left > 100 {
		return 100
	}
	return left
}

// EffectiveLeft returns 100 if the window has already reset at now.
func (w QuotaWindow) EffectiveLeft(now time.Time) float64 {
	if w.HasReset(now) {
		return 100
	}
	return w.Left()
}

// HasReset reports whether ResetsAt is known and in the past at now.
func (w QuotaWindow) HasReset(now time.Time) bool {
	return !w.ResetsAt.IsZero() && !w.ResetsAt.After(now)
}

// IsExhausted reports whether the window is at 0% remaining (and hasn't reset).
func (w QuotaWindow) IsExhausted(now time.Time) bool {
	if w.HasReset(now) {
		return false
	}
	return w.Left() <= 0.5
}

// IsRunningLow reports whether remaining quota is strictly below 10% (and hasn't reset).
func (w QuotaWindow) IsRunningLow(now time.Time) bool {
	if w.HasReset(now) {
		return false
	}
	return w.Left() < LowThresholdPct
}

// IsWarning reports whether remaining quota is below 25% (and hasn't reset).
func (w QuotaWindow) IsWarning(now time.Time) bool {
	if w.HasReset(now) {
		return false
	}
	return w.Left() < WarningThresholdPct
}

// TimeLeft returns the fraction of the window duration still remaining, as a
// percentage [0..100], or (0, false) when ResetsAt or Duration is unavailable.
// On a remaining-quota bar (100% left = full), TimeLeft is the expected
// remaining mark at now: if Left() < TimeLeft, usage is ahead of linear pace.
func (w QuotaWindow) TimeLeft(now time.Time) (float64, bool) {
	if w.ResetsAt.IsZero() || w.Duration <= 0 {
		return 0, false
	}
	rem := w.ResetsAt.Sub(now)
	if rem <= 0 {
		return 0, true
	}
	pct := float64(rem) / float64(w.Duration) * 100.0
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// RunsOutAt projects when the window will hit 0% remaining at the current
// average consumption rate since the window started. Returns zero time if the
// projection cannot be computed or would occur after ResetsAt (meaning the
// user is on pace to last until reset).
func (w QuotaWindow) RunsOutAt(now time.Time) time.Time {
	if w.ResetsAt.IsZero() || w.Duration <= 0 || w.HasReset(now) {
		return time.Time{}
	}
	if w.Used <= 0.5 || w.Left() <= 0.5 {
		return time.Time{}
	}
	windowStart := w.ResetsAt.Add(-w.Duration)
	elapsed := now.Sub(windowStart)
	// Require at least 2% of window elapsed (min 5m) to avoid wild spikes at window start.
	minElapsed := max(w.Duration/50, 5*time.Minute)
	if elapsed < minElapsed {
		return time.Time{}
	}
	ratePerSec := w.Used / elapsed.Seconds()
	if ratePerSec <= 0 {
		return time.Time{}
	}
	secsToEmpty := w.Left() / ratePerSec
	projected := now.Add(time.Duration(secsToEmpty * float64(time.Second)))
	if !projected.Before(w.ResetsAt) {
		return time.Time{}
	}
	return projected
}

// PaceStatus classifies the window's consumption rate relative to remaining time.
func (w QuotaWindow) PaceStatus(now time.Time) PaceKind {
	if w.HasReset(now) {
		return PaceSpare
	}
	if w.IsExhausted(now) {
		return PaceExhausted
	}
	if w.IsRunningLow(now) {
		return PaceLow
	}
	expectedLeft, ok := w.TimeLeft(now)
	if !ok {
		return PaceUnknown
	}
	diff := w.Left() - expectedLeft
	switch {
	case diff >= 12:
		return PaceSpare
	case diff <= -10:
		return PaceFast
	default:
		return PaceNormal
	}
}

// CreditsInfo holds optional extra / pay-as-you-go credit balance info.
type CreditsInfo struct {
	UsedUSD      *float64 `json:"usedUsd,omitempty"`
	LimitUSD     *float64 `json:"limitUsd,omitempty"`
	RemainingUSD *float64 `json:"remainingUsd,omitempty"`
	RemainingPct *float64 `json:"remainingPct,omitempty"`
	Unlimited    bool     `json:"unlimited,omitempty"`
	Label        string   `json:"label,omitempty"`
}

// ProviderQuota holds the quota reading for a single provider.
type ProviderQuota struct {
	Provider          ProviderID    `json:"provider"`
	Name              string        `json:"name"`
	Plan              string        `json:"plan,omitempty"`
	Status            Status        `json:"status"`
	Windows           []QuotaWindow `json:"windows,omitempty"`
	UpdatedAt         time.Time     `json:"updatedAt,omitzero"`
	SourceDateUnknown bool          `json:"sourceDateUnknown,omitempty"`
	RetryAfter        time.Time     `json:"retryAfter,omitzero"`
	Credits           *CreditsInfo  `json:"credits,omitempty"`
	ReviewCount       *int          `json:"reviewCount,omitempty"`
	Installed         bool          `json:"installed,omitempty"`
	Err               string        `json:"err,omitempty"`
}

// Connected reports whether the provider has active readable quota data.
func (p ProviderQuota) Connected() bool {
	return p.Status == StatusOK && (len(p.Windows) > 0 || p.Credits != nil || p.ReviewCount != nil)
}

// IsStale reports whether the last successful reading is older than StaleAfter.
func (p ProviderQuota) IsStale(now time.Time) bool {
	if p.SourceDateUnknown || p.UpdatedAt.IsZero() {
		return false
	}
	return now.Sub(p.UpdatedAt) > StaleAfter
}

// MinLeft returns the lowest remaining percentage across all active windows at
// now, or (100, false) if there are no windows.
func (p ProviderQuota) MinLeft(now time.Time) (float64, bool) {
	if len(p.Windows) == 0 {
		return 100, false
	}
	minVal := 100.0
	for _, w := range p.Windows {
		if l := w.EffectiveLeft(now); l < minVal {
			minVal = l
		}
	}
	return minVal, true
}

// NearestReset returns the earliest future ResetsAt among p's windows.
func (p ProviderQuota) NearestReset(now time.Time) (QuotaWindow, bool) {
	var best QuotaWindow
	found := false
	for _, w := range p.Windows {
		if w.ResetsAt.IsZero() || !w.ResetsAt.After(now) {
			continue
		}
		if !found || w.ResetsAt.Before(best.ResetsAt) {
			best = w
			found = true
		}
	}
	return best, found
}

// HasRunningLow reports whether any active window of p is below LowThresholdPct.
func (p ProviderQuota) HasRunningLow(now time.Time) bool {
	for _, w := range p.Windows {
		if w.IsRunningLow(now) {
			return true
		}
	}
	return false
}

// Snapshot is an immutable point-in-time view of all provider quotas.
type Snapshot struct {
	Providers []ProviderQuota `json:"providers"`
	CheckedAt time.Time       `json:"checkedAt"`
}

// ConnectedCount returns the number of providers with active quota data.
func (s Snapshot) ConnectedCount() int {
	n := 0
	for _, p := range s.Providers {
		if p.Connected() {
			n++
		}
	}
	return n
}

// FoundCount returns the number of detected providers (connected or inactive).
func (s Snapshot) FoundCount() int {
	n := 0
	for _, p := range s.Providers {
		if p.Connected() || (p.Installed && p.Status != StatusNotFound) || (p.Status != "" && p.Status != StatusNotFound) {
			n++
		}
	}
	return n
}

// LowCount returns the number of connected providers with at least one window
// strictly below LowThresholdPct at now.
func (s Snapshot) LowCount(now time.Time) int {
	n := 0
	for _, p := range s.Providers {
		if p.Connected() && p.HasRunningLow(now) {
			n++
		}
	}
	return n
}

// UpcomingReset describes one future window reset across providers.
type UpcomingReset struct {
	Provider ProviderID
	Name     string
	Window   QuotaWindow
}

// UpcomingResets returns future window resets sorted ascending by ResetsAt.
func (s Snapshot) UpcomingResets(now time.Time) []UpcomingReset {
	var out []UpcomingReset
	for _, p := range s.Providers {
		if !p.Connected() {
			continue
		}
		for _, w := range p.Windows {
			if w.ResetsAt.IsZero() || !w.ResetsAt.After(now) {
				continue
			}
			out = append(out, UpcomingReset{
				Provider: p.Provider,
				Name:     p.Name,
				Window:   w,
			})
		}
	}
	slices.SortFunc(out, func(a, b UpcomingReset) int {
		if c := a.Window.ResetsAt.Compare(b.Window.ResetsAt); c != 0 {
			return c
		}
		return cmp.Compare(string(a.Provider), string(b.Provider))
	})
	return out
}

// Ordered returns connected providers ordered by customOrder (then DefaultOrder).
func (s Snapshot) Ordered(customOrder []string) []ProviderQuota {
	rank := make(map[ProviderID]int, len(DefaultOrder)+len(customOrder))
	idx := 0
	for _, id := range customOrder {
		pid := ProviderID(id)
		if _, ok := rank[pid]; !ok {
			rank[pid] = idx
			idx++
		}
	}
	for _, pid := range DefaultOrder {
		if _, ok := rank[pid]; !ok {
			rank[pid] = idx
			idx++
		}
	}
	var out []ProviderQuota
	for _, p := range s.Providers {
		if p.Connected() {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b ProviderQuota) int {
		ra, oka := rank[a.Provider]
		if !oka {
			ra = 9999
		}
		rb, okb := rank[b.Provider]
		if !okb {
			rb = 9999
		}
		if c := cmp.Compare(ra, rb); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

// Inactive returns detected providers that do not currently have active quota
// windows (e.g. signed out, API key, expired, no data, unavailable).
func (s Snapshot) Inactive() []ProviderQuota {
	var out []ProviderQuota
	for _, p := range s.Providers {
		if p.Connected() {
			continue
		}
		if p.Status == "" || p.Status == StatusNotFound {
			continue
		}
		out = append(out, p)
	}
	return out
}
