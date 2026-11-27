package aiquota

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// ActivePollInterval is the refresh cadence while the user is actively
	// viewing the AI Quotas page or has the Tray Popover open. Two minutes
	// keeps data fresh without triggering strict provider rate limits (e.g. Claude).
	ActivePollInterval = 2 * time.Minute

	// BackgroundActiveInterval is the background polling cadence when low-quota
	// notifications are enabled and a connected provider's usage is changing.
	BackgroundActiveInterval = 5 * time.Minute

	// BackgroundIdleInterval is the backed-off background cadence after a
	// connected provider reports unchanged usage for two consecutive polls.
	BackgroundIdleInterval = 15 * time.Minute

	// BackgroundMissingInterval is the background cadence for providers that
	// are not installed, signed out, or configured with an API key.
	BackgroundMissingInterval = 30 * time.Minute

	// ExhaustedResetBuffer is added past a known ResetsAt when pausing polling
	// for an exhausted provider so the server has rolled over the window.
	ExhaustedResetBuffer = 15 * time.Second

	maxExhaustedSleep = 6 * time.Hour
	progressDebounce  = 120 * time.Millisecond
)

// PollMode controls how Monitor selects providers during a refresh pass.
type PollMode int

const (
	// PollActive polls every provider that is not currently rate-limited.
	PollActive PollMode = iota
	// PollBackground polls only providers whose adaptive schedule is due.
	PollBackground
)

type providerFetcher func(ctx context.Context, env FetchEnv) ProviderQuota

var allFetchers = []struct {
	id ProviderID
	fn providerFetcher
}{
	{ProviderClaude, fetchClaude},
	{ProviderCodex, fetchCodex},
	{ProviderAntigravity, fetchAntigravity},
	{ProviderGemini, fetchGemini},
	{ProviderCursor, fetchCursor},
	{ProviderCopilot, fetchCopilot},
	{ProviderJetBrains, fetchJetBrains},
	{ProviderGrok, fetchGrok},
	{ProviderWindsurf, fetchWindsurf},
	{ProviderCline, fetchCline},
	{ProviderKimi, fetchKimi},
	{ProviderOpenCode, fetchOpenCode},
	{ProviderKiro, fetchKiro},
	{ProviderKilo, fetchKilo},
	{ProviderAmp, fetchAmp},
	{ProviderAugment, fetchAugment},
	{ProviderCodeRabbit, fetchCodeRabbit},
	{ProviderHuggingFace, fetchHuggingFace},
}

// FetchAll runs all provider quota readers concurrently with a bounded worker
// pool and returns a Snapshot ordered by DefaultOrder. A single failing
// provider never aborts the poll.
func FetchAll(ctx context.Context, env FetchEnv) Snapshot {
	snap, _ := fetchWithCache(ctx, env, nil, nil, nil)
	return snap
}

