package ui

import (
	"cmp"
	"fmt"
	"math"
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

// usageProjectOutside is the filter sentinel for sessions outside known projects.
const usageProjectOutside = "__outside__"

// usagePeriods are the Usage page's periods, in days, a tab each; 0 is
// all time.
var usagePeriods = []int{7, 30, 90, 0}

// usageAgents are the agents whose local token history Hearth reads, with
// the color of their chart bars and chips.
var usageAgents = []usageAgent{
	{"claude", ui.Hex("#D97757")},
	{"codex", ui.Hex("#A8A296")},
	{"gemini", ui.Hex("#4E8DF5")},
	{"copilot", ui.Hex("#8B6CE8")},
	{"kimi", ui.Hex("#2F9E84")},
	{"grok", ui.Hex("#D99B38")},
	{"opencode", ui.Hex("#E06C9F")},
	{"devin", ui.Hex("#57A8C9")},
	{"antigravity", ui.Hex("#73A950")},
}

type usageAgent struct {
	id    string
	color ui.Color
}

func isSQLiteUsageAgent(id string) bool {
	switch id {
	case "opencode", "devin", "antigravity":
		return true
	}
	return false
}

// usageTotal is tokens and their cost; unpriced reports that some are of
// a model Hearth has no price of, left out of the cost.
type usageTotal struct {
	cost       float64
	tokens     int64
	input      int64
	output     int64
	cacheRead  int64
	cacheWrite int64
	unpriced   bool
}

func (t *usageTotal) add(u inventory.TokenUsage) {
	t.tokens += u.Tokens()
	t.input += u.Input
	t.output += u.Output
	t.cacheRead += u.CacheRead
	t.cacheWrite += u.CacheWrite + u.CacheWrite1h
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
// cost and tokens of each of usageAgents.
type usageBar struct {
	at     time.Time
	cost   []float64
	tokens []int64
}

// usageStore is an agent's history on disk.
type usageStore struct {
	agent    string
	dir      string
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
	// availableAgents, availableProjects, and availableModels list all values
	// present in the snapshot so filters stay populated and stale filters reset.
	availableAgents   []string
	availableProjects []string
	availableModels   []string
}

type usageSummaryKey struct {
	snapshot      *inventory.Snapshot
	sessions      int
	home          string
	year          int
	month         time.Month
	day           int
	loc           *time.Location
	minute        int64
	days          int
	agentFilter   string
	projectFilter string
	modelFilter   string
	byTokens      bool
}

type usageSummaryCacheEntry struct {
	key         usageSummaryKey
	projectDirs []string
	summary     usageSummary
}

const maxUsageSummaryCache = 8

func (a *App) sameProjectDirs(dirs []string) bool {
	if len(dirs) != len(a.Projects) {
		return false
	}
	for i, d := range dirs {
		if a.Projects[i].Dir != d {
			return false
		}
	}
	return true
}

func (a *App) snapshotProjectDirs() []string {
	if len(a.Projects) == 0 {
		return nil
	}
	dirs := make([]string, len(a.Projects))
	for i, p := range a.Projects {
		dirs[i] = p.Dir
	}
	return dirs
}

// usageSummary sums the sessions' usage of the last days to now, all of
// it for 0 days, without agent or project filters.
func (a *App) usageSummary(now time.Time, days int) usageSummary {
	return a.usageSummaryFor(now, days, "", "", "", false)
}

// usageSummaryFiltered sums usage with optional agent and project filters.
// When byTokens is true, project and model rows are sorted by tokens first.
func (a *App) usageSummaryFiltered(now time.Time, days int, agentFilter, projectFilter string, byTokens bool) usageSummary {
	return a.usageSummaryFor(now, days, agentFilter, projectFilter, "", byTokens)
}

// usageSummaryFor sums usage with optional agent, project, and model filters.
func (a *App) usageSummaryFor(now time.Time, days int, agentFilter, projectFilter, modelFilter string, byTokens bool) usageSummary {
	var sessions []inventory.AgentSession
	if a.Snapshot != nil {
		sessions = a.Snapshot.Sessions
	}
	if len(a.usageCache) > 0 && a.usageCache[0].key.snapshot != a.Snapshot {
		a.usageCache = nil
	}
	y, m, d := now.Date()
	key := usageSummaryKey{
		snapshot:      a.Snapshot,
		sessions:      len(sessions),
		home:          a.Home,
		year:          y,
		month:         m,
		day:           d,
		loc:           now.Location(),
		minute:        now.Truncate(time.Minute).Unix(),
		days:          days,
		agentFilter:   agentFilter,
		projectFilter: projectFilter,
		modelFilter:   modelFilter,
		byTokens:      byTokens,
	}
	for i := range a.usageCache {
		if a.usageCache[i].key == key && a.sameProjectDirs(a.usageCache[i].projectDirs) {
			return a.usageCache[i].summary
		}
	}
	s := a.computeUsageSummary(now, sessions, days, agentFilter, projectFilter, modelFilter, byTokens)
	if len(a.usageCache) >= maxUsageSummaryCache {
		copy(a.usageCache, a.usageCache[1:])
		a.usageCache[len(a.usageCache)-1] = usageSummaryCacheEntry{
			key:         key,
			projectDirs: a.snapshotProjectDirs(),
			summary:     s,
		}
	} else {
		a.usageCache = append(a.usageCache, usageSummaryCacheEntry{
			key:         key,
			projectDirs: a.snapshotProjectDirs(),
			summary:     s,
		})
	}
	return s
}

func (a *App) computeUsageSummary(now time.Time, sessions []inventory.AgentSession, days int, agentFilter, projectFilter, modelFilter string, byTokens bool) usageSummary {
	var s usageSummary
	today := now.Format(time.DateOnly)
	from := ""
	if days > 0 {
		from = now.AddDate(0, 0, 1-days).Format(time.DateOnly)
	}
	first := today
	costBars := map[string][]float64{}
	tokenBars := map[string][]int64{}
	group := func(gs *[]usageGroup, key string) *usageGroup {
		i := slices.IndexFunc(*gs, func(g usageGroup) bool { return g.key == key })
		if i < 0 {
			*gs = append(*gs, usageGroup{key: key})
			i = len(*gs) - 1
		}
		return &(*gs)[i]
	}
	seenAgents := map[string]bool{}
	seenProjects := map[string]bool{}
	seenModels := map[string]bool{}
	hasOutside := false

	for _, ss := range sessions {
		storeDir := ss.StoreDir
		if storeDir == "" && isSQLiteUsageAgent(ss.Agent) && len(ss.Files) > 0 {
			storeDir = filepath.Dir(ss.Files[0])
		}
		if i := slices.IndexFunc(s.stores, func(x usageStore) bool { return x.agent == ss.Agent }); i >= 0 {
			s.stores[i].sessions++
			s.stores[i].size += ss.Size
			if s.stores[i].dir == "" && storeDir != "" {
				s.stores[i].dir = storeDir
			}
		} else {
			s.stores = append(s.stores, usageStore{agent: ss.Agent, dir: storeDir, sessions: 1, size: ss.Size})
		}
		if now.Sub(ss.Modified) >= cleanup.HistoryAfter && (len(ss.Files) > 0 || !isSQLiteUsageAgent(ss.Agent)) {
			s.old += ss.Size
		}
		if !seenAgents[ss.Agent] {
			seenAgents[ss.Agent] = true
			s.availableAgents = append(s.availableAgents, ss.Agent)
		}
		project := a.usageProjectDir(ss.Dir)
		if project == "" {
			hasOutside = true
		} else if !seenProjects[project] {
			seenProjects[project] = true
			s.availableProjects = append(s.availableProjects, project)
		}
		for _, u := range ss.Usage {
			if u.Model != "" && !seenModels[u.Model] {
				seenModels[u.Model] = true
				s.availableModels = append(s.availableModels, u.Model)
			}
		}

		if agentFilter != "" && ss.Agent != agentFilter {
			continue
		}
		if projectFilter != "" {
			if projectFilter == usageProjectOutside {
				if project != "" {
					continue
				}
			} else if project != projectFilter {
				continue
			}
		}

		agentIdx := slices.IndexFunc(usageAgents, func(x usageAgent) bool { return x.id == ss.Agent })
		counted := map[string]bool{}
		for _, u := range ss.Usage {
			if u.Day < from {
				continue
			}
			if modelFilter != "" && u.Model != modelFilter {
				continue
			}
			first = min(first, u.Day)
			if u.Day == today {
				s.today.add(u)
			}
			s.total.add(u)
			key := u.Day
			if days == 0 && len(u.Day) >= 7 {
				key = u.Day[:7]
			}
			if costBars[key] == nil {
				costBars[key] = make([]float64, len(usageAgents))
				tokenBars[key] = make([]int64, len(usageAgents))
			}
			if agentIdx >= 0 {
				tokenBars[key][agentIdx] += u.Tokens()
				if c, ok := ai.Cost(u); ok {
					costBars[key][agentIdx] += c
				}
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

	// Keep availableAgents in Hearth's canonical order.
	slices.SortFunc(s.availableAgents, func(x, y string) int {
		ix := slices.IndexFunc(usageAgents, func(u usageAgent) bool { return u.id == x })
		iy := slices.IndexFunc(usageAgents, func(u usageAgent) bool { return u.id == y })
		if ix < 0 {
			ix = len(usageAgents)
		}
		if iy < 0 {
			iy = len(usageAgents)
		}
		return cmp.Or(cmp.Compare(ix, iy), cmp.Compare(x, y))
	})
	slices.SortFunc(s.availableProjects, func(x, y string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(filepath.Base(x)), strings.ToLower(filepath.Base(y))), cmp.Compare(x, y))
	})
	if hasOutside {
		s.availableProjects = append(s.availableProjects, usageProjectOutside)
	}

	s.projects = len(s.byProject)
	sortGroup := func(x, y usageGroup) int {
		if byTokens {
			return cmp.Or(cmp.Compare(y.tokens, x.tokens), cmp.Compare(y.cost, x.cost), cmp.Compare(x.key, y.key))
		}
		return cmp.Or(cmp.Compare(y.cost, x.cost), cmp.Compare(y.tokens, x.tokens), cmp.Compare(x.key, y.key))
	}
	slices.SortFunc(s.byProject, sortGroup)
	slices.SortFunc(s.byModel, sortGroup)
	// A bar each day of the period, or each month since the first use.
	if days > 0 {
		for d := range days {
			at := now.AddDate(0, 0, d+1-days)
			k := at.Format(time.DateOnly)
			s.bars = append(s.bars, usageBar{at: at, cost: costBars[k], tokens: tokenBars[k]})
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
		k := m.Format("2006-01")
		s.bars = append(s.bars, usageBar{at: m, cost: costBars[k], tokens: tokenBars[k]})
	}
	return s
}

// usageProjectDir is the project a session in dir ran in: the innermost
// known project dir is in, else dir itself; "" for none or the home.
func (a *App) usageProjectDir(dir string) string {
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

// unsupportedUsageAgents returns display names of installed agents that do
// not save local token counts (such as Cursor and Aider).
func (a *App) unsupportedUsageAgents() []string {
	var out []string
	for _, ag := range a.agents() {
		installed := len(ag.Installations) > 0 || ag.App != ""
		if installed && !ai.UsageSupport(ag.ID) {
			out = append(out, agentTitle(ag.ID))
		}
	}
	return out
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

// usageCompactTokens formats a token count compactly without the "tokens" word:
// "0", "450", "12.5K", "2M".
func usageCompactTokens(n int64) string {
	format := func(v float64, unit string) string {
		s := fmt.Sprintf("%.1f", v)
		s = strings.TrimSuffix(s, ".0")
		return s + unit
	}
	switch {
	case n >= 1e9:
		return format(float64(n)/1e9, "B")
	case n >= 1e6:
		return format(float64(n)/1e6, "M")
	case n >= 1e3:
		return format(float64(n)/1e3, "K")
	}
	return strconv.FormatInt(n, 10)
}

// usageAxisCost formats a Y-axis cost tick compactly: "$0", "$2.50", "$10".
func usageAxisCost(v float64) string {
	switch {
	case v <= 0:
		return "$0"
	case v < 0.01:
		return "<$0.01"
	case math.Abs(v-math.Round(v)) <= 1e-6:
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func (a *App) usageView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		days := usagePeriods[a.usageTab]
		byTokens := a.usageMetric == 1
		s := a.usageSummaryFor(c.Now(), days, a.usageAgent, a.usageProject, a.usageModel, byTokens)
		// Reset stale filters if the backing snapshot no longer contains them.
		if a.usageAgent != "" && !slices.Contains(s.availableAgents, a.usageAgent) {
			a.usageAgent = ""
			s = a.usageSummaryFor(c.Now(), days, a.usageAgent, a.usageProject, a.usageModel, byTokens)
		}
		if a.usageProject != "" && !slices.Contains(s.availableProjects, a.usageProject) {
			a.usageProject = ""
			s = a.usageSummaryFor(c.Now(), days, a.usageAgent, a.usageProject, a.usageModel, byTokens)
		}
		if a.usageModel != "" && !slices.Contains(s.availableModels, a.usageModel) {
			a.usageModel = ""
			s = a.usageSummaryFor(c.Now(), days, a.usageAgent, a.usageProject, a.usageModel, byTokens)
		}
		unsupported := a.unsupportedUsageAgents()

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
					todayMain, todaySub := usageCost(s.today), usageTokens(s.today.tokens)
					totalMain, totalSub := usageCost(s.total), usageTokens(s.total.tokens)
					if byTokens {
						todayMain, todaySub = todaySub, todayMain
						totalMain, totalSub = totalSub, totalMain
					}
					stateCard(c, pal.TextFaint, locales.UsageToday(), todayMain, todaySub)
					stateCard(c, t.Success, period, totalMain, totalSub)
					stateCard(c, pal.Info, locales.UsageSessions(), strconv.Itoa(s.sessions), locales.UsageFolders(s.projects))
					var size int64
					for _, st := range s.stores {
						size += st.size
					}
					stateCard(c, t.Warning, locales.UsageOnDisk(), formatBytes(size), locales.UsageOld(formatBytes(s.old)))
				})
				ui.Column(c).Gap(8).Children(func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
						segs := make([]segment, len(usagePeriods))
						for i, d := range usagePeriods {
							segs[i] = segment{locales.UsageDays(d), ""}
							if d == 0 {
								segs[i].label = locales.UsageAllTime()
							}
						}
						segments(c, "usage-period", &a.usageTab, segs)
						segments(c, "usage-metric", &a.usageMetric, []segment{
							{locales.UsageMetricCost(), ""},
							{locales.UsageMetricTokens(), ""},
						})
						if len(s.stores) > 0 {
							a.usageFilterSelects(c, s)
						}
					})
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
					a.usageChart(c, s, byTokens)
					listCard(c, locales.UsageByProject(), "", func() {
						for _, g := range s.byProject {
							a.usageProjectRow(c, g, byTokens)
						}
						if len(s.byProject) == 0 {
							cardNote(c, locales.UsageNone())
						}
					})
				}, func() {
					if len(s.byModel) > 0 {
						listCard(c, locales.UsageByModel(), "", func() {
							for _, m := range s.byModel {
								a.usageModelRow(c, m, byTokens)
							}
						})
					}
					if len(s.stores) > 0 || len(unsupported) > 0 {
						a.usageStores(c, s, unsupported)
					}
				})
			})
		})
	})
}

