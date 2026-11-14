package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// usagePage is "Usage": the tokens the AI agents' sessions used and what
// they would cost at the API's prices.
const usagePage = "/ai/usage"

// usagePeriods are the Usage page's periods, in days, a tab each; 0 is
// all time.
var usagePeriods = []int{7, 30, 90, 0}

// usageAgents are the agents whose history Hearth reads, with the color
// of their bars.
var usageAgents = []usageAgent{{"claude", ui.Hex("#D97757")}, {"codex", ui.Hex("#A8A296")}}

type usageAgent struct {
	id    string
	color ui.Color
}

// usageTotal is tokens and their cost; unpriced reports that some are of
// a model Hearth has no price of, left out of the cost.
type usageTotal struct {
	cost     float64
	tokens   int64
	unpriced bool
}

func (t *usageTotal) add(u inventory.TokenUsage) {
	t.tokens += u.Tokens()
	if c, ok := ai.Cost(u); ok {
		t.cost += c
	} else {
		t.unpriced = true
	}
}

// usageGroup is the usage of a project, or of a model: its sessions, the
// agents they are of and the last day of one.
type usageGroup struct {
	// key is the project's folder, "" for sessions outside projects; or
	// the model.
	key      string
	agents   []string
	sessions int
	last     string
	usageTotal
}

// usageBar is a bar of the chart: a day, or a month for all time, and the
// cost of each of usageAgents.
type usageBar struct {
	at   time.Time
	cost []float64
}

// usageStore is an agent's history on disk.
type usageStore struct {
	agent    string
	sessions int
	size     int64
}

// usageSummary is the agents' usage in a period.
type usageSummary struct {
	today, total       usageTotal
	sessions, projects int
	bars               []usageBar
	monthly            bool
	byProject, byModel []usageGroup
	stores             []usageStore
	// old is what the sessions Cleanup offers take.
	old int64
}

// usageSummary sums the sessions' usage of the last days to now, all of
// it for 0 days.
func (a *App) usageSummary(now time.Time, days int) usageSummary {
	var s usageSummary
	var sessions []inventory.AgentSession
	if a.Snapshot != nil {
		sessions = a.Snapshot.Sessions
	}
	today := now.Format(time.DateOnly)
	from := ""
	if days > 0 {
		from = now.AddDate(0, 0, 1-days).Format(time.DateOnly)
	}
	first := today
	bars := map[string][]float64{}
	group := func(gs *[]usageGroup, key string) *usageGroup {
		i := slices.IndexFunc(*gs, func(g usageGroup) bool { return g.key == key })
		if i < 0 {
			*gs = append(*gs, usageGroup{key: key})
			i = len(*gs) - 1
		}
		return &(*gs)[i]
	}
	for _, ss := range sessions {
		if i := slices.IndexFunc(s.stores, func(x usageStore) bool { return x.agent == ss.Agent }); i >= 0 {
			s.stores[i].sessions++
			s.stores[i].size += ss.Size
		} else {
			s.stores = append(s.stores, usageStore{ss.Agent, 1, ss.Size})
		}
		if now.Sub(ss.Modified) >= cleanup.HistoryAfter {
			s.old += ss.Size
		}
		agent := slices.IndexFunc(usageAgents, func(x usageAgent) bool { return x.id == ss.Agent })
		project := a.usageProject(ss.Dir)
		counted := map[string]bool{}
		for _, u := range ss.Usage {
			if u.Day < from {
				continue
			}
			first = min(first, u.Day)
			if u.Day == today {
				s.today.add(u)
			}
			s.total.add(u)
			key := u.Day
			if days == 0 {
				key = u.Day[:7]
			}
			if bars[key] == nil {
				bars[key] = make([]float64, len(usageAgents))
			}
			if c, ok := ai.Cost(u); ok && agent >= 0 {
				bars[key][agent] += c
			}
			p := group(&s.byProject, project)
			p.add(u)
			p.last = max(p.last, u.Day)
			m := group(&s.byModel, u.Model)
			m.add(u)
			if !counted[project] {
				counted[project] = true
				p.sessions++
				if !slices.Contains(p.agents, ss.Agent) {
					p.agents = append(p.agents, ss.Agent)
				}
			}
			if !counted["model:"+u.Model] {
				counted["model:"+u.Model] = true
				m.sessions++
				if !slices.Contains(m.agents, ss.Agent) {
					m.agents = append(m.agents, ss.Agent)
				}
			}
		}
		if len(counted) > 0 {
			s.sessions++
		}
	}
	s.projects = len(s.byProject)
	byCost := func(x, y usageGroup) int {
		return cmp.Or(cmp.Compare(y.cost, x.cost), cmp.Compare(y.tokens, x.tokens), cmp.Compare(x.key, y.key))
	}
	slices.SortFunc(s.byProject, byCost)
	slices.SortFunc(s.byModel, byCost)
	// A bar each day of the period, or each month since the first use.
	if days > 0 {
		for d := range days {
			at := now.AddDate(0, 0, d+1-days)
			s.bars = append(s.bars, usageBar{at, bars[at.Format(time.DateOnly)]})
		}
		return s
	}
	s.monthly = true
	start, _ := time.ParseInLocation(time.DateOnly, first, now.Location())
	// A year at least, so that a few months do not make wide bars.
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, now.Location())
	if yearAgo := thisMonth.AddDate(0, -11, 0); yearAgo.Before(start) {
		start = yearAgo
	}
	for m := start; !m.After(now); m = m.AddDate(0, 1, 0) {
		s.bars = append(s.bars, usageBar{m, bars[m.Format("2006-01")]})
	}
	return s
}