func fetchWithCache(
	ctx context.Context,
	env FetchEnv,
	prev map[ProviderID]ProviderQuota,
	due map[ProviderID]bool,
	onProgress func(Snapshot),
) (Snapshot, map[ProviderID]bool) {
	if env.HTTP == nil {
		env.HTTP = newHTTPClient()
	}
	now := env.now()
	results := make([]ProviderQuota, len(allFetchers))
	fetched := make(map[ProviderID]bool, len(allFetchers))

	// Pre-populate results with previous cached entries so progressive updates
	// never flash empty cards for providers that are still in flight.
	for i, pf := range allFetchers {
		if prev != nil {
			if old, ok := prev[pf.id]; ok {
				old.Windows = NormalizeWindows(old.Windows)
				results[i] = old
			}
		}
	}

	var (
		resMu        sync.Mutex
		progWG       sync.WaitGroup
		progTimer    *time.Timer
		progStopped  bool
		progLastSent time.Time
	)
	dispatchProgress := func(snap Snapshot) {
		defer progWG.Done()
		onProgress(snap)
	}
	emitProgress := func() {
		resMu.Lock()
		if progStopped || onProgress == nil {
			resMu.Unlock()
			return
		}
		snap := Snapshot{
			Providers: slices.Clone(results),
			CheckedAt: now,
		}
		progLastSent = time.Now()
		progWG.Add(1)
		resMu.Unlock()
		dispatchProgress(snap)
	}
	scheduleProgress := func() {
		if onProgress == nil {
			return
		}
		resMu.Lock()
		defer resMu.Unlock()
		if progStopped {
			return
		}
		if progLastSent.IsZero() || time.Since(progLastSent) >= progressDebounce {
			if progTimer != nil {
				progTimer.Stop()
				progTimer = nil
			}
			snap := Snapshot{
				Providers: slices.Clone(results),
				CheckedAt: now,
			}
			progLastSent = time.Now()
			progWG.Add(1)
			go dispatchProgress(snap)
			return
		}
		if progTimer == nil {
			progTimer = time.AfterFunc(progressDebounce, emitProgress)
		}
	}

	const maxWorkers = 6
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	for i, pf := range allFetchers {
		// Always respect an active 429 Retry-After cooldown (even on manual
		// refresh) so clicking Refresh never extends a server rate-limit ban.
		if prev != nil {
			if old, ok := prev[pf.id]; ok && !old.RetryAfter.IsZero() && now.Before(old.RetryAfter) {
				old.Windows = NormalizeWindows(old.Windows)
				results[i] = old
				continue
			}
		}
		if due != nil && !due[pf.id] {
			if prev != nil {
				if old, ok := prev[pf.id]; ok {
					old.Windows = NormalizeWindows(old.Windows)
					results[i] = old
					continue
				}
			}
		}
		fetched[pf.id] = true
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, id ProviderID, fn providerFetcher) {
			defer wg.Done()
			defer func() { <-sem }()
			got := fn(ctx, env)
			got.Windows = NormalizeWindows(got.Windows)
			// Preserve last good windows as stale if a transient network/429 error occurred.
			if got.Status == StatusUnavailable && prev != nil {
				if old, ok := prev[id]; ok && old.Connected() {
					preserved := old
					preserved.Windows = NormalizeWindows(preserved.Windows)
					preserved.RetryAfter = got.RetryAfter
					preserved.Err = got.Err
					got = preserved
				}
			}
			resMu.Lock()
			results[idx] = got
			resMu.Unlock()
			if got.Connected() {
				scheduleProgress()
			}
		}(i, pf.id, pf.fn)
	}
	wg.Wait()

	resMu.Lock()
	progStopped = true
	if progTimer != nil {
		progTimer.Stop()
		progTimer = nil
	}
	resMu.Unlock()
	progWG.Wait()

	return Snapshot{
		Providers: results,
		CheckedAt: now,
	}, fetched
}

// AlertEvent describes a quota threshold crossing worthy of a system notification.
type AlertEvent struct {
	Provider ProviderID
	Name     string
	Window   QuotaWindow
	Reset    bool // true when a previously low/exhausted window has reset
}

// LowWindowTracker debounces low-quota (<10%) and reset notifications so each
// low episode notifies at most once until the window resets.
type LowWindowTracker struct {
	mu      sync.Mutex
	alerted map[string]time.Time // window key -> resetsAt (or zero)
}

// NewLowWindowTracker constructs a tracker with empty state.
func NewLowWindowTracker() *LowWindowTracker {
	return &LowWindowTracker{alerted: map[string]time.Time{}}
}

// Seed records currently low windows from a persisted cache without emitting
// notifications, preventing duplicate alerts across app restarts.
func (t *LowWindowTracker) Seed(snap Snapshot, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range snap.Providers {
		if !p.Connected() || p.IsStale(now) {
			continue
		}
		for _, w := range p.Windows {
			if w.IsRunningLow(now) {
				k := string(p.Provider) + ":" + w.Key()
				t.alerted[k] = w.ResetsAt
			}
		}
	}
}

// Evaluate compares snap against previously alerted low windows and returns
// newly triggered low-quota or reset events.
func (t *LowWindowTracker) Evaluate(snap Snapshot, now time.Time) []AlertEvent {
	t.mu.Lock()
	defer t.mu.Unlock()

	var events []AlertEvent
	seen := make(map[string]bool)

	for _, p := range snap.Providers {
		if !p.Connected() {
			continue
		}
		stale := p.IsStale(now)
		for _, w := range p.Windows {
			k := string(p.Provider) + ":" + w.Key()
			seen[k] = true
			if stale {
				continue
			}
			prevReset, wasAlerted := t.alerted[k]

			if w.IsRunningLow(now) {
				if !wasAlerted {
					t.alerted[k] = w.ResetsAt
					events = append(events, AlertEvent{
						Provider: p.Provider,
						Name:     p.Name,
						Window:   w,
					})
				} else if isNewQuotaPeriod(prevReset, w, now) {
					// A brand new cycle became low again.
					t.alerted[k] = w.ResetsAt
					events = append(events, AlertEvent{
						Provider: p.Provider,
						Name:     p.Name,
						Window:   w,
					})
				}
				continue
			}

			// Re-arm once the window recovers well above threshold (e.g. after reset).
			if wasAlerted && (w.HasReset(now) || w.EffectiveLeft(now) >= WarningThresholdPct) {
				delete(t.alerted, k)
				events = append(events, AlertEvent{
					Provider: p.Provider,
					Name:     p.Name,
					Window:   w,
					Reset:    true,
				})
			}
		}
	}

	for k := range t.alerted {
		if !seen[k] {
			delete(t.alerted, k)
		}
	}
	return events
}

