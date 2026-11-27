package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/aiquota"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/locales"
)

func sampleQuotasSnapshot(now time.Time) *aiquota.Snapshot {
	used1240 := 12.40
	limit20 := 20.0
	extraUsed := 4.50
	extraLimit := 50.0
	return &aiquota.Snapshot{
		CheckedAt: now.Add(-40 * time.Second),
		Providers: []aiquota.ProviderQuota{
			{
				Provider:  aiquota.ProviderClaude,
				Name:      "Claude Code",
				Plan:      "Max",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now.Add(-30 * time.Second),
				Windows: []aiquota.QuotaWindow{
					{Kind: aiquota.WindowSession, Used: 38, Duration: 5 * time.Hour, ResetsAt: now.Add(2*time.Hour + 15*time.Minute)},
					{Kind: aiquota.WindowWeekly, Used: 26, Duration: 7 * 24 * time.Hour, ResetsAt: now.Add(4 * 24 * time.Hour)},
					{Kind: aiquota.WindowWeekly, Model: "Sonnet", Used: 12, Duration: 7 * 24 * time.Hour, ResetsAt: now.Add(4 * 24 * time.Hour)},
				},
			},
			{
				Provider:  aiquota.ProviderCodex,
				Name:      "Codex",
				Plan:      "Plus",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now.Add(-30 * time.Second),
				Windows: []aiquota.QuotaWindow{
					{Kind: aiquota.WindowSession, Used: 18, Duration: 5 * time.Hour, ResetsAt: now.Add(3*time.Hour + 40*time.Minute)},
					{Kind: aiquota.WindowWeekly, Used: 59, Duration: 7 * 24 * time.Hour, ResetsAt: now.Add(2 * 24 * time.Hour)},
				},
			},
			{
				Provider:  aiquota.ProviderAntigravity,
				Name:      "Antigravity",
				Plan:      "Pro",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now.Add(-20 * time.Second),
				Windows: []aiquota.QuotaWindow{
					{Kind: aiquota.WindowSession, Group: "Claude", Used: 94, Duration: 5 * time.Hour, ResetsAt: now.Add(52 * time.Minute)},
					{Kind: aiquota.WindowSession, Group: "Gemini Pro", Used: 46, Duration: 5 * time.Hour, ResetsAt: now.Add(3 * time.Hour)},
					{Kind: aiquota.WindowSession, Group: "Gemini Flash", Used: 8, Duration: 5 * time.Hour, ResetsAt: now.Add(4 * time.Hour)},
					{Kind: aiquota.WindowSession, Group: "GPT-OSS", Used: 0, Duration: 5 * time.Hour, ResetsAt: now.Add(5 * time.Hour)},
				},
			},
			{
				Provider:  aiquota.ProviderCursor,
				Name:      "Cursor",
				Plan:      "Pro",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now.Add(-45 * time.Second),
				Windows: []aiquota.QuotaWindow{
					{Kind: aiquota.WindowMonthly, Used: 62, DollarsUsed: &used1240, DollarsLimit: &limit20, Duration: 30 * 24 * time.Hour, ResetsAt: now.Add(11 * 24 * time.Hour)},
				},
				Credits: &aiquota.CreditsInfo{UsedUSD: &extraUsed, LimitUSD: &extraLimit},
			},
			{
				Provider:  aiquota.ProviderGemini,
				Name:      "Gemini CLI",
				Status:    aiquota.StatusSignedOut,
				Installed: true,
			},
			{
				Provider:  aiquota.ProviderKimi,
				Name:      "Kimi Code",
				Status:    aiquota.StatusAPIKey,
				Installed: true,
			},
		},
	}
}

