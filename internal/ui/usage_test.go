package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

// usageApp has sessions of both agents: two in a project, one deeper in
// it, one in the home folder, and an old one the period leaves out.
func usageApp(now time.Time) *App {
	day := func(ago int) string { return now.AddDate(0, 0, -ago).Format(time.DateOnly) }
	shop := home + "/dev/shop"
	a := NewApp()
	a.Home = home
	a.Projects = []projects.Project{{Dir: shop}}
	a.Snapshot = &inventory.Snapshot{Sessions: []inventory.AgentSession{
		{Agent: "claude", Dir: shop, Size: 2e6, Modified: now, Usage: []inventory.TokenUsage{
			{Day: day(0), Model: "claude-opus-5-5", Input: 1e6},           // $4
			{Day: day(1), Model: "claude-haiku-5-5", Output: 2e6},         // $1
			{Day: day(1), Model: "claude-fable-9", Input: 5e5, Output: 1}, // no price
		}},
		{Agent: "codex", Dir: shop + "/web", Size: 1e6, Modified: now, Usage: []inventory.TokenUsage{
			{Day: day(2), Model: "gpt-6.1-sol", Output: 1e6}, // $10
		}},
		{Agent: "claude", Dir: home, Size: 1e6, Modified: now, Usage: []inventory.TokenUsage{
			{Day: day(0), Model: "claude-opus-5-5", Output: 1e5}, // $2
		}},
		{Agent: "codex", Dir: "/tmp/old", Size: 5e6, Modified: now.Add(-cleanup.HistoryAfter - time.Hour), Usage: []inventory.TokenUsage{
			{Day: day(100), Model: "gpt-6.1-sol", Output: 1e6},
		}},
	}}
	return a
}

func TestUsageSummary(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.Local)
	a := usageApp(now)
	s := a.usageSummary(now, 30)
	if s.today.cost != 6 || s.total.cost != 17 || !s.total.unpriced || s.sessions != 3 || s.old != 5e6 {
		t.Errorf("today %+v, total %+v, sessions %d, old %d", s.today, s.total, s.sessions, s.old)
	}
	if len(s.bars) != 30 || s.bars[29].cost[0] != 6 || s.bars[27].cost[1] != 10 || s.bars[0].cost != nil {
		t.Errorf("bars %+v", s.bars[27:])
	}
	// The project has both agents' sessions, the one in its subfolder
	// too; the home's are outside projects.
	if len(s.byProject) != 2 || s.byProject[0].key != home+"/dev/shop" || s.byProject[0].sessions != 2 || s.byProject[0].cost != 15 ||
		!slices.Equal(s.byProject[0].agents, []string{"claude", "codex"}) || s.byProject[1].key != "" {
		t.Errorf("by project %+v", s.byProject)
	}
	if len(s.byModel) != 4 || s.byModel[0].key != "gpt-6.1-sol" || s.byModel[3].key != "claude-fable-9" || !s.byModel[3].unpriced {
		t.Errorf("by model %+v", s.byModel)
	}
	if len(s.stores) != 2 || s.stores[0].agent != "claude" || s.stores[0].size != 3e6 || s.stores[1].sessions != 2 {
		t.Errorf("stores %+v", s.stores)
	}
	// All time: a bar each month of the last year at least, the old
	// session's among them.
	if all := a.usageSummary(now, 0); !all.monthly || all.total.cost != 27 || len(all.bars) != 12 || all.bars[8].at.Month() != time.August || all.bars[8].cost == nil {
		t.Errorf("all time: %+v", all)
	}
}

// The page shows the period's cost and its projects and models; the
// arrow of the old sessions opens their tab of Cleanup.
func TestUsageView(t *testing.T) {
	a := usageApp(time.Now())
	a.router.Push(usagePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "usage")
	hasTexts(t, tt, "Usage", "Last 30 days", "≈ $17.00", "shop", "Outside projects", "gpt-6.1-sol", "price unknown",
		"Older than 90 days", "Claude Code", "Codex")
	if err := tt.Click("7 days"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Last 7 days")
	if err := tt.Click("Open in Cleanup"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage || cleanupGroups[a.cleanupTab-1] != cleanup.History {
		t.Errorf("path %s, tab %d", a.router.Path(), a.cleanupTab)
	}
}

func TestUsageTokensFiltersAndUnsupported(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.Local)
	day := func(ago int) string { return now.AddDate(0, 0, -ago).Format(time.DateOnly) }
	shop := home + "/dev/shop"
	api := home + "/dev/api"
	a := usageApp(now)
	a.Projects = append(a.Projects, projects.Project{Dir: api})
	a.Snapshot.Sessions = append(a.Snapshot.Sessions, inventory.AgentSession{
		Agent: "gemini", Dir: api, Size: 500_000, Modified: now,
		Usage: []inventory.TokenUsage{
			{Day: day(0), Model: "gemini-2.5-pro", Input: 2_000_000, Output: 500_000},
		},
	})
	a.Snapshot.Agents = []inventory.Agent{
		{ID: "claude", Installations: []inventory.Installation{{Path: "/usr/local/bin/claude"}}},
		{ID: "gemini", Installations: []inventory.Installation{{Path: "/usr/local/bin/gemini"}}},
		{ID: "cursor", App: "/Applications/Cursor.app"},
		{ID: "aider", Installations: []inventory.Installation{{Path: "/usr/local/bin/aider"}}},
	}

	// Filtered summary by agent and by project.
	byGemini := a.usageSummaryFiltered(now, 30, "gemini", "", true)
	if byGemini.sessions != 1 || byGemini.total.tokens != 2_500_000 || len(byGemini.byProject) != 1 || byGemini.byProject[0].key != api {
		t.Fatalf("gemini filtered summary = %+v", byGemini)
	}
	byOutside := a.usageSummaryFiltered(now, 30, "", usageProjectOutside, false)
	if byOutside.sessions != 1 || byOutside.total.cost != 2 || len(byOutside.byProject) != 1 || byOutside.byProject[0].key != "" {
		t.Fatalf("outside filtered summary = %+v", byOutside)
	}
	// Tokens mode sorts projects by tokens (shop has 4.5M+1M=5.5M vs api 2.5M) and records token bars.
	byTokens := a.usageSummaryFiltered(now, 30, "", "", true)
	if len(byTokens.bars[29].tokens) != len(usageAgents) || byTokens.bars[29].tokens[2] != 2_500_000 {
		t.Fatalf("gemini token bar = %+v", byTokens.bars[29].tokens)
	}
	if len(byTokens.byProject) < 2 || byTokens.byProject[0].key != shop {
		t.Fatalf("byProject tokens order = %+v", byTokens.byProject)
	}

	// UI view: Tokens tab + unsupported agents note (Cursor, Aider) at wide and narrow sizes.
	a.router.Push(usagePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Cost", "Tokens", "All agents", "All projects", "No local token history", "Cursor, Aider", "Gemini CLI")
	if err := tt.Click("Tokens"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.usageMetric != 1 {
		t.Fatalf("usageMetric = %d, want 1", a.usageMetric)
	}
	screenshot(t, tt, "usage-tokens-wide")

	narrow := ui.NewTester(a.View, 1000, 680)
	narrow.Advance(time.Second)
	hasTexts(t, narrow, "Tokens", "No local token history", "Cursor, Aider")
	screenshot(t, narrow, "usage-tokens-narrow")
}