// usageFilterSelects renders the Agent and Project filter dropdowns and the
// active model filter chip when a model is selected.
func (a *App) usageFilterSelects(c *ui.Context, s usageSummary) {
	t := c.Theme()
	pal := Colors(t)
	agentValues := make([]string, 0, len(s.availableAgents)+1)
	agentLabels := make([]string, 0, len(s.availableAgents)+1)
	agentValues = append(agentValues, "")
	agentLabels = append(agentLabels, locales.UsageAllAgents())
	for _, id := range s.availableAgents {
		agentValues = append(agentValues, id)
		agentLabels = append(agentLabels, agentTitle(id))
	}
	if i := slices.Index(agentValues, a.usageAgent); i >= 0 {
		a.usageAgentLabel = agentLabels[i]
	} else {
		a.usageAgent = ""
		a.usageAgentLabel = agentLabels[0]
	}
	ui.Select(c.Key("usage-filter-agent"), &a.usageAgentLabel, agentLabels).
		Label(locales.UsageFilterAgent()).Width(156).OnChange(func() {
		if i := slices.Index(agentLabels, a.usageAgentLabel); i >= 0 {
			a.usageAgent = agentValues[i]
		}
	})

	baseCount := map[string]int{}
	for _, p := range s.availableProjects {
		if p != usageProjectOutside {
			baseCount[filepath.Base(p)]++
		}
	}
	projValues := make([]string, 0, len(s.availableProjects)+1)
	projLabels := make([]string, 0, len(s.availableProjects)+1)
	projValues = append(projValues, "")
	projLabels = append(projLabels, locales.UsageAllProjects())
	for _, p := range s.availableProjects {
		projValues = append(projValues, p)
		if p == usageProjectOutside {
			projLabels = append(projLabels, locales.UsageOutside())
		} else if baseCount[filepath.Base(p)] > 1 {
			projLabels = append(projLabels, tilde(p, a.Home))
		} else {
			projLabels = append(projLabels, filepath.Base(p))
		}
	}
	if i := slices.Index(projValues, a.usageProject); i >= 0 {
		a.usageProjectLabel = projLabels[i]
	} else {
		a.usageProject = ""
		a.usageProjectLabel = projLabels[0]
	}
	ui.Select(c.Key("usage-filter-project"), &a.usageProjectLabel, projLabels).
		Label(locales.UsageFilterProject()).Width(176).OnChange(func() {
		if i := slices.Index(projLabels, a.usageProjectLabel); i >= 0 {
			a.usageProject = projValues[i]
		}
	})
	if a.usageModel != "" {
		clear := ui.ButtonBase(c.Key("usage-filter-model-clear")).Height(controlHeight).Padding(0, 10).Radius(7).Gap(6).
			AlignItems(ui.Center).Background(pal.ActiveRow).BorderWidth(1).BorderColor(pal.Line).
			Label(locales.UsageClearModel()).Tooltip(locales.UsageClearModel())
		clear.Children(func() {
			ui.Text(c, a.usageModel).Font(FontMono).FontSize(11.5).SingleLine()
			ui.Icon(c, Icon("x")).Size(12, 12).TextColor(t.TextMuted)
		})
		if clear.Clicked() {
			a.usageModel = ""
		}
	}
}