func TestQuotasPageStandardAndNarrow(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	a := NewApp()
	a.Snapshot = calmSnapshot()
	now := time.Now()
	a.Quotas = sampleQuotasSnapshot(now)
	saves := 0
	refreshes := 0
	a.OnSettings = func() { saves++ }
	a.RefreshQuotas = func(force bool) {
		if force {
			refreshes++
		}
	}
	a.OpenQuotas()

	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	for _, want := range []string{
		locales.QuotasTitle(),
		locales.QuotasStatConnected(),
		locales.QuotasStatInTray(),
		locales.QuotasStatNextReset(),
		locales.QuotasStatRunningLow(),
		"Claude Code",
		"Codex",
		"Antigravity",
		"Cursor",
		"Gemini CLI",
		"Kimi Code",
		"$12.40 / $20",
		"$4.50 / $50",
		locales.QuotasRightTrayTitle(),
		locales.QuotasRightUpcomingTitle(),
		locales.QuotasRightInactiveTitle(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q on /ai/quotas; texts=%q", want, tt.Texts())
		}
	}

	// Click Refresh button.
	if err := tt.Click(locales.QuotasRefresh()); err != nil {
		t.Fatalf("click refresh: %v", err)
	}
	if refreshes != 1 {
		t.Fatalf("expected force refresh count=1, got %d", refreshes)
	}

	// Filter to "Running low" -> Antigravity stays, Claude Code/Codex/Cursor hidden from grid.
	lowTab, ok := tt.AccessByRole(ui.RoleRadio, locales.QuotasFilterLow())
	if !ok {
		lowTab, ok = tt.AccessByRole(ui.RoleTab, locales.QuotasFilterLow())
	}
	if !ok {
		t.Fatalf("missing segmented tab %q", locales.QuotasFilterLow())
	}
	tt.ClickAt(lowTab.Bounds.X+lowTab.Bounds.W/2, lowTab.Bounds.Y+lowTab.Bounds.H/2)
	tt.Frame()
	if a.quotasTab != 2 {
		t.Fatalf("expected quotasTab=2, got %d", a.quotasTab)
	}
	if !tt.HasText("Antigravity") {
		t.Fatalf("expected Antigravity in low filter")
	}

	// Switch back to "All".
	allTab, ok := tt.AccessByRole(ui.RoleRadio, locales.QuotasFilterAll())
	if !ok {
		allTab, ok = tt.AccessByRole(ui.RoleTab, locales.QuotasFilterAll())
	}
	if !ok {
		t.Fatalf("missing segmented tab %q", locales.QuotasFilterAll())
	}
	tt.ClickAt(allTab.Bounds.X+allTab.Bounds.W/2, allTab.Bounds.Y+allTab.Bounds.H/2)
	tt.Frame()
	if a.quotasTab != 0 {
		t.Fatalf("expected quotasTab=0, got %d", a.quotasTab)
	}

	// Toggle tray visibility pill on Claude Code card.
	if err := tt.Click(locales.QuotasBadgeInTray() + " Claude Code"); err != nil {
		t.Fatalf("click Claude Code tray toggle: %v", err)
	}
	tt.Frame()
	if !slices.Contains(a.QuotaTrayHidden, string(aiquota.ProviderClaude)) || saves != 1 {
		t.Fatalf("expected claude hidden in tray and saves=1; hidden=%v saves=%d", a.QuotaTrayHidden, saves)
	}

	// Reorder Claude Code down in the right sidebar.
	if err := tt.Click(locales.QuotasMoveDown() + " Claude Code"); err != nil {
		t.Fatalf("click move down Claude Code: %v", err)
	}
	tt.Frame()
	if len(a.QuotaOrder) < 2 || a.QuotaOrder[0] != string(aiquota.ProviderCodex) || a.QuotaOrder[1] != string(aiquota.ProviderClaude) {
		t.Fatalf("unexpected QuotaOrder after move down: %v", a.QuotaOrder)
	}
	if saves != 2 {
		t.Fatalf("expected saves=2, got %d", saves)
	}

	// Narrow window (1000x680) and Russian locale must render cleanly without panics or missing sections.
	i18n.SetLocaleFunc(func() string { return "ru" })
	narrow := ui.NewTester(a.View, 1000, 680)
	narrow.Advance(time.Second)
	for _, want := range []string{locales.QuotasTitle(), "Claude Code", "Antigravity"} {
		if !narrow.HasText(want) {
			t.Fatalf("narrow ru view missing %q", want)
		}
	}

	// Empty state when no quotas detected.
	i18n.SetLocaleFunc(nil)
	emptyApp := NewApp()
	emptyApp.Snapshot = calmSnapshot()
	emptyApp.OpenQuotas()
	emptyTT := ui.NewTester(emptyApp.View, 1280, 820)
	emptyTT.Advance(time.Second)
	if !emptyTT.HasText(locales.QuotasEmptyTitle()) {
		t.Fatalf("expected empty state title %q, texts=%q", locales.QuotasEmptyTitle(), emptyTT.Texts())
	}
}

