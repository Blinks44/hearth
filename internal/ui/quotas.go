package ui

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/aiquota"
	"github.com/Blinks44/hearth/locales"
)

const quotasPage = "/ai/quotas"

const (
	traySectionServices = "services"
	traySectionQuotas   = "quotas"
	traySectionContexts = "contexts"
)

var defaultTraySections = []string{traySectionServices, traySectionQuotas, traySectionContexts}

// quotaProviderBrand maps an aiquota.ProviderID to a resources/icons/logo slug
// when available, or "" for lucide fallback.
func quotaProviderIcon(id aiquota.ProviderID) string {
	switch id {
	case aiquota.ProviderClaude:
		return "brand/claude"
	case aiquota.ProviderCodex:
		return "brand/openai"
	case aiquota.ProviderAntigravity:
		return "brand/antigravity"
	case aiquota.ProviderGemini:
		return "brand/googlegemini"
	case aiquota.ProviderCursor:
		return "brand/cursor"
	case aiquota.ProviderCopilot:
		return "brand/githubcopilot"
	case aiquota.ProviderGrok:
		return "brand/grok"
	case aiquota.ProviderKimi:
		return "brand/kimi"
	case aiquota.ProviderOpenCode:
		return "brand/opencode"
	case aiquota.ProviderJetBrains:
		return "lucide/code-xml"
	case aiquota.ProviderWindsurf, aiquota.ProviderCline, aiquota.ProviderKiro, aiquota.ProviderKilo:
		return "lucide/sparkles"
	case aiquota.ProviderAmp, aiquota.ProviderAugment, aiquota.ProviderCodeRabbit, aiquota.ProviderHuggingFace:
		return "lucide/bot"
	default:
		return "lucide/bot"
	}
}

func quotaProviderLogo(c *ui.Context, id aiquota.ProviderID, size float32) {
	iconOrLogo(c, quotaProviderIcon(id), size, c.Theme().Text)
}

func quotaProviderTile(c *ui.Context, id aiquota.ProviderID, size float32) {
	pal := Colors(c.Theme())
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() { quotaProviderLogo(c, id, size*0.58) })
}

// --- Tray section & provider order preference helpers ---

