package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

func searchApp() *App {
	a := NewApp()
	a.Snapshot = found("node")
	a.Snapshot.Tools[0].Installations[0].Version = "22.12.0"
	a.Snapshot.Tools[0].Installations[0].Path = "/usr/local/bin/node"
	a.Snapshot.Skills = []inventory.Skill{{Name: "component-workflow", Description: "Reusable components", Dir: "/skills/component-workflow", Paths: []inventory.SkillPath{{Path: "/skills/component-workflow"}}}}
	a.Snapshot.MCP = []inventory.MCPServer{{Name: "docs-server", Agent: "codex"}}
	a.Snapshot.Tools = append(a.Snapshot.Tools,
		inventory.ToolState{Tool: "cli", Cli: &inventory.CliState{Tools: []inventory.Installation{{Tool: "kubectl", Version: "1.34.1", Path: "/opt/homebrew/bin/kubectl"}}}},
		inventory.ToolState{Tool: "mobile", Mobile: &inventory.MobileState{Xcodes: []inventory.XcodeApp{{Path: "/Applications/Xcode.app", Version: "27.2"}}}},
		inventory.ToolState{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{{Kind: "MongoDB", Version: "9.0.4", Path: "/opt/homebrew/bin/mongod", Source: "brew"}}}})
	a.Projects = []projects.Project{{Dir: "/work/shop-web", Requirements: []projects.Requirement{{Tool: "node", Want: "22", File: ".nvmrc"}}}}
	return a
}

func TestSearchResults(t *testing.T) {
	a := searchApp()
	for _, tc := range []struct{ name, query, want string }{
		{"tool", "NODE", "/tools/node"}, {"version", "node 22", "version:node:"}, {"project requirement", "node .nvmrc", "/projects/project?dir="}, {"skill description", "reusable components", "skill:"}, {"MCP", "docs-server", "mcp:"}, {"page", "mobile development", "/tools/mobile"}, {"CLI tool", "kubectl", "version:cli:"}, {"Xcode", "xcode 27", "version:mobile:"}, {"database", "mongodb", "version:databases:"}, {"all words required", "node nonexistent", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.search.query = tc.query
			got := a.searchResults()
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatal(got)
				}
				return
			}
			if !slices.ContainsFunc(got, func(r searchResult) bool { return len(r.id) >= len(tc.want) && r.id[:len(tc.want)] == tc.want }) {
				t.Fatalf("missing %s: %v", tc.want, got)
			}
		})
	}
}