func TestTrayPopoverQuotasAndSectionOrder(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	now := time.Now()
	quotas := sampleQuotasSnapshot(now)
	openedQuotas := 0
	pg := docker.Container{ID: "c1", Name: "postgres", Image: "postgres:16", Ports: "5432"}
	pg.State.Status = "running"
	act := TrayPopoverActions{
		Containers:     []docker.Container{pg},
		Quotas:         quotas,
		DockerContext:  "colima",
		DockerContexts: []string{"colima", "desktop-linux"},
		KubeContext:    "staging-n3",
		KubeContexts:   []string{"staging-n3", "prod"},
		OnOpenQuotas:   func() { openedQuotas++ },
		OnOpenPorts:    func() {},
		OnOpenHearth:   func() {},
		OnOpenSettings: func() {},
	}

	// 1. Default section order: Services -> AI quotas -> Contexts.
	tt := ui.NewTester(func(c *ui.Context) { a.TrayPopoverView(c, act) }, 444, 620)
	tt.Advance(time.Second)

	srvHdr, okSrv := tt.Find(locales.TrayPopoverRunningHint())
	quoLink, okQuo := tt.Find(locales.TrayPopoverAllQuotas())
	ctxBtn, okCtx := tt.Find("Docker: colima")
	if !okSrv || !okQuo || !okCtx {
		t.Fatalf("missing default popover sections: srv=%v quo=%v ctx=%v; texts=%q", okSrv, okQuo, okCtx, tt.Texts())
	}
	if !(srvHdr.Y < quoLink.Y && quoLink.Y < ctxBtn.Y) {
		t.Fatalf("expected default order services(%.1f) < quotas(%.1f) < contexts(%.1f)", srvHdr.Y, quoLink.Y, ctxBtn.Y)
	}
	if !tt.HasText("Claude Code") || !tt.HasText("Antigravity") {
		t.Fatalf("missing quota providers in popover: %q", tt.Texts())
	}

	// Clicking "All quotas →" opens /ai/quotas.
	if err := tt.Click(locales.TrayPopoverAllQuotas()); err != nil {
		t.Fatalf("click all quotas: %v", err)
	}
	if openedQuotas != 1 {
		t.Fatalf("expected openedQuotas=1, got %d", openedQuotas)
	}

	// 2. Custom order: Quotas first, then Contexts, then Services.
	a.TraySections = []string{traySectionQuotas, traySectionContexts, traySectionServices}
	tt.Frame()
	srvHdr, okSrv = tt.Find(locales.TrayPopoverRunningHint())
	quoLink, okQuo = tt.Find(locales.TrayPopoverAllQuotas())
	ctxBtn, okCtx = tt.Find("Docker: colima")
	if !okSrv || !okQuo || !okCtx {
		t.Fatalf("missing reordered popover sections: srv=%v quo=%v ctx=%v", okSrv, okQuo, okCtx)
	}
	if !(quoLink.Y < ctxBtn.Y && ctxBtn.Y < srvHdr.Y) {
		t.Fatalf("expected custom order quotas(%.1f) < contexts(%.1f) < services(%.1f)", quoLink.Y, ctxBtn.Y, srvHdr.Y)
	}

	// 3. Hide Codex provider -> Claude Code stays, Codex gone.
	a.QuotaTrayHidden = []string{string(aiquota.ProviderCodex)}
	tt.Frame()
	if tt.HasText("Codex") || !tt.HasText("Claude Code") {
		t.Fatalf("expected Codex hidden and Claude Code shown; texts=%q", tt.Texts())
	}

	// 4. Hide entire AI quotas section -> All quotas link disappears.
	a.TrayHiddenSections = []string{traySectionQuotas}
	tt.Frame()
	if tt.HasText(locales.TrayPopoverAllQuotas()) {
		t.Fatalf("expected quotas section hidden; texts=%q", tt.Texts())
	}
}