// usageAgentNames names agents: "Claude Code, Codex".
func usageAgentNames(ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = agentTitle(id)
	}
	return strings.Join(names, ", ")
}

// usageChart is the period's cost or tokens by day, or by month, a bar of
// each agent's contribution stacked on another's, with a Y-axis scale and
// token breakdown strip underneath.
func (a *App) usageChart(c *ui.Context, s usageSummary, byTokens bool) {
	t := c.Theme()
	pal := Colors(t)
	title := locales.UsageByDay()
	if s.monthly {
		title = locales.UsageByMonth()
	}
	labels := make([]string, len(s.bars))
	series := make([][]float64, len(usageAgents))
	active := make([]bool, len(usageAgents))
	hi := 0.0
	for i, b := range s.bars {
		labels[i] = locales.AgoOn(b.at)
		if s.monthly {
			labels[i] = locales.UsageMonth(b.at)
		}
		sum := 0.0
		for j := range usageAgents {
			v := 0.0
			if byTokens {
				if b.tokens != nil {
					v = float64(b.tokens[j])
				}
			} else if b.cost != nil {
				v = b.cost[j]
			}
			if v > 0 {
				active[j] = true
			}
			series[j] = append(series[j], v)
			sum += v
		}
		hi = max(hi, sum)
	}

	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(12, 14, 10, 14).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			for j, ag := range usageAgents {
				if !active[j] {
					continue
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(ag.color)
					ui.Text(c, agentTitle(ag.id)).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
		x := chart.NewBand(labels...).Padding(0.3, 0.1)
		y := chart.NewLinear(0, max(hi, 1)).Nice(4)
		yTicks := y.Ticks(4)
		// Five dates along the bottom, the last day's among them.
		var ticks []chart.Tick
		all := x.Ticks()
		step := max((len(all)+3)/5, 1)
		for i := len(all) - 1; i >= 0; i -= step {
			ticks = append(ticks, all[i])
		}
		label := func(side chart.Side, tick chart.Tick) {
			if side == chart.Left {
				text := usageAxisCost(tick.Value)
				if byTokens {
					text = usageCompactTokens(int64(math.Round(tick.Value)))
				}
				ui.Text(c, text).NoWrap().Font(FontMono).FontSize(10).TextColor(pal.TextFaint)
				return
			}
			ui.Text(c, tick.Label).NoWrap().FontSize(10.5).TextColor(pal.TextFaint)
		}
		stacked := chart.Stack(series, chart.StackStandard)
		chart.Plot(c, chart.Axes{Left: yTicks, Bottom: ticks, Label: label}, func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range yTicks {
					yy := chart.Y(r, tick.At)
					p.Line(r.X, yy, r.X+r.W, yy, 1, pal.Line)
				}
			})
			for j, seg := range stacked {
				for i, v := range seg {
					if v.Hi <= v.Lo {
						continue
					}
					val := locales.UsageCost(v.Hi - v.Lo)
					if byTokens {
						val = usageTokens(int64(v.Hi - v.Lo))
					}
					tip := labels[i] + " · " + agentTitle(usageAgents[j].id) + " · " + val
					chart.Rect(c, x.Band(i), y.Span(v.Lo, v.Hi)).Background(usageAgents[j].color).Tooltip(tip).Label(tip)
				}
			}
		}).Label(title).Height(160).Gap(6).Margin(4, 16, 10, 12)
		ui.Row(c).Gap(16).Padding(8, 14).AlignItems(ui.Center).Wrap().BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			for _, item := range []struct {
				label string
				count int64
			}{
				{locales.UsageTokenInput(), s.total.input},
				{locales.UsageTokenOutput(), s.total.output},
				{locales.UsageTokenCacheRead(), s.total.cacheRead},
				{locales.UsageTokenCacheWrite(), s.total.cacheWrite},
			} {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.label).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					ui.Text(c, usageCompactTokens(item.count)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
	})
}