func (a *App) traySectionOrder() []string {
	seen := make(map[string]bool, len(defaultTraySections))
	out := make([]string, 0, len(defaultTraySections))
	for _, s := range a.TraySections {
		if slices.Contains(defaultTraySections, s) && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range defaultTraySections {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (a *App) isTraySectionHidden(id string) bool {
	return slices.Contains(a.TrayHiddenSections, id)
}

func (a *App) setTraySectionVisible(id string, visible bool) {
	hidden := slices.DeleteFunc(slices.Clone(a.TrayHiddenSections), func(s string) bool { return s == id })
	if !visible {
		hidden = append(hidden, id)
	}
	a.TrayHiddenSections = hidden
	a.savePreferences()
}

func (a *App) moveTraySection(id string, delta int) {
	order := a.traySectionOrder()
	idx := slices.Index(order, id)
	next := idx + delta
	if idx < 0 || next < 0 || next >= len(order) {
		return
	}
	order[idx], order[next] = order[next], order[idx]
	a.TraySections = order
	a.savePreferences()
}

func (a *App) isQuotaTrayHidden(id aiquota.ProviderID) bool {
	return slices.Contains(a.QuotaTrayHidden, string(id))
}

func (a *App) setQuotaTrayVisible(id aiquota.ProviderID, visible bool) {
	sid := string(id)
	hidden := slices.DeleteFunc(slices.Clone(a.QuotaTrayHidden), func(s string) bool { return s == sid })
	if !visible {
		hidden = append(hidden, sid)
	}
	a.QuotaTrayHidden = hidden
	a.savePreferences()
}

func (a *App) moveQuotaProvider(ordered []aiquota.ProviderQuota, id aiquota.ProviderID, delta int) {
	ids := make([]string, len(ordered))
	for i, p := range ordered {
		ids[i] = string(p.Provider)
	}
	idx := slices.Index(ids, string(id))
	next := idx + delta
	if idx < 0 || next < 0 || next >= len(ids) {
		return
	}
	ids[idx], ids[next] = ids[next], ids[idx]
	a.QuotaOrder = ids
	a.savePreferences()
}

func (a *App) traySectionOrderSummary() string {
	order := a.traySectionOrder()
	var parts []string
	for _, id := range order {
		if a.isTraySectionHidden(id) {
			continue
		}
		switch id {
		case traySectionServices:
			parts = append(parts, locales.SettingsTraySectionShortServices())
		case traySectionQuotas:
			parts = append(parts, locales.SettingsTraySectionShortQuotas())
		case traySectionContexts:
			parts = append(parts, locales.SettingsTraySectionShortContexts())
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " → ")
}

// trayVisibleQuotas returns connected providers that are not hidden from tray,
// in the user's configured order.
func (a *App) trayVisibleQuotas(snap *aiquota.Snapshot) []aiquota.ProviderQuota {
	if snap == nil {
		return nil
	}
	ordered := snap.Ordered(a.QuotaOrder)
	out := make([]aiquota.ProviderQuota, 0, len(ordered))
	for _, p := range ordered {
		if !a.isQuotaTrayHidden(p.Provider) {
			out = append(out, p)
		}
	}
	return out
}

// --- /ai/quotas page ---

func (a *App) quotasView(c *ui.Context, p page) {
	now := c.Now()
	t := c.Theme()
	pal := Colors(t)

	snap := aiquota.Snapshot{}
	if a.Quotas != nil {
		snap = *a.Quotas
	}
	ordered := snap.Ordered(a.QuotaOrder)
	inactive := snap.Inactive()
	upcoming := snap.UpcomingResets(now)
	lowCount := snap.LowCount(now)
	shownCount := 0
	for _, pq := range ordered {
		if !a.isQuotaTrayHidden(pq.Provider) {
			shownCount++
		}
	}

	sub := p.subtitle()
	if !snap.CheckedAt.IsZero() {
		sub = sub + " · " + quotaUpdatedText(c, snap.CheckedAt)
	}

	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), sub, headerExtras{
			after: func() {
				btn := ui.Button(c.Key("quotas-refresh"), "").Label(locales.QuotasRefresh()).
					Height(controlHeight).Padding(0, 12).Disabled(a.RefreshQuotas == nil || a.QuotasRefreshing).
					OnClick(func() {
						if a.RefreshQuotas != nil {
							a.RefreshQuotas(true)
						}
					})
				btn.Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						spinIcon(c, "refresh-cw", 13, t.TextMuted, a.QuotasRefreshing)
						ui.Text(c, locales.QuotasRefresh()).FontSize(12.5).FontWeight(500).SingleLine()
					})
				})
			},
		})

		ui.Scroll(c.Key("quotas-scroll")).Grow(1).Children(func() {
			ui.Column(c).Padding(22, 28, 28, 28).Gap(16).Children(func() {
				// Lead title + 4 stat cards row (matching Sketch 161).
				winW, _ := c.Size()
				wide := winW == 0 || winW >= 1080
				a.quotasTopRow(c, p, snap, ordered, upcoming, shownCount, lowCount, wide)

				if len(ordered) == 0 && len(inactive) == 0 {
					a.quotasEmptyState(c)
					return
				}

				// Filter tabs + sort dropdown row.
				a.quotasControlsRow(c, len(ordered), shownCount, lowCount)

				// Pace-tick legend caption.
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Width(2).Height(10).Radius(1).Background(t.TextMuted).Shrink(0)
					ui.Text(c, locales.QuotasPaceLegend()).FontSize(11.5).TextColor(pal.TextFaint).Wrap()
				})

				// Main 2-column split: left provider cards, right tray/resets/inactive cards.
				filtered := a.filterAndSortQuotas(ordered, now)
				if wide {
					ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.quotasProviderGrid(c, filtered, now, winW == 0 || winW >= 1180)
						})
						ui.Column(c).Width(332).Shrink(0).Gap(12).Children(func() {
							a.quotasRightSidebar(c, ordered, upcoming, inactive, shownCount, now)
						})
					})
				} else {
					ui.Column(c).Gap(14).Children(func() {
						a.quotasProviderGrid(c, filtered, now, false)
						a.quotasRightSidebar(c, ordered, upcoming, inactive, shownCount, now)
					})
				}
			})
		})
	})
}