// isNewQuotaPeriod distinguishes genuine cycle rollovers from minor server
// resets_at clock jitter (e.g. +/- 2 minutes on the same window).
func isNewQuotaPeriod(prevReset time.Time, w QuotaWindow, now time.Time) bool {
	if prevReset.IsZero() || w.ResetsAt.IsZero() {
		return false
	}
	if !prevReset.After(now) {
		return true
	}
	if w.Duration > 0 {
		windowStart := w.ResetsAt.Add(-w.Duration)
		return !windowStart.Before(prevReset)
	}
	return w.ResetsAt.Sub(prevReset) > 15*time.Minute
}

type providerSchedule struct {
	nextPollAt      time.Time
	unchangedCount  int
	lastFingerprint string
	lastInstalled   bool
}

// Monitor runs periodic background AI quota polling with caching, per-provider
// rate-limit backoff, adaptive background intervals, and progressive updates.
type Monitor struct {
	DataDir    string
	EnvFn      func() FetchEnv
	OnUpdate   func(Snapshot, []AlertEvent)
	OnProgress func(Snapshot)

	mu          sync.Mutex
	snap        Snapshot
	tracker     *LowWindowTracker
	inFlight    bool
	client      *http.Client
	geminiCache *geminiProjectCache
	schedules   map[ProviderID]*providerSchedule
}

// NewMonitor initializes a Monitor, loads any persisted cached snapshot, and
// pre-seeds the low-quota alert tracker from cache.
func NewMonitor(dataDir string, envFn func() FetchEnv, onUpdate func(Snapshot, []AlertEvent)) *Monitor {
	cached, _ := LoadCache(dataDir)
	for i := range cached.Providers {
		cached.Providers[i].Windows = NormalizeWindows(cached.Providers[i].Windows)
	}
	tracker := NewLowWindowTracker()
	if !cached.CheckedAt.IsZero() {
		tracker.Seed(cached, time.Now())
	}
	return &Monitor{
		DataDir:     dataDir,
		EnvFn:       envFn,
		OnUpdate:    onUpdate,
		snap:        cached,
		tracker:     tracker,
		client:      newHTTPClient(),
		geminiCache: newGeminiProjectCache(),
		schedules:   make(map[ProviderID]*providerSchedule, len(allFetchers)),
	}
}

// Snapshot returns the most recent quota snapshot.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap
}

// Refresh polls all non-rate-limited providers immediately, persists the cache,
// evaluates low-quota alerts, and calls OnUpdate.
func (m *Monitor) Refresh(ctx context.Context, force bool) Snapshot {
	_ = force // 429 RetryAfter is always respected; use RefreshMode for background scheduling.
	return m.RefreshMode(ctx, PollActive)
}

