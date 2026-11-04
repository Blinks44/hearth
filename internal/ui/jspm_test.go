package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

func jsSnapshot() *inventory.Snapshot {
	return &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "npm", Installations: []inventory.Installation{{Tool: "npm", Version: "10.9.0", OnPath: 1, Path: home + "/.local/share/fnm/node-versions/v22.12.0/installation/bin/npm", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}, Packages: []inventory.Package{{Name: "typescript", Version: "5.7.2", Commands: []string{"tsc", "tsserver"}}}}}},
		{Tool: "pnpm", Managers: []inventory.Manager{{Name: "corepack", SwitchOnCD: true}}, Installations: []inventory.Installation{
			{Tool: "pnpm", Path: "/opt/homebrew/bin/pnpm", OnPath: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack", Shim: true}},
			{Tool: "pnpm", Version: "9.15.0", Default: true, Path: home + "/.cache/node/corepack/v1/pnpm/9.15.0", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack"}},
			{Tool: "pnpm", Version: "8.15.9", Path: home + "/.cache/node/corepack/v1/pnpm/8.15.9", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack"}},
		}},
		{Tool: "yarn", Installations: []inventory.Installation{{Tool: "yarn", Version: "4.5.3", Path: home + "/.cache/node/corepack/v1/yarn/4.5.3", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack"}}}},
		{Tool: "corepack", Installations: []inventory.Installation{{Tool: "corepack", Version: "0.30.0", Path: "/opt/homebrew/bin/corepack"}}},
	}}
}

func TestJSManagersPage(t *testing.T) {
	a := NewApp()
	a.Shell = &fakeShell{}
	a.Home = home
	a.Snapshot = jsSnapshot()
	a.JSVersions = &versions.JSManagers{Npm: versions.Latest{Version: "11.0.0"}, Pnpm: versions.Latest{Version: "9.15.4"}, Yarn: versions.Latest{Version: "4.6.0"}}
	a.Projects = []projects.Project{
		{Dir: home + "/code/moduix", Requirements: []projects.Requirement{{Tool: "pnpm", Want: "9.15.0", File: "package.json", Key: "packageManager"}, {Tool: "pnpm", File: "pnpm-lock.yaml"}}},
		{Dir: home + "/code/legacy-admin", Requirements: []projects.Requirement{{Tool: "pnpm", Want: "8.15.9", File: "package.json", Key: "packageManager"}, {Tool: "pnpm", File: "pnpm-lock.yaml"}}},
		{Dir: home + "/code/next-app", Requirements: []projects.Requirement{{Tool: "pnpm", Want: "10.0.0", File: "package.json", Key: "packageManager"}}},
		{Dir: home + "/code/api-docs", Requirements: []projects.Requirement{{Tool: "npm", File: "package-lock.json"}, {Tool: "yarn", File: "yarn.lock"}}},
	}
	a.router.Push("/tools/jspm")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	for _, text := range []string{"Managers", "Corepack shim", "pnpm v8.15.9", "Projects and their managers", "pnpm@9.15.0 · pnpm-lock.yaml", "multiple lockfiles", "Shims found on the terminal PATH", "typescript", "npm 11.0.0", "pnpm 10.0.0: Corepack downloads it on first run in the project"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q; texts: %q", text, tt.Texts())
		}
	}
	// The Corepack copy a project's packageManager picks carries its chip,
	// beside the project's own row.
	if n := slices.Index(tt.Texts(), "moduix"); n < 0 || !slices.Contains(tt.Texts()[n+1:], "moduix") {
		t.Errorf("no project chip on the cached copy; texts: %q", tt.Texts())
	}
	screenshot(t, tt, "jspm")
	// All tabs must rebuild safely, including a filtered table and PATH shims.
	for tab := 1; tab < 4; tab++ {
		a.jspmTab = tab
		tt.Frame()
		tt.Advance(time.Second)
	}
	a.jspmTab = 0
	a.jspmFilter = "no-match"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("No package managers found") {
		t.Error("filter did not apply")
	}
}

func TestJSManagersBeforeScan(t *testing.T) {
	a := NewApp()
	a.router.Push("/tools/jspm")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if !tt.HasText("npm · Not checked yet") {
		t.Fatalf("texts: %q", tt.Texts())
	}
}