func (a *App) quotasTopRow(
	c *ui.Context,
	p page,
	snap aiquota.Snapshot,
	ordered []aiquota.ProviderQuota,
	upcoming []aiquota.UpcomingReset,
	shownCount, lowCount int,
	wide bool,
) {
	t := c.Theme()
	pal := Colors(t)

	nextVal := "—"
	nextSub := locales.QuotasStatNoReset()
	if len(upcoming) > 0 {
		u := upcoming[0]
		nextVal = formatQuotaResetShort(u.Window.ResetsAt.Sub(c.Now()))
		if u.Window.Group != "" {
			nextSub = u.Name + " · " + u.Window.Group
		} else {
			nextSub = u.Name
		}
	}

	lowSub := locales.QuotasStatAllHealthy()
	lowDot := pal.TextFaint
	if lowCount > 0 {
		lowSub = locales.QuotasStatRunningLowSub()
		lowDot = t.Danger
	}

	renderLead := func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
			ui.Box(c).Size(48, 48).Radius(12).Background(pal.AccentSoft).Center().Shrink(0).Children(func() {
				ui.Icon(c, Icon("gauge")).Size(24, 24).TextColor(Accent)
			})
			ui.Column(c).Gap(3).MinWidth(0).Children(func() {
				ui.Text(c, p.title()).FontSize(22).FontWeight(600).SingleLine()
				ui.Text(c, locales.QuotasLeadSubtitle()).FontSize(12.5).TextColor(t.TextMuted).Wrap()
			})
		})
	}

	renderStats := func() {
		ui.Row(c).Gap(10).Shrink(0).Children(func() {
			quotaStatTile(c, t.Success, locales.QuotasStatConnected(), strconv.Itoa(len(ordered)), locales.QuotasStatOfFound(snap.FoundCount()))
			quotaStatTile(c, Accent, locales.QuotasStatInTray(), locales.QuotasStatInTrayValue(shownCount, len(ordered)), locales.QuotasStatVisibleInPopover())
			quotaStatTile(c, pal.Info, locales.QuotasStatNextReset(), nextVal, nextSub)
			quotaStatTile(c, lowDot, locales.QuotasStatRunningLow(), strconv.Itoa(lowCount), lowSub)
		})
	}

	if wide {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			renderLead()
			renderStats()
		})
	} else {
		ui.Column(c).Gap(12).Children(func() {
			renderLead()
			renderStats()
		})
	}
}

func quotaStatTile(c *ui.Context, dot ui.Color, label, value, sub string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).MinWidth(122).Padding(10, 14).Gap(3).Radius(10).
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, value).FontSize(17).FontWeight(600).SingleLine()
		ui.Text(c, sub).FontSize(11).TextColor(pal.TextFaint).SingleLine().MaxWidth(136)
	})
}

func (a *App) quotasControlsRow(c *ui.Context, total, inTray, low int) {
	t := c.Theme()
	pal := Colors(t)
	tabs := []struct {
		label string
		count int
	}{
		{locales.QuotasFilterAll(), total},
		{locales.QuotasFilterInTray(), inTray},
		{locales.QuotasFilterLow(), low},
	}
	sortOptions := []string{
		locales.QuotasSortCustom(),
		locales.QuotasSortLeastLeft(),
		locales.QuotasSortNextReset(),
	}
	if a.quotasSort < 0 || a.quotasSort >= len(sortOptions) {
		a.quotasSort = 0
	}
	a.quotasSortLabel = sortOptions[a.quotasSort]

	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		seg := ui.SegmentedBase(c.Key("quotas-filter"), &a.quotasTab, len(tabs))
		seg.Track.Gap(2).Padding(2).Radius(8).Background(pal.Muted).Children(func() {
			for i, tab := range tabs {
				s := seg.Segment(i).Gap(5).Padding(5, 12).Radius(6).AlignItems(ui.Center).Label(tab.label)
				color := t.TextMuted
				if i == a.quotasTab {
					s.Background(t.Background).Shadow(0, 1, 2, 0, t.Text.Alpha(0.1))
					color = t.Text
				}
				s.Children(func() {
					ui.Text(c, tab.label).FontSize(12.5).TextColor(color).SingleLine()
					countColor := pal.TextFaint
					if i == 2 && tab.count > 0 {
						countColor = pal.DangerText
					}
					ui.Text(c, strconv.Itoa(tab.count)).FontSize(11.5).TextColor(countColor)
				})
			}
		})
		ui.Box(c).Grow(1).MinWidth(0).Height(1)
		ui.Select(c.Key("quotas-sort"), &a.quotasSortLabel, sortOptions).Width(176).OnChange(func() {
			if idx := slices.Index(sortOptions, a.quotasSortLabel); idx >= 0 {
				a.quotasSort = idx
			}
		})
	})
}