// usageProjectRow is a clickable project row: clicking filters the page by
// that project, clicking again clears the filter.
func (a *App) usageProjectRow(c *ui.Context, g usageGroup, byTokens bool) {
	t := c.Theme()
	pal := Colors(t)
	name, caption := locales.UsageOutside(), locales.UsageOutsideCaption()
	filterVal := usageProjectOutside
	if g.key != "" {
		name, caption = filepath.Base(g.key), tilde(g.key, a.Home)
		filterVal = g.key
	}
	primary := usageCost(g.usageTotal)
	secondary := usageTokens(g.tokens)
	if byTokens {
		primary, secondary = secondary, primary
	}
	last, _ := time.ParseInLocation(time.DateOnly, g.last, time.Local)
	caption += " · " + locales.UsageSessionCount(g.sessions) + " · " + secondary + " · " + locales.AgoOn(last)
	selected := a.usageProject == filterVal
	row := ui.ButtonBase(c.Key("usage:"+g.key)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(name)
	if selected {
		row.Background(pal.ActiveRow)
	} else if row.Hovered() {
		row.Background(pal.Muted)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, name).FontSize(13)
			clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
		})
		for _, ag := range usageAgents {
			if slices.Contains(g.agents, ag.id) {
				chip(c, ag.color, pal.Muted, t.TextMuted, agentTitle(ag.id))
			}
		}
		ui.Text(c, primary).Font(FontMono).FontSize(12).MinWidth(76).Shrink(0).TextAlign(ui.End).SingleLine()
	})
	if row.Clicked() {
		if selected {
			a.usageProject = ""
		} else {
			a.usageProject = filterVal
		}
	}
}

