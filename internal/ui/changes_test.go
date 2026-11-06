package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/history"
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