func (a *App) filterAndSortQuotas(ordered []aiquota.ProviderQuota, now time.Time) []aiquota.ProviderQuota {
	var out []aiquota.ProviderQuota
	for _, p := range ordered {
		switch a.quotasTab {
		case 1:
			if a.isQuotaTrayHidden(p.Provider) {
				continue
			}
		case 2:
			if !p.HasRunningLow(now) {
				continue
			}
		}
		out = append(out, p)
	}
	switch a.quotasSort {
	case 1: // Least remaining
		slices.SortStableFunc(out, func(x, y aiquota.ProviderQuota) int {
			lx, _ := x.MinLeft(now)
			ly, _ := y.MinLeft(now)
			if c := cmp.Compare(lx, ly); c != 0 {
				return c
			}
			return cmp.Compare(x.Name, y.Name)
		})
	case 2: // Soonest reset
		slices.SortStableFunc(out, func(x, y aiquota.ProviderQuota) int {
			rx, okx := x.NearestReset(now)
			ry, oky := y.NearestReset(now)
			switch {
			case okx && !oky:
				return -1
			case !okx && oky:
				return 1
			case okx && oky:
				if c := rx.ResetsAt.Compare(ry.ResetsAt); c != 0 {
					return c
				}
			}
			return cmp.Compare(x.Name, y.Name)
		})
	}
	return out
}

func (a *App) quotasProviderGrid(c *ui.Context, providers []aiquota.ProviderQuota, now time.Time, twoCol bool) {
	t := c.Theme()
	pal := Colors(t)
	if len(providers) == 0 {
		ui.Column(c.Key("quotas-filter-empty")).Padding(28, 20).Radius(12).
			Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Gap(6).Children(func() {
			ui.Icon(c, Icon("gauge")).Size(22, 22).TextColor(pal.TextFaint)
			ui.Text(c, locales.QuotasFilterEmptyTitle()).FontSize(13.5).FontWeight(500).TextColor(t.TextMuted)
		})
		return
	}
	if !twoCol {
		for _, p := range providers {
			a.quotaProviderCard(c.Key("qcard-"+string(p.Provider)), p, now)
		}
		return
	}
	for i := 0; i < len(providers); i += 2 {
		ui.Row(c.Key("qrow-" + strconv.Itoa(i))).Gap(12).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				a.quotaProviderCard(c.Key("qcard-"+string(providers[i].Provider)), providers[i], now)
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				if i+1 < len(providers) {
					a.quotaProviderCard(c.Key("qcard-"+string(providers[i+1].Provider)), providers[i+1], now)
				}
			})
		})
	}
}

