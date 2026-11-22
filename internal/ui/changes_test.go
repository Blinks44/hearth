package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
)

// The page lists each check's changes under its date with their sizes,
// the terminal's version once with its copy's; the tries are the cleanup's
// candidates that came in the history. The status opens it, and the
// overview links it.
func TestChangesPage(t *testing.T) {
	a := cleanupApp()
	now := time.Now()
	a.History = []history.Entry{
		{Taken: now.Add(-time.Hour), Changes: []history.Change{
			{Kind: history.Added, Area: history.Copy, Name: "bun", Manager: "script", Path: home + "/.bun/bin/bun", Root: home + "/.bun", To: "1.2.5"},
			{Kind: history.Updated, Area: history.Copy, Name: "node", Manager: "fnm", Path: home + "/.fnm/node-versions/v22.12.0/bin/node", Root: home + "/.fnm/node-versions/v22.12.0", From: "22.12.0", To: "22.14.0"},
			{Kind: history.Updated, Area: history.Terminal, Name: "node", From: "22.12.0", To: "22.14.0"},
			{Kind: history.Added, Area: history.MCP, Name: "postgres", Agent: "claude", Path: home + "/.claude.json"},
		}},
		{Taken: now.AddDate(0, 0, -3), Changes: []history.Change{
			{Kind: history.Added, Area: history.Copy, Name: "node", Manager: "brew", Path: "/opt/homebrew/Cellar/node/23.6.0/bin/node", Root: "/opt/homebrew/Cellar/node/23.6.0", To: "23.6.0"},
			{Kind: history.Removed, Area: history.Copy, Name: "python", Manager: "uv", Path: home + "/.local/share/uv/python/3.11/bin/python", From: "3.11.11", Bytes: 61_000_000},
			{Kind: history.Updated, Area: history.Packages, Name: "brew", Manager: "Homebrew", Names: []string{"node", "python@3.13", "sqlite", "git", "jq"}},
			{Kind: history.Added, Area: history.PathEntry, Name: home + "/.zshrc", Path: home + "/.deno/bin"},
		}},
	}
	a.router.Push(overview.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "3 changes since the last check")
	if err := tt.Click("3 changes since the last check"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "changes")
	if a.router.Path() != changesPage.path {
		t.Fatalf("the link went to %q", a.router.Path())
	}
	hasTexts(t, tt, "What changed", "Bun 1.2.5 · script", "came in ~/.bun", "+",
		"Node.js 22.12.0 → 22.14.0 · fnm", "MCP postgres · Claude Code", "added to ~/.claude.json",
		"Node.js 23.6.0 · brew", "+ 200 MB", "Python 3.11.11 · uv", "− 61 MB",
		"Homebrew: 5 packages updated", "node, python@3.13, sqlite, git, …", "PATH: a new entry", "~/.deno/bin · from ~/.zshrc",
		"Looks like a try", "Node.js 23.6.0 · brew · 200 MB", "Open in Cleanup", "The last 30 checks with changes")
	if tt.HasText("the terminal runs another version") {
		t.Error("the terminal's version shows apart from its copy's")
	}
	if err := tt.Click("Open in Cleanup"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage {
		t.Errorf("the button went to %q", a.router.Path())
	}
	if err := tt.Click("What changed"); err != nil { // the sidebar's status
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != changesPage.path {
		t.Errorf("the status went to %q", a.router.Path())
	}
	a.changesTab = 3 // removed
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Bun 1.2.5 · script") || !tt.HasText("Python 3.11.11 · uv") {
		t.Errorf("removed tab: %q", tt.Texts())
	}
	a.History = nil
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "No changes yet: the history fills as checks find something new, updated or gone.")
}

func TestCheckDateDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone unavailable: %v", err)
	}
	tests := []struct {
		name     string
		now, ts  time.Time
		wantPref string
	}{
		{
			name:     "spring forward 23h day",
			now:      time.Date(2026, 3, 9, 10, 0, 0, 0, ny),
			ts:       time.Date(2026, 3, 8, 18, 30, 0, 0, ny),
			wantPref: "Yesterday",
		},
		{
			name:     "fall back 25h day",
			now:      time.Date(2026, 11, 2, 10, 0, 0, 0, ny),
			ts:       time.Date(2026, 11, 1, 18, 30, 0, 0, ny),
			wantPref: "Yesterday",
		},
		{
			name:     "utc timestamp converted to local yesterday",
			now:      time.Date(2026, 6, 10, 1, 0, 0, 0, ny),
			ts:       time.Date(2026, 6, 10, 2, 0, 0, 0, time.UTC), // 22:00 on June 9 in NY
			wantPref: "Yesterday",
		},
		{
			name:     "same day",
			now:      time.Date(2026, 3, 9, 10, 0, 0, 0, ny),
			ts:       time.Date(2026, 3, 9, 8, 15, 0, 0, ny),
			wantPref: "Today",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCheckDate(tc.now, tc.ts)
			if len(got) < len(tc.wantPref) || got[:len(tc.wantPref)] != tc.wantPref {
				t.Errorf("formatCheckDate(%v, %v) = %q, want prefix %q", tc.now, tc.ts, got, tc.wantPref)
			}
		})
	}
}

