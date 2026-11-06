package history

import (
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func node(version, manager, root string, term inventory.PathRank) inventory.Installation {
	return inventory.Installation{Tool: "node", Version: version, Path: root + "/bin/node", OnPath: term,
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: manager, Root: root}}
}

func TestDiff(t *testing.T) {
	prev := &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{node("22.12.0", "fnm", "/fnm/v22", 1), node("20.18.0", "nvm", "/nvm/v20", 0)}},
			{Tool: "go", Err: "timeout"},
			{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/brew", Packages: []inventory.SystemPackage{
				{Name: "git", Kind: "formula", Version: "2.47"}, {Name: "wget", Kind: "formula", Version: "1.24"}, {Name: "git", Kind: "cask", Version: "1"},
			}}}},
		},
		TerminalPath: []inventory.PathEntry{{Dir: "/usr/bin"}, {Dir: "/old/bin"}},
		Agents:       []inventory.Agent{{ID: "claude", Installations: []inventory.Installation{{Version: "2.1.0"}}}, {ID: "codex"}},
		Skills:       []inventory.Skill{{Name: "review", Dir: "/skills/review", Sum: "a"}, {Name: "gone", Dir: "/skills/gone", Sum: "b"}},
		MCP:          []inventory.MCPServer{{Name: "github", Agent: "claude", File: "/home/.claude.json"}},
	}
	// Node's 22 moved to 22.14 in fnm, which the terminal now runs; nvm's
	// 20 went, Bun came. Go failed before, so it tells nothing.
	cur := &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{node("22.14.0", "fnm", "/fnm/v22", 1)}},
			{Tool: "bun", Installations: []inventory.Installation{{Tool: "bun", Version: "1.2.5", Path: "/bun/bin/bun", OnPath: 2,
				Source: inventory.Source{Kind: inventory.KindInstaller, Manager: "script", Root: "/bun"}}}},
			{Tool: "go", Installations: []inventory.Installation{{Tool: "go", Version: "1.27.3", Path: "/go/bin/go"}}},
			{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/brew", Packages: []inventory.SystemPackage{
				{Name: "git", Kind: "formula", Version: "2.48"}, {Name: "jq", Kind: "formula", Version: "1.7"}, {Name: "git", Kind: "cask", Version: "1"},
			}}}},
		},
		TerminalPath: []inventory.PathEntry{{Dir: "/usr/bin"}, {Dir: "/home/.deno/bin", Sources: []inventory.PathSource{{File: "/home/.zshrc", Line: 48}}}},
		Agents:       []inventory.Agent{{ID: "claude", Installations: []inventory.Installation{{Version: "2.1.3"}}}, {ID: "codex", App: "/Applications/Codex.app", AppVersion: "0.3"}},
		Skills:       []inventory.Skill{{Name: "review", Dir: "/skills/review", Sum: "c"}},
		MCP:          []inventory.MCPServer{{Name: "github", Agent: "claude", File: "/home/.claude.json"}, {Name: "postgres", Agent: "claude", File: "/home/.claude.json"}},
	}
	sizes := &inventory.Sizes{Paths: map[string]inventory.Usage{"/nvm/v20": {Bytes: 61e6}}}
	want := []Change{
		{Kind: Updated, Area: Copy, Name: "node", Manager: "fnm", Path: "/fnm/v22/bin/node", Root: "/fnm/v22", From: "22.12.0", To: "22.14.0"},
		{Kind: Removed, Area: Copy, Name: "node", Manager: "nvm", Path: "/nvm/v20/bin/node", Root: "/nvm/v20", From: "20.18.0", Bytes: 61e6},
		{Kind: Updated, Area: Terminal, Name: "node", From: "22.12.0", To: "22.14.0"},
		{Kind: Added, Area: Packages, Name: "brew", Manager: "Homebrew", Names: []string{"jq"}},
		{Kind: Updated, Area: Packages, Name: "brew", Manager: "Homebrew", Names: []string{"git"}},
		{Kind: Removed, Area: Packages, Name: "brew", Manager: "Homebrew", Names: []string{"wget"}},
		{Kind: Added, Area: PathEntry, Name: "/home/.zshrc", Path: "/home/.deno/bin"},
		{Kind: Removed, Area: PathEntry, Path: "/old/bin"},
		{Kind: Updated, Area: Agent, Name: "claude", From: "2.1.0", To: "2.1.3"},
		{Kind: Added, Area: Agent, Name: "codex", To: "0.3"},
		{Kind: Updated, Area: Skill, Name: "review", Path: "/skills/review"},
		{Kind: Removed, Area: Skill, Name: "gone", Path: "/skills/gone"},
		{Kind: Added, Area: MCP, Name: "postgres", Agent: "claude", Path: "/home/.claude.json"},
	}
	// Bun is a tool the old scan had no state of: it was not looked for.
	got := Diff(prev, cur, sizes)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", got, want)
	}
	// Flatpak's branches of one name are two packages, not one added.
	branches := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "flatpak", Path: "/flatpak", Packages: []inventory.SystemPackage{
		{Name: "org.freedesktop.Platform.GL.default", Version: "26.2.5", Reference: "aarch64/26.08"},
		{Name: "org.freedesktop.Platform.GL.default", Version: "26.2.5", Reference: "aarch64/26.08-extra"},
	}}}}}}
	if got := Diff(branches, branches, nil); len(got) != 0 {
		t.Errorf("branches of one name: %+v", got)
	}
	if Diff(nil, cur, nil) != nil || len(Diff(cur, cur, nil)) != 0 {
		t.Error("changes without a check before, or of the same one")
	}
}

// The history keeps the newest first, at most Limit, and no empty check.
func TestAdd(t *testing.T) {
	var entries []Entry
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range Limit + 5 {
		entries = Add(entries, Entry{Taken: day.Add(time.Duration(i) * time.Hour), Changes: []Change{{Kind: Added}}})
	}
	entries = Add(entries, Entry{Taken: day.AddDate(0, 1, 0)})
	if len(entries) != Limit || !entries[0].Taken.Equal(day.Add(time.Duration(Limit+4)*time.Hour)) {
		t.Fatalf("%d entries, the first %v", len(entries), entries[0].Taken)
	}
}