func TestSearchKeyboard(t *testing.T) {
	a := searchApp()
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("node")
	tt.Frame()
	if a.search.query != "node" {
		t.Fatalf("autofocus failed: %q", a.search.query)
	}
	screenshot(t, tt, "search-results")
	tt.Key(0, ui.KeyDown)
	if a.search.selected != 1 {
		t.Fatal(a.search.selected)
	}
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if a.search.open || a.router.Path() != "/tools/node" {
		t.Fatal("Enter did not open tool")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !tt.HasText("RECENT") || !tt.HasText("Node.js") {
		t.Fatal("missing recent result")
	}
	screenshot(t, tt, "search-recent")
	tt.Key(0, ui.KeyEscape)
	if a.search.open {
		t.Fatal("Escape did not close")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Key(ui.Cmd, ui.KeyK)
	if a.search.open {
		t.Fatal("shortcut did not toggle")
	}
	if err := tt.Click("Find a tool…"); err != nil {
		t.Fatal(err)
	}
	tt.Type("zzzzzz")
	tt.Frame()
	if !tt.HasText("No results for “zzzzzz”") {
		t.Fatal(tt.Texts())
	}
	screenshot(t, tt, "search-empty")
	a.NodeVersions = &versions.Node{Releases: []versions.Release{{Version: "99.0.0"}}}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("99.0.0")
	tt.Frame()
	if err := tt.Click("Search available versions"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("node 99.0.0") {
		t.Fatal(tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.search.open || a.nodeTab != 1 || a.router.Path() != "/tools/node" {
		t.Fatal("available version did not open")
	}
}

func TestSearchDetails(t *testing.T) {
	for _, tc := range []struct{ name, query, path string }{{"skill", "component-workflow", skillsPage}, {"MCP", "docs-server", mcpPage}, {"project", "shop-web", projectPage}} {
		t.Run(tc.name, func(t *testing.T) {
			a := searchApp()
			a.skillQuery = "other"
			a.mcpQuery = "other"
			tt := ui.NewTester(a.View, 1280, 820)
			tt.Key(ui.Cmd, ui.KeyK)
			tt.Type(tc.query)
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			tt.Advance(time.Second)
			if !within(a.router.Path(), tc.path) {
				t.Fatal(a.router.Path())
			}
			if tc.name == "skill" && a.skillChosen != "\x00/skills/component-workflow" {
				t.Fatal(a.skillChosen)
			}
			if tc.name == "MCP" && a.mcpChosen != "\x00docs-server" {
				t.Fatal(a.mcpChosen)
			}
		})
	}
}

func TestSearchActionsAndAppearance(t *testing.T) {
	a := searchApp()
	scans := 0
	a.OnScan = func() { scans++ }
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if scans != 1 || a.search.open {
		t.Fatal("scan action did not run once")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.SetDark(true)
	tt.Frame()
	screenshot(t, tt, "search-dark")
	// Reopening and rebuilding must not duplicate action identities.
	tt.Frame()
	tt.Frame()
	if len(a.search.recent) != 0 {
		t.Fatal("actions should stay in the actions group")
	}
}

func TestSearchScroll(t *testing.T) {
	a := searchApp()
	a.NodeVersions = &versions.Node{}
	for i := range 40 {
		a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: fmt.Sprintf("%d.0.0", 40-i)})
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyK)
	a.searchAvailable(true)
	tt.Frame()
	top, ok := tt.Find("node 40.0.0")
	if !ok {
		t.Fatal(tt.Texts())
	}
	// The chosen first row must not hold the list where it is.
	before, _ := tt.Find("node 35.0.0")
	tt.Scroll(top.X+10, top.Y+100, 0, 300)
	tt.Frame()
	if after, _ := tt.Find("node 35.0.0"); after.Y > before.Y-200 {
		t.Fatalf("scrolling did not move the results: %v → %v", before, after)
	}
	// The keyboard brings its choice into view: the last row, from the first.
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	last, _ := tt.Find("node 1.0.0")
	footer, _ := tt.Find("40 results")
	if last.Y < top.Y || last.Y+last.H > footer.Y {
		t.Fatalf("the chosen row is out of view: %v, footer %v", last, footer)
	}
}

func TestSearchExtendedIndex(t *testing.T) {
	a := searchApp()
	a.Home = home
	a.Snapshot.Agents = []inventory.Agent{
		{ID: "claude", Installations: []inventory.Installation{{Tool: "claude", Version: "1.0.12", Path: home + "/.local/bin/claude", OnPath: 1}}},
	}
	a.Snapshot.Tools = append(a.Snapshot.Tools,
		inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Models: []inventory.Model{{Name: "qwen2.5-coder:7b", Format: "GGUF", Quant: "Q4_K_M", Size: 4_700_000_000}}},
		}},
		inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Packages: []inventory.SystemPackage{
				{Name: "ripgrep", Version: "14.1.1", Kind: "formula"},
			}},
		}},
	)
	a.Snapshot.TerminalPath = []inventory.PathEntry{
		{Dir: home + "/.cargo/bin", Sources: []inventory.PathSource{{File: home + "/.zshrc"}}},
		{Dir: "/no/such/path-entry", Missing: true},
	}
	a.Snapshot.Variables = []inventory.EnvironmentVariable{
		{Name: "GOPATH", Terminal: home + "/go"},
	}

	for _, tc := range []struct {
		name, query, wantID, wantPath string
		check                         func(*testing.T, *App)
	}{
		{name: "AI agent", query: "claude 1.0.12", wantID: "agent:claude", wantPath: agentPage, check: func(t *testing.T, a *App) {
			if a.router.Query("id") != "claude" {
				t.Errorf("agent query id = %q, want claude", a.router.Query("id"))
			}
		}},
		{name: "local model", query: "qwen2.5-coder", wantID: "model:Ollama:qwen2.5-coder:7b", wantPath: modelsPage},
		{name: "visible issue", query: "/no/such/path-entry", wantID: "issue:", wantPath: problemsPage, check: func(t *testing.T, a *App) {
			if a.problemsTab != 0 || a.problemsFilter == "" {
				t.Errorf("problemsTab=%d problemsFilter=%q", a.problemsTab, a.problemsFilter)
			}
		}},
		{name: "PATH entry", query: ".cargo/bin", wantID: "path:" + home + "/.cargo/bin", wantPath: pathPage, check: func(t *testing.T, a *App) {
			if a.pathTab != 0 || a.pathFilter != home+"/.cargo/bin" {
				t.Errorf("pathTab=%d pathFilter=%q", a.pathTab, a.pathFilter)
			}
		}},
		{name: "environment variable", query: "GOPATH", wantID: "var:GOPATH", wantPath: pathPage, check: func(t *testing.T, a *App) {
			if a.pathTab != 2 || a.pathFilter != "GOPATH" {
				t.Errorf("pathTab=%d pathFilter=%q", a.pathTab, a.pathFilter)
			}
		}},
		{name: "system package", query: "ripgrep", wantID: "pkg:/opt/homebrew/bin/brew:ripgrep", wantPath: packagesManagerPage, check: func(t *testing.T, a *App) {
			if a.packageManager != "/opt/homebrew/bin/brew" || a.packageFilter != "ripgrep" || a.router.Query("path") != "/opt/homebrew/bin/brew" {
				t.Errorf("packageManager=%q packageFilter=%q query=%q", a.packageManager, a.packageFilter, a.router.Query("path"))
			}
		}},
		{name: "install catalog", query: "python install", wantID: "install:python", wantPath: setupPage, check: func(t *testing.T, a *App) {
			if a.setup.wizard || a.setup.filter != 0 {
				t.Errorf("wizard=%v filter=%d", a.setup.wizard, a.setup.filter)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.router.Push(overview.path)
			a.search.query = tc.query
			got := a.searchResults()
			i := slices.IndexFunc(got, func(r searchResult) bool {
				return len(r.id) >= len(tc.wantID) && r.id[:len(tc.wantID)] == tc.wantID
			})
			if i < 0 {
				t.Fatalf("query %q missing %q in %v", tc.query, tc.wantID, got)
			}
			got[i].open()
			if a.router.Path() != tc.wantPath {
				t.Errorf("open %q went to %q, want %q", got[i].id, a.router.Path(), tc.wantPath)
			}
			if tc.check != nil {
				tc.check(t, a)
			}
		})
	}
}