// usageProject is the project a session in dir ran in: the innermost
// known project dir is in, else dir itself; "" for none or the home.
func (a *App) usageProject(dir string) string {
	if dir == "" || dir == a.Home {
		return ""
	}
	found := ""
	for _, p := range a.Projects {
		if within(filepath.ToSlash(dir), filepath.ToSlash(p.Dir)) && len(p.Dir) > len(found) {
			found = p.Dir
		}
	}
	return cmp.Or(found, dir)
}

// usageCost says what a total costs, about: "≈ $4.80", "< $0.01", or "—"
// without a price.
func usageCost(t usageTotal) string {
	switch {
	case t.cost == 0 && t.unpriced:
		return "—"
	case t.cost > 0 && t.cost < 0.005:
		return locales.UsageCostSmall()
	}
	return locales.UsageCost(t.cost)
}

// usageTokens says a count of tokens briefly: "1.2M tokens".
func usageTokens(n int64) string {
	switch {
	case n >= 1e9:
		return locales.UsageTokensB(float64(n) / 1e9)
	case n >= 1e6:
		return locales.UsageTokensM(float64(n) / 1e6)
	case n >= 1e3:
		return locales.UsageTokensK(float64(n) / 1e3)
	}
	return locales.UsageTokens(int(n))
}

func (a *App) usageView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		days := usagePeriods[a.usageTab]
		s := a.usageSummary(c.Now(), days)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("chart-column")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.UsageLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					period := locales.UsageAllTime()
					if days > 0 {
						period = locales.UsagePeriod(days)
					}
					stateCard(c, pal.TextFaint, locales.UsageToday(), usageCost(s.today), usageTokens(s.today.tokens))
					stateCard(c, t.Success, period, usageCost(s.total), usageTokens(s.total.tokens))
					stateCard(c, pal.Info, locales.UsageSessions(), strconv.Itoa(s.sessions), locales.UsageFolders(s.projects))
					var size int64
					for _, st := range s.stores {
						size += st.size
					}
					stateCard(c, t.Warning, locales.UsageOnDisk(), formatBytes(size), locales.UsageOld(formatBytes(s.old)))
				})
				ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
					segs := make([]segment, len(usagePeriods))
					for i, d := range usagePeriods {
						segs[i] = segment{locales.UsageDays(d), ""}
						if d == 0 {
							segs[i].label = locales.UsageAllTime()
						}
					}
					segments(c, "usage-period", &a.usageTab, segs)
					ui.Box(c).Grow(1).Height(1)
					month, _ := time.Parse("2006-01", ai.PricesAsOf)
					ui.Text(c, locales.UsageNote(month)).FontSize(12).TextColor(pal.TextFaint)
				})
				columns(c, 12, func() {
					switch {
					case a.Snapshot == nil:
						toolNote(c, locales.Checking())
						return
					case len(s.stores) == 0:
						listCard(c, locales.UsageByProject(), "", func() { cardNote(c, locales.UsageEmpty()) })
						return
					}
					a.usageChart(c, s)
					listCard(c, locales.UsageByProject(), "", func() {
						for _, g := range s.byProject {
							a.usageProjectRow(c, g)
						}
						if len(s.byProject) == 0 {
							cardNote(c, locales.UsageNone())
						}
					})
				}, func() {
					if len(s.byModel) > 0 {
						listCard(c, locales.UsageByModel(), "", func() {
							for _, m := range s.byModel {
								caption := usageTokens(m.tokens) + " · " + usageAgentNames(m.agents)
								if m.unpriced {
									caption += " · " + locales.UsagePriceUnknown()
								}
								listRow(c, m.key, false, caption, func() {
									ui.Text(c, usageCost(m.usageTotal)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
								})
							}
						})
					}
					if len(s.stores) > 0 {
						a.usageStores(c, s)
					}
				})
			})
		})
	})
}

// usageAgentNames names agents: "Claude Code, Codex".
func usageAgentNames(ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = agentTitle(id)
	}
	return strings.Join(names, ", ")
}