// usageModelRow is a clickable model row: clicking filters the page by that
// model, clicking again clears the filter.
func (a *App) usageModelRow(c *ui.Context, m usageGroup, byTokens bool) {
	t := c.Theme()
	pal := Colors(t)
	primary := usageCost(m.usageTotal)
	secondary := usageTokens(m.tokens)
	if byTokens {
		primary, secondary = secondary, primary
	}
	caption := secondary + " · " + usageAgentNames(m.agents)
	if m.unpriced {
		caption += " · " + locales.UsagePriceUnknown()
	}
	selected := a.usageModel == m.key
	row := ui.ButtonBase(c.Key("usage-model:"+m.key)).Gap(12).Padding(8, 16).MinHeight(44).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(m.key)
	if selected {
		row.Background(pal.ActiveRow)
	} else if row.Hovered() {
		row.Background(pal.Muted)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, m.key).FontSize(12.5)
			clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
		})
		ui.Text(c, primary).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
	})
	if row.Clicked() {
		if selected {
			a.usageModel = ""
		} else {
			a.usageModel = m.key
		}
	}
}

// usageStores says where each agent keeps its history and what it takes,
// what of it Cleanup offers, and which installed agents do not save local
// token logs.
func (a *App) usageStores(c *ui.Context, s usageSummary, unsupported []string) {
	t := c.Theme()
	pal := Colors(t)
	listCard(c, locales.UsageWhere(), "", func() {
		for _, st := range s.stores {
			caption := locales.UsageSessionCount(st.sessions)
			dir := st.dir
			if dir == "" {
				if ag, ok := a.agent(st.agent); ok {
					dir = ag.Dir
				}
			}
			if dir != "" {
				caption = tilde(dir, a.Home) + " · " + caption
			}
			listRow(c, agentTitle(st.agent), false, caption, func() {
				ui.Text(c, formatBytes(st.size)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
			})
		}
		if s.old > 0 {
			days := int(cleanup.HistoryAfter.Hours() / 24)
			listRow(c, locales.UsageOldRow(days), false, locales.UsageOldCaption(), func() {
				ui.Text(c, formatBytes(s.old)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
				if iconButton(c, "arrow-right", locales.UsageOpenCleanup()).Clicked() {
					a.cleanupTab = slices.Index(cleanupGroups, cleanup.History) + 1
					a.router.Push(cleanupPage)
				}
			})
		}
		if len(unsupported) > 0 {
			ui.Column(c).Gap(3).Padding(10, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, locales.UsageUnsupportedTitle()).FontSize(12.5).FontWeight(500).TextColor(t.TextMuted)
				ui.Text(c, locales.UsageUnsupportedText(strings.Join(unsupported, ", "))).FontSize(11.5).TextColor(pal.TextFaint)
			})
		}
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