func (a *App) quotaProviderCard(c *ui.Context, p aiquota.ProviderQuota, now time.Time) {
	t := c.Theme()
	pal := Colors(t)
	isLow := p.HasRunningLow(now)
	inTray := !a.isQuotaTrayHidden(p.Provider)

	borderColor := pal.Line
	if isLow {
		borderColor = t.Danger
	}

	ui.Column(c).Grow(1).Padding(16).Gap(14).Radius(12).
		Background(t.Background).BorderWidth(1).BorderColor(borderColor).Children(func() {
		// Card header: logo + Name + plan/updated + [In tray / Hidden] toggle badge.
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			quotaProviderTile(c, p.Provider, 30)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).FontSize(14).FontWeight(600).SingleLine()
					if p.IsStale(now) {
						aiTag(c, locales.QuotasStaleBadge(), pal.WarningSoft, pal.WarningText)
					}
				})
				metaParts := []string{}
				if p.Plan != "" {
					metaParts = append(metaParts, p.Plan)
				}
				if !p.UpdatedAt.IsZero() && !p.SourceDateUnknown {
					metaParts = append(metaParts, quotaUpdatedShortText(c, p.UpdatedAt))
				}
				if len(metaParts) > 0 {
					ui.Text(c, strings.Join(metaParts, " · ")).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
				}
			})
			pillBg, pillFg := pal.Subtle, t.TextMuted
			pillIcon, pillText := "eye-off", locales.QuotasBadgeHidden()
			if inTray {
				pillBg, pillFg = pal.AccentSoft, pal.AccentInk
				pillIcon, pillText = "eye", locales.QuotasBadgeInTray()
			}
			pid := p.Provider
			nextVis := !inTray
			badge := ui.ButtonBase(c.Key("tray-toggle-"+string(pid))).Row().Gap(5).Padding(4, 9).Radius(999).
				AlignItems(ui.Center).Background(pillBg).Label(pillText + " " + p.Name).Tooltip(locales.QuotasToggleTrayTooltip()).
				OnClick(func() { a.setQuotaTrayVisible(pid, nextVis) })
			badge.Children(func() {
				ui.Icon(c, Icon(pillIcon)).Size(11, 11).TextColor(pillFg)
				ui.Text(c, pillText).FontSize(11).FontWeight(500).TextColor(pillFg).SingleLine()
			})
		})

		// Windows list (always ordered shorter cadence first: 5h -> daily -> weekly -> monthly).
		windows := aiquota.NormalizeWindows(p.Windows)
		ui.Column(c).Gap(12).Children(func() {
			for i, w := range windows {
				quotaWindowBlock(c.Key("win-"+strconv.Itoa(i)), w, now)
			}
		})

		// Optional projection / extra usage / review count footer.
		var soonestRunsOut time.Time
		for _, w := range windows {
			if ro := w.RunsOutAt(now); !ro.IsZero() {
				if soonestRunsOut.IsZero() || ro.Before(soonestRunsOut) {
					soonestRunsOut = ro
				}
			}
		}
		if !soonestRunsOut.IsZero() {
			ui.Row(c).Padding(8, 10).Radius(8).Background(pal.Subtle).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, locales.QuotasRunsOutAtPace()).FontSize(11.5).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				ui.Text(c, "~"+formatQuotaResetShort(soonestRunsOut.Sub(now))).Font(FontMono).FontSize(11.5).FontWeight(600).TextColor(pal.WarningText).SingleLine()
			})
		}
		if p.Credits != nil {
			val := ""
			switch {
			case p.Credits.Unlimited:
				val = locales.QuotasCreditsUnlimited()
			case p.Credits.UsedUSD != nil && p.Credits.LimitUSD != nil:
				val = fmt.Sprintf("$%.2f / $%.0f", *p.Credits.UsedUSD, *p.Credits.LimitUSD)
			case p.Credits.RemainingUSD != nil:
				val = fmt.Sprintf("$%.2f", *p.Credits.RemainingUSD)
			case p.Credits.UsedUSD != nil:
				val = fmt.Sprintf("$%.2f", *p.Credits.UsedUSD)
			}
			if val != "" {
				ui.Row(c).Padding(8, 10).Radius(8).Background(pal.Subtle).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, locales.QuotasExtraUsage()).FontSize(11.5).TextColor(t.TextMuted).Grow(1).MinWidth(0)
					ui.Text(c, val).Font(FontMono).FontSize(11.5).FontWeight(500).TextColor(t.Text).SingleLine()
				})
			}
		}
		if p.ReviewCount != nil {
			ui.Row(c).Padding(8, 10).Radius(8).Background(pal.Subtle).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, locales.QuotasReviewsCount(*p.ReviewCount)).FontSize(11.5).TextColor(t.TextMuted)
			})
		}
	})
}