// usageChart is the period's cost by day, or by month, a bar of each
// agent's cost on another's.
func (a *App) usageChart(c *ui.Context, s usageSummary) {
	t := c.Theme()
	pal := Colors(t)
	title := locales.UsageByDay()
	if s.monthly {
		title = locales.UsageByMonth()
	}
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			for _, ag := range usageAgents {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(ag.color)
					ui.Text(c, agentTitle(ag.id)).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
		labels := make([]string, len(s.bars))
		series := make([][]float64, len(usageAgents))
		hi := 0.0
		for i, b := range s.bars {
			labels[i] = locales.AgoOn(b.at)
			if s.monthly {
				labels[i] = locales.UsageMonth(b.at)
			}
			sum := 0.0
			for j := range usageAgents {
				v := 0.0
				if b.cost != nil {
					v = b.cost[j]
				}
				series[j] = append(series[j], v)
				sum += v
			}
			hi = max(hi, sum)
		}
		x := chart.NewBand(labels...).Padding(0.3, 0.1)
		y := chart.NewLinear(0, max(hi, 1)).Nice(4)
		// Five dates along the bottom, the last day's among them.
		var ticks []chart.Tick
		all := x.Ticks()
		step := max((len(all)+3)/5, 1)
		for i := len(all) - 1; i >= 0; i -= step {
			ticks = append(ticks, all[i])
		}
		label := func(_ chart.Side, tick chart.Tick) {
			ui.Text(c, tick.Label).NoWrap().FontSize(10.5).TextColor(pal.TextFaint)
		}
		stacked := chart.Stack(series, chart.StackStandard)
		chart.Plot(c, chart.Axes{Bottom: ticks, Label: label}, func(area ui.Element) {
			for j, seg := range stacked {
				for i, v := range seg {
					if v.Hi <= v.Lo {
						continue
					}
					tip := labels[i] + " · " + agentTitle(usageAgents[j].id) + " · " + locales.UsageCost(v.Hi-v.Lo)
					chart.Rect(c, x.Band(i), y.Span(v.Lo, v.Hi)).Background(usageAgents[j].color).Tooltip(tip).Label(tip)
				}
			}
		}).Label(title).Height(150).Gap(6).Margin(4, 16, 12, 16)
	})
}

// usageProjectRow is a project's row: its name and folder, sessions,
// tokens and last day, its agents and cost.
func (a *App) usageProjectRow(c *ui.Context, g usageGroup) {
	t := c.Theme()
	pal := Colors(t)
	name, caption := locales.UsageOutside(), locales.UsageOutsideCaption()
	if g.key != "" {
		name, caption = filepath.Base(g.key), tilde(g.key, a.Home)
	}
	last, _ := time.ParseInLocation(time.DateOnly, g.last, time.Local)
	caption += " · " + locales.UsageSessionCount(g.sessions) + " · " + usageTokens(g.tokens) + " · " + locales.AgoOn(last)
	ui.Row(c.Key("usage:"+g.key)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, name).FontSize(13)
			clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
		})
		for _, ag := range usageAgents {
			if slices.Contains(g.agents, ag.id) {
				chip(c, ag.color, pal.Muted, t.TextMuted, agentTitle(ag.id))
			}
		}
		ui.Text(c, usageCost(g.usageTotal)).Font(FontMono).FontSize(12).Width(76).Shrink(0).TextAlign(ui.End).SingleLine()
	})
}

// usageStores says where each agent keeps its history and what it takes,
// and what of it Cleanup offers.
func (a *App) usageStores(c *ui.Context, s usageSummary) {
	listCard(c, locales.UsageWhere(), "", func() {
		for _, st := range s.stores {
			caption := locales.UsageSessionCount(st.sessions)
			if ag, ok := a.agent(st.agent); ok && ag.Dir != "" {
				caption = tilde(ag.Dir, a.Home) + " · " + caption
			}
			listRow(c, agentTitle(st.agent), false, caption, func() {
				ui.Text(c, formatBytes(st.size)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
			})
		}
		if s.old == 0 {
			return
		}
		days := int(cleanup.HistoryAfter.Hours() / 24)
		listRow(c, locales.UsageOldRow(days), false, locales.UsageOldCaption(), func() {
			ui.Text(c, formatBytes(s.old)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
			if iconButton(c, "arrow-right", locales.UsageOpenCleanup()).Clicked() {
				a.cleanupTab = slices.Index(cleanupGroups, cleanup.History) + 1
				a.router.Push(cleanupPage)
			}
		})
	})
}

// agent is the agent id the last scan found, or knows of.
func (a *App) agent(id string) (inventory.Agent, bool) {
	for _, ag := range a.agents() {
		if ag.ID == id {
			return ag, true
		}
	}
	return inventory.Agent{}, false
}