func TestSettingsTrayPanelControls(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Quotas = sampleQuotasSnapshot(time.Now())
	saves := 0
	a.OnSettings = func() { saves++ }
	a.router.Push(settings.path)

	// Use a tall canvas so the Tray panel card at the bottom of the right column is directly clickable.
	tt := ui.NewTester(a.View, 1280, 1200)
	tt.Advance(time.Second)

	for _, want := range []string{
		locales.SettingsTrayPanelTitle(),
		locales.SettingsTraySectionsTitle(),
		locales.SettingsTraySectionServices(),
		locales.SettingsTraySectionQuotas(),
		locales.SettingsTraySectionContexts(),
		locales.SettingsTrayProvidersTitle(),
		locales.SettingsQuotaNotifyTitle(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("settings missing %q", want)
		}
	}

	// Move AI quotas up -> becomes section 0.
	if err := tt.Click(locales.QuotasMoveUp() + " " + locales.SettingsTraySectionQuotas()); err != nil {
		t.Fatalf("move AI quotas up: %v", err)
	}
	tt.Frame()
	if len(a.TraySections) == 0 || a.TraySections[0] != traySectionQuotas || saves != 1 {
		t.Fatalf("expected quotas moved to index 0; TraySections=%v saves=%d", a.TraySections, saves)
	}

	// Toggle AI quotas section off.
	info, ok := tt.AccessByRole(ui.RoleSwitch, locales.SettingsTraySectionQuotas())
	if !ok {
		t.Fatalf("missing switch %q", locales.SettingsTraySectionQuotas())
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	tt.Frame()
	if !slices.Contains(a.TrayHiddenSections, traySectionQuotas) || saves != 2 {
		t.Fatalf("expected quotas section hidden; TrayHiddenSections=%v saves=%d", a.TrayHiddenSections, saves)
	}

	// Toggle low-quota notification off.
	notifyInfo, ok := tt.AccessByRole(ui.RoleSwitch, locales.SettingsQuotaNotifyTitle())
	if !ok {
		t.Fatalf("missing switch %q", locales.SettingsQuotaNotifyTitle())
	}
	tt.ClickAt(notifyInfo.Bounds.X+notifyInfo.Bounds.W/2, notifyInfo.Bounds.Y+notifyInfo.Bounds.H/2)
	tt.Frame()
	if a.QuotaNotifyLow || saves != 3 {
		t.Fatalf("expected QuotaNotifyLow=false and saves=3; notify=%v saves=%d", a.QuotaNotifyLow, saves)
	}

	// Clicking the providers link opens /ai/quotas.
	if err := tt.Click(locales.SettingsTrayProvidersTitle()); err != nil {
		t.Fatalf("click providers link: %v", err)
	}
	tt.Frame()
	if a.router.Path() != quotasPage {
		t.Fatalf("expected navigation to %q, got %q", quotasPage, a.router.Path())
	}
}

func TestTrayPopoverQuotaWindowOrderAndInlineReset(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	now := time.Now()
	reset5h := now.Add(2*time.Hour + 15*time.Minute)
	resetWk := now.Add(4 * 24 * time.Hour)
	wantClock := "· " + reset5h.Local().Format("15:04")

	// Intentionally feed reversed windows (weekly before 5h, Claude before Gemini)
	// to verify UI normalizes 5h on the left and Weekly on the right.
	snap := &aiquota.Snapshot{
		CheckedAt: now,
		Providers: []aiquota.ProviderQuota{
			{
				Provider:  aiquota.ProviderCodex,
				Name:      "Codex",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now,
				Windows: []aiquota.QuotaWindow{
					{ID: "wk", Kind: aiquota.WindowWeekly, Used: 50, Duration: 7 * 24 * time.Hour, ResetsAt: resetWk},
					{ID: "5h", Kind: aiquota.WindowSession, Used: 10, Duration: 5 * time.Hour, ResetsAt: reset5h},
				},
			},
			{
				Provider:  aiquota.ProviderAntigravity,
				Name:      "Antigravity",
				Status:    aiquota.StatusOK,
				Installed: true,
				UpdatedAt: now,
				Windows: []aiquota.QuotaWindow{
					{ID: "c-wk", Group: "Claude", Kind: aiquota.WindowWeekly, Used: 40, Duration: 7 * 24 * time.Hour, ResetsAt: resetWk},
					{ID: "g-wk", Group: "Gemini", Kind: aiquota.WindowWeekly, Used: 25, Duration: 7 * 24 * time.Hour, ResetsAt: resetWk},
					{ID: "c-5h", Group: "Claude", Kind: aiquota.WindowSession, Used: 15, Duration: 5 * time.Hour, ResetsAt: reset5h},
					{ID: "g-5h", Group: "Gemini", Kind: aiquota.WindowSession, Used: 5, Duration: 5 * time.Hour, ResetsAt: reset5h},
				},
			},
		},
	}

	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			a := NewApp()
			a.Snapshot = calmSnapshot()
			act := TrayPopoverActions{
				Quotas:       snap,
				OnOpenQuotas: func() {},
			}
			tt := ui.NewTester(func(c *ui.Context) { a.TrayPopoverView(c, act) }, trayPopoverWidth, 560)
			tt.Advance(time.Second)

			short5h := locales.QuotasWindowShort5h()
			shortWk := locales.QuotasWindowShortWeek()
			wantDate := "· " + locales.AgoOn(resetWk.Local())

			lbl5h, ok5h := tt.Find(short5h)
			lblWk, okWk := tt.Find(shortWk)
			if !ok5h || !okWk {
				t.Fatalf("missing window labels %q/%q; texts=%q", short5h, shortWk, tt.Texts())
			}
			if !(lbl5h.X < lblWk.X) {
				t.Fatalf("expected 5h (x=%.1f) to the left of Weekly (x=%.1f)", lbl5h.X, lblWk.X)
			}

			gem5h, okGem := tt.Find("Gemini " + short5h)
			cl5h, okCl := tt.Find("Claude " + short5h)
			if !okGem || !okCl {
				t.Fatalf("expected Gemini/Claude 5h labels on the left; texts=%q", tt.Texts())
			}
			if !(gem5h.Y < cl5h.Y) {
				t.Fatalf("expected Gemini row (y=%.1f) above Claude row (y=%.1f)", gem5h.Y, cl5h.Y)
			}
			if !(gem5h.X < lblWk.X && cl5h.X < lblWk.X) {
				t.Fatalf("expected Antigravity 5h bars (x=%.1f, %.1f) left of Weekly column (x=%.1f)", gem5h.X, cl5h.X, lblWk.X)
			}

			clk, okClk := tt.Find(wantClock)
			dt, okDt := tt.Find(wantDate)
			if !okClk || !okDt {
				t.Fatalf("missing inline reset labels %q / %q; texts=%q", wantClock, wantDate, tt.Texts())
			}
			if clk.X+clk.W > trayPopoverWidth || dt.X+dt.W > trayPopoverWidth {
				t.Fatalf("inline reset overflows popover width %d: clock=%.1f date=%.1f", trayPopoverWidth, clk.X+clk.W, dt.X+dt.W)
			}
		})
	}
}