func quotaWindowBlock(c *ui.Context, w aiquota.QuotaWindow, now time.Time) {
	t := c.Theme()
	pal := Colors(t)
	left := w.EffectiveLeft(now)
	pace := w.PaceStatus(now)
	barColor, pctColor := quotaBarColors(t, pal, w, now)

	valText := fmt.Sprintf("%.0f%%", math.Round(left))
	if w.DollarsUsed != nil && w.DollarsLimit != nil && *w.DollarsLimit > 0 {
		valText = fmt.Sprintf("$%.2f / $%.0f", *w.DollarsUsed, *w.DollarsLimit)
	}

	ui.Column(c).Gap(5).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, quotaWindowLabel(w)).FontSize(12.5).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
			ui.Text(c, valText).Font(FontMono).FontSize(12.5).FontWeight(600).TextColor(pctColor).SingleLine()
		})

		expectedLeft, hasTick := w.TimeLeft(now)
		quotaProgressBar(c, left, expectedLeft, hasTick, barColor, 7)

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			resetLabel := ""
			switch {
			case w.HasReset(now):
				resetLabel = locales.QuotasResetDone()
			case !w.ResetsAt.IsZero():
				resetLabel = locales.QuotasResetIn(formatQuotaResetShort(w.ResetsAt.Sub(now)))
				if at := formatQuotaResetAt(w.ResetsAt, now); at != "" {
					resetLabel += " · " + at
				}
			}
			ui.Text(c, resetLabel).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0).SingleLine()
			if paceText, paceFg := quotaPaceBadge(t, pal, pace); paceText != "" {
				ui.Text(c, paceText).FontSize(11).FontWeight(500).TextColor(paceFg).SingleLine()
			}
		})
	})
}

func quotaProgressBar(c *ui.Context, leftPct, expectedLeftPct float64, showTick bool, fillColor ui.Color, height float32) {
	t := c.Theme()
	pal := Colors(t)
	radius := height / 2
	ui.Row(c).Height(height).Radius(radius).Background(pal.Line).Children(func() {
		if leftPct > 0 {
			ui.Box(c).WidthPercent(float32(min(max(leftPct, 0), 100))).Height(height).Radius(radius).Background(fillColor)
		}
		if showTick && expectedLeftPct > 2 && expectedLeftPct < 98 {
			ui.Box(c).Absolute().LeftPercent(float32(expectedLeftPct)).Top(-2).Width(2).Height(height + 4).Radius(1).Background(t.TextMuted)
		}
	})
}

func quotaBarColors(t *ui.Theme, pal Palette, w aiquota.QuotaWindow, now time.Time) (bar, text ui.Color) {
	pace := w.PaceStatus(now)
	switch {
	case w.IsRunningLow(now) || w.IsExhausted(now):
		return t.Danger, pal.DangerText
	case w.IsWarning(now) || pace == aiquota.PaceFast:
		return t.Warning, pal.WarningText
	default:
		return t.Success, t.Text
	}
}

func quotaPaceBadge(t *ui.Theme, pal Palette, pace aiquota.PaceKind) (string, ui.Color) {
	switch pace {
	case aiquota.PaceSpare:
		return locales.QuotasPaceSpare(), pal.SuccessText
	case aiquota.PaceNormal:
		return locales.QuotasPaceNormal(), pal.TextFaint
	case aiquota.PaceFast:
		return locales.QuotasPaceFast(), pal.WarningText
	case aiquota.PaceLow:
		return locales.QuotasPaceLow(), pal.DangerText
	case aiquota.PaceExhausted:
		return locales.QuotasPaceExhausted(), pal.DangerText
	default:
		return "", pal.TextFaint
	}
}