func TestChangesRowsClickable(t *testing.T) {
	now := time.Now()
	entries := []history.Entry{{
		Taken: now.Add(-time.Hour),
		Changes: []history.Change{
			{Kind: history.Added, Area: history.Copy, Name: "bun", Manager: "script", Path: home + "/.bun/bin/bun", To: "1.2.5"},
			{Kind: history.Updated, Area: history.Terminal, Name: "python", From: "3.12.0", To: "3.13.1"},
			{Kind: history.Updated, Area: history.Packages, Name: "brew", Manager: "Homebrew", Names: []string{"jq", "git"}},
			{Kind: history.Added, Area: history.PathEntry, Name: home + "/.zshrc", Path: home + "/.deno/bin"},
			{Kind: history.Added, Area: history.Agent, Name: "claude", To: "1.0.5"},
			{Kind: history.Added, Area: history.Skill, Name: "deploy-check", Path: home + "/.claude/skills/deploy-check"},
			{Kind: history.Added, Area: history.MCP, Name: "postgres", Agent: "claude", Path: home + "/.claude.json"},
		},
	}}
	for _, tc := range []struct {
		name, click, wantPath string
		check                 func(*testing.T, *App)
	}{
		{name: "copy", click: "Bun 1.2.5 · script", wantPath: "/tools/bun"},
		{name: "terminal", click: "Python 3.12.0 → 3.13.1", wantPath: "/tools/python"},
		{name: "packages", click: "Homebrew: 2 packages updated", wantPath: "/system/packages", check: func(t *testing.T, a *App) {
			if a.packageTab != 0 {
				t.Errorf("packageTab = %d, want 0", a.packageTab)
			}
		}},
		{name: "path", click: "PATH: a new entry", wantPath: pathPage, check: func(t *testing.T, a *App) {
			if a.pathTab != 0 || a.pathFilter != home+"/.deno/bin" {
				t.Errorf("pathTab=%d pathFilter=%q", a.pathTab, a.pathFilter)
			}
		}},
		{name: "agent", click: "Claude Code 1.0.5", wantPath: agentPage, check: func(t *testing.T, a *App) {
			if a.router.Query("id") != "claude" {
				t.Errorf("agent query id = %q, want claude", a.router.Query("id"))
			}
		}},
		{name: "skill", click: "Skill deploy-check", wantPath: skillsPage, check: func(t *testing.T, a *App) {
			if a.skillQuery != "deploy-check" {
				t.Errorf("skillQuery = %q, want deploy-check", a.skillQuery)
			}
		}},
		{name: "mcp", click: "MCP postgres · Claude Code", wantPath: mcpPage, check: func(t *testing.T, a *App) {
			if a.mcpChosen != "\x00postgres" {
				t.Errorf("mcpChosen = %q, want %q", a.mcpChosen, "\x00postgres")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := cleanupApp()
			a.Snapshot.MCP = []inventory.MCPServer{{Name: "postgres", Agent: "claude", Transport: "stdio", Command: "npx"}}
			a.History = entries
			a.router.Push(changesPage.path)
			tt := ui.NewTester(a.View, 1280, 900)
			tt.Advance(time.Second)
			if err := tt.Click(tc.click); err != nil {
				t.Fatalf("click %q: %v", tc.click, err)
			}
			tt.Advance(time.Second)
			if a.router.Path() != tc.wantPath {
				t.Errorf("went to %q, want %q", a.router.Path(), tc.wantPath)
			}
			if tc.check != nil {
				tc.check(t, a)
			}
		})
	}
}

func TestTriesCardClickFiltersHistory(t *testing.T) {
	a := cleanupApp()
	now := time.Now()
	a.History = []history.Entry{
		{Taken: now.Add(-time.Hour), Changes: []history.Change{
			{Kind: history.Added, Area: history.Copy, Name: "bun", Manager: "script", Path: home + "/.bun/bin/bun", Root: home + "/.bun", To: "1.2.5"},
		}},
		{Taken: now.AddDate(0, 0, -3), Changes: []history.Change{
			{Kind: history.Added, Area: history.Copy, Name: "node", Manager: "brew", Path: "/opt/homebrew/Cellar/node/23.6.0/bin/node", Root: "/opt/homebrew/Cellar/node/23.6.0", To: "23.6.0"},
		}},
	}
	a.router.Push(changesPage.path)
	a.changesTab = 2 // Updated tab initially so click must reset to All tab
	tt := ui.NewTester(a.View, 1280, 900)
	tt.Advance(time.Second)
	if err := tt.Click("Node.js 23.6.0 · brew · 200 MB"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != changesPage.path {
		t.Fatalf("went to %q, want %q", a.router.Path(), changesPage.path)
	}
	if a.changesTab != 0 {
		t.Errorf("changesTab = %d, want 0", a.changesTab)
	}
	if a.changesFilter == "" {
		t.Fatal("changesFilter was not populated from tries row click")
	}
	hasTexts(t, tt, "Node.js 23.6.0 · brew")
	if tt.HasText("Bun 1.2.5 · script") {
		t.Errorf("history was not filtered to clicked try item: %q", tt.Texts())
	}
}