// RefreshMode polls providers according to mode (PollActive polls every
// non-rate-limited provider; PollBackground polls only providers whose adaptive
// schedule is due and skips work completely when nothing is due).
func (m *Monitor) RefreshMode(ctx context.Context, mode PollMode) Snapshot {
	var env FetchEnv
	if m.EnvFn != nil {
		env = m.EnvFn()
	}
	if env.HTTP == nil {
		env.HTTP = m.client
	}
	env.geminiCache = m.geminiCache
	now := env.now()

	m.mu.Lock()
	if m.inFlight {
		snap := m.snap
		m.mu.Unlock()
		return snap
	}
	prev := make(map[ProviderID]ProviderQuota, len(m.snap.Providers))
	for _, p := range m.snap.Providers {
		prev[p.Provider] = p
	}

	var due map[ProviderID]bool
	if mode == PollBackground {
		due = make(map[ProviderID]bool, len(allFetchers))
		anyDue := false
		for _, pf := range allFetchers {
			if old, ok := prev[pf.id]; ok && !old.RetryAfter.IsZero() && now.Before(old.RetryAfter) {
				continue
			}
			sched := m.schedules[pf.id]
			nowInstalled := env.Installed[pf.id]
			if sched == nil || sched.nextPollAt.IsZero() || !now.Before(sched.nextPollAt) || (nowInstalled && !sched.lastInstalled) {
				due[pf.id] = true
				anyDue = true
			}
		}
		if !anyDue {
			snap := m.snap
			m.mu.Unlock()
			return snap
		}
	}

	m.inFlight = true
	onProgress := m.OnProgress
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.inFlight = false
		m.mu.Unlock()
	}()

	snap, fetched := fetchWithCache(ctx, env, prev, due, onProgress)
	doneAt := env.now()

	m.mu.Lock()
	m.snap = snap
	for _, p := range snap.Providers {
		if fetched[p.Provider] {
			m.updateScheduleLocked(p, env.Installed[p.Provider], doneAt)
		}
	}
	m.mu.Unlock()

	_ = SaveCache(m.DataDir, snap)
	_ = AppendHistory(m.DataDir, snap, doneAt)
	events := m.tracker.Evaluate(snap, doneAt)
	if m.OnUpdate != nil {
		m.OnUpdate(snap, events)
	}
	return snap
}

func (m *Monitor) updateScheduleLocked(p ProviderQuota, installedHint bool, now time.Time) {
	sched := m.schedules[p.Provider]
	if sched == nil {
		sched = &providerSchedule{}
		m.schedules[p.Provider] = sched
	}
	sched.lastInstalled = installedHint || p.Installed

	if !p.RetryAfter.IsZero() && p.RetryAfter.After(now) {
		sched.nextPollAt = p.RetryAfter
		return
	}

	fp := providerFingerprint(p)
	if fp == sched.lastFingerprint && sched.lastFingerprint != "" {
		sched.unchangedCount++
	} else {
		sched.lastFingerprint = fp
		sched.unchangedCount = 0
	}

	switch p.Status {
	case StatusOK:
		if p.Connected() {
			if resetAt, ok := exhaustedWakeupTime(p, now); ok {
				sched.nextPollAt = resetAt
				return
			}
			if sched.unchangedCount >= 2 {
				sched.nextPollAt = now.Add(BackgroundIdleInterval)
			} else {
				sched.nextPollAt = now.Add(BackgroundActiveInterval)
			}
			return
		}
		sched.nextPollAt = now.Add(BackgroundIdleInterval)
	case StatusNotFound, StatusSignedOut, StatusAPIKey, StatusExpired:
		sched.nextPollAt = now.Add(BackgroundMissingInterval)
	default:
		sched.nextPollAt = now.Add(BackgroundActiveInterval)
	}
}

func exhaustedWakeupTime(p ProviderQuota, now time.Time) (time.Time, bool) {
	if len(p.Windows) == 0 {
		return time.Time{}, false
	}
	var earliest time.Time
	for _, w := range p.Windows {
		if !w.IsExhausted(now) {
			return time.Time{}, false
		}
		if !w.ResetsAt.IsZero() && w.ResetsAt.After(now) {
			if earliest.IsZero() || w.ResetsAt.Before(earliest) {
				earliest = w.ResetsAt
			}
		}
	}
	if earliest.IsZero() {
		return time.Time{}, false
	}
	wake := earliest.Add(ExhaustedResetBuffer)
	if minWake := now.Add(ActivePollInterval); wake.Before(minWake) {
		wake = minWake
	}
	if maxWake := now.Add(maxExhaustedSleep); wake.After(maxWake) {
		wake = maxWake
	}
	return wake, true
}

func providerFingerprint(p ProviderQuota) string {
	var b strings.Builder
	b.WriteString(string(p.Status))
	for _, w := range p.Windows {
		_, _ = fmt.Fprintf(&b, "|%s:%d:%d", w.Key(), int(math.Round(w.Used*10)), w.ResetsAt.Truncate(time.Minute).Unix())
	}
	if p.Credits != nil && p.Credits.RemainingUSD != nil {
		_, _ = fmt.Fprintf(&b, "|c:%.2f", *p.Credits.RemainingUSD)
	}
	if p.ReviewCount != nil {
		_, _ = fmt.Fprintf(&b, "|r:%d", *p.ReviewCount)
	}
	return b.String()
}