func (a *App) quotasRightSidebar(
	c *ui.Context,
	ordered []aiquota.ProviderQuota,
	upcoming []aiquota.UpcomingReset,
	inactive []aiquota.ProviderQuota,
	shownCount int,
	now time.Time,
) {
	t := c.Theme()
	pal := Colors(t)

	// 1. "In tray panel" order + toggle card.
	card(c.Key("quotas-right-tray"), locales.QuotasRightTrayTitle(), locales.QuotasStatInTrayValue(shownCount, len(ordered)), nil, func() {
		for i, p := range ordered {
			pid := p.Provider
			vis := !a.isQuotaTrayHidden(pid)
			idx := i
			ui.Row(c.Key("tray-prov-"+string(pid))).MinHeight(44).Padding(6, 14).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider).Children(func() {
				quotaProviderLogo(c, pid, 16)
				ui.Text(c, p.Name).FontSize(12.5).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
				iconButton(c.Key("up-"+string(pid)), "chevron-up", locales.QuotasMoveUp()+" "+p.Name).
					Disabled(idx == 0).OnClick(func() { a.moveQuotaProvider(ordered, pid, -1) })
				iconButton(c.Key("down-"+string(pid)), "chevron-down", locales.QuotasMoveDown()+" "+p.Name).
					Disabled(idx == len(ordered)-1).OnClick(func() { a.moveQuotaProvider(ordered, pid, 1) })
				toggle(c, "tray-sw-"+string(pid), &vis).Label(p.Name).OnChange(func() {
					a.setQuotaTrayVisible(pid, vis)
				})
			})
		}
		navListRow(c, "quotas-open-settings", locales.QuotasRightTraySectionsTitle(), a.traySectionOrderSummary(), locales.SettingsTitle(), func() {
			a.router.Push(settings.path)
		})
	})

	// 2. "Upcoming resets" card.
	card(c.Key("quotas-right-resets"), locales.QuotasRightUpcomingTitle(), locales.QuotasRightUpcomingSub(), nil, func() {
		if len(upcoming) == 0 {
			settingRow(c, locales.QuotasRightUpcomingEmpty(), "", nil)
			return
		}
		limit := min(len(upcoming), 6)
		for i := range limit {
			u := upcoming[i]
			left := u.Window.EffectiveLeft(now)
			bg, fg := pal.SuccessSoft, pal.SuccessText
			switch {
			case u.Window.IsRunningLow(now):
				bg, fg = pal.DangerSoft, pal.DangerText
			case u.Window.IsWarning(now):
				bg, fg = pal.WarningSoft, pal.WarningText
			}
			title := u.Name
			if u.Window.Group != "" {
				title = title + " · " + u.Window.Group
			}
			meta := quotaShortWindowLabel(u.Window) + " · " + formatQuotaResetShort(u.Window.ResetsAt.Sub(now))
			ui.Row(c.Key("reset-"+strconv.Itoa(i))).MinHeight(44).Padding(6, 14).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider).Children(func() {
				quotaProviderLogo(c, u.Provider, 15)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, title).FontSize(12.5).FontWeight(500).SingleLine()
					ui.Text(c, meta).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				})
				aiTag(c, fmt.Sprintf("%.0f%%", math.Round(left)), bg, fg)
			})
		}
	})

	// 3. "Without active quota" card.
	if len(inactive) > 0 {
		card(c.Key("quotas-right-inactive"), locales.QuotasRightInactiveTitle(), strconv.Itoa(len(inactive)), nil, func() {
			for _, p := range inactive {
				ui.Row(c.Key("inact-"+string(p.Provider))).MinHeight(44).Padding(6, 14).Gap(8).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider).Children(func() {
					quotaProviderLogo(c, p.Provider, 15)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, p.Name).FontSize(12.5).FontWeight(500).SingleLine()
						ui.Text(c, quotaInactiveReason(p.Status)).FontSize(11).TextColor(pal.TextFaint).Wrap()
					})
				})
			}
		})
	}
}

func (a *App) quotasEmptyState(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key("quotas-empty")).Padding(40, 28).Radius(12).
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Gap(8).Children(func() {
		ui.Icon(c, Icon("gauge")).Size(28, 28).TextColor(pal.TextFaint)
		ui.Text(c, locales.QuotasEmptyTitle()).FontSize(15).FontWeight(600).TextAlign(ui.Center)
		ui.Text(c, locales.QuotasEmptyBody()).FontSize(12.5).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(520).Wrap()
	})
}

// --- Shared formatting helpers (used by /ai/quotas, TrayPopoverView, and OS notifications) ---

// QuotaWindowDisplayName returns a localized human-readable label for w.
func QuotaWindowDisplayName(w aiquota.QuotaWindow) string {
	return quotaWindowLabel(w)
}

func quotaWindowLabel(w aiquota.QuotaWindow) string {
	if w.Label != "" {
		return w.Label
	}
	if w.Group != "" {
		return w.Group + " · " + quotaShortWindowLabel(w)
	}
	switch w.Kind {
	case aiquota.WindowSession:
		if w.Model != "" {
			return w.Model + " · " + locales.QuotasWindowShort5h()
		}
		return locales.QuotasWindowSession()
	case aiquota.WindowDaily:
		if w.Model != "" {
			return w.Model + " · " + locales.QuotasWindowShortDaily()
		}
		return locales.QuotasWindowDaily()
	case aiquota.WindowWeekly:
		if w.Model != "" {
			return locales.QuotasWindowWeeklyModel(w.Model)
		}
		return locales.QuotasWindowWeekly()
	case aiquota.WindowMonthly:
		if w.Model != "" {
			return w.Model + " · " + locales.QuotasWindowShortMonth()
		}
		return locales.QuotasWindowMonthly()
	case aiquota.WindowCredits:
		return locales.QuotasWindowCredits()
	default:
		return string(w.Kind)
	}
}

func quotaShortWindowLabel(w aiquota.QuotaWindow) string {
	if w.Model != "" && w.Kind != aiquota.WindowWeekly {
		return w.Model
	}
	switch w.Kind {
	case aiquota.WindowSession:
		return locales.QuotasWindowShort5h()
	case aiquota.WindowDaily:
		return locales.QuotasWindowShortDaily()
	case aiquota.WindowWeekly:
		return locales.QuotasWindowShortWeek()
	case aiquota.WindowMonthly:
		return locales.QuotasWindowShortMonth()
	case aiquota.WindowCredits:
		return locales.QuotasWindowCredits()
	default:
		return string(w.Kind)
	}
}

func quotaInactiveReason(s aiquota.Status) string {
	switch s {
	case aiquota.StatusSignedOut:
		return locales.QuotasStatusSignedOut()
	case aiquota.StatusAPIKey:
		return locales.QuotasStatusAPIKey()
	case aiquota.StatusExpired:
		return locales.QuotasStatusExpired()
	case aiquota.StatusUnavailable:
		return locales.QuotasStatusUnavailable()
	default:
		return locales.QuotasStatusNoData()
	}
}

func formatQuotaResetShort(d time.Duration) string {
	if d <= 0 {
		return locales.QuotasDurMin(0)
	}
	if d >= 48*time.Hour {
		days := int(math.Round(d.Hours() / 24))
		return locales.QuotasDurDays(days)
	}
	totalMins := int(math.Round(d.Minutes()))
	h := totalMins / 60
	m := totalMins % 60
	if h == 0 {
		return locales.QuotasDurMin(max(m, 1))
	}
	if m == 0 {
		return locales.QuotasDurHours(h)
	}
	return locales.QuotasDurHoursMin(h, m)
}

// formatQuotaResetAt formats a future reset timestamp as local "HH:MM" when
// within 24 hours, or as a compact localized month-day date ("Dec 2" / "2 дек.")
// when 24 hours or further away.
func formatQuotaResetAt(resetsAt, now time.Time) string {
	if resetsAt.IsZero() || !resetsAt.After(now) {
		return ""
	}
	local := resetsAt.Local()
	if resetsAt.Sub(now) < 24*time.Hour {
		return local.Format("15:04")
	}
	return locales.AgoOn(local)
}

func quotaUpdatedText(c *ui.Context, at time.Time) string {
	ago := c.Now().Sub(at)
	if ago < time.Minute {
		return locales.QuotasUpdatedJustNow()
	}
	if ago < time.Hour {
		return locales.QuotasUpdatedMinAgo(int(ago / time.Minute))
	}
	return locales.QuotasUpdatedHoursAgo(int(ago / time.Hour))
}

func quotaUpdatedShortText(c *ui.Context, at time.Time) string {
	ago := c.Now().Sub(at)
	if ago < time.Minute {
		return locales.QuotasUpdatedShortJustNow()
	}
	if ago < time.Hour {
		return locales.QuotasUpdatedShortMinAgo(int(ago / time.Minute))
	}
	return locales.QuotasUpdatedShortHoursAgo(int(ago / time.Hour))
}
