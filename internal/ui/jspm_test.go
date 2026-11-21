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

// Node 25 and later come without Corepack: say how to get it, not what
// its cache does.
func TestJSManagersWithoutCorepack(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{}
	a.router.Push("/tools/jspm")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if !tt.HasText("Corepack is not installed. Since Node 25 it no longer comes with Node and installs separately: npm install -g corepack.") {
		t.Errorf("texts: %q", tt.Texts())
	}
	if tt.HasText("Corepack downloads project versions on first use and keeps them in its cache. Hearth only reads the local cache and never runs these shims.") {
		t.Error("cache note without Corepack")
	}
}

// Corepack's shims with an empty cache, as right after corepack enable:
// in PATH, the version chosen per project, not "not in PATH".
func TestJSManagersCorepackShimsOnly(t *testing.T) {
	a := NewApp()
	shim := func(tool string) inventory.ToolState {
		return inventory.ToolState{Tool: tool, Installations: []inventory.Installation{{Tool: tool, Path: `C:\Users\Ann\AppData\Roaming\npm\` + tool + ".cmd", OnPath: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack", Shim: true}}}}
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{shim("pnpm"), shim("yarn")}}
	a.router.Push("/tools/jspm")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	notInPath := 0 // npm's card alone: there is no npm here
	for _, s := range tt.Texts() {
		if s == "not in PATH" {
			notInPath++
		}
	}
	if !tt.HasText("version from packageManager") || notInPath != 1 {
		t.Errorf("texts: %q", tt.Texts())
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

func TestJSManagersChipsAndGlobals(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "npm", Installations: []inventory.Installation{
			{Tool: "npm", Version: "10.9.0", OnPath: 1, Default: true, Path: home + "/.local/share/fnm/bin/npm", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}},
			{Tool: "npm", Version: "10.2.0", OnPath: 2, Path: "/opt/homebrew/bin/npm", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
		}},
		{Tool: "pnpm", Installations: []inventory.Installation{
			{Tool: "pnpm", Version: "9.15.0", OnPath: 1, Default: true, Path: home + "/.local/share/pnpm/pnpm", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise"},
				Packages: []inventory.Package{{Name: "turbo", Version: "2.3.0"}}},
		}},
		{Tool: "yarn", Installations: []inventory.Installation{
			{Tool: "yarn", Version: "1.22.22", OnPath: 1, Path: "/opt/homebrew/bin/yarn", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
				Packages: []inventory.Package{{Name: "serve", Version: "14.2.4"}}},
		}},
	}}
	a.router.Push("/tools/jspm")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "jspm-chips-globals")
	hasTexts(t, tt, "default", "in PATH", "duplicate in PATH", "pnpm -g", "turbo", "2.3.0", "yarn global", "serve", "14.2.4")

	// Global packages tab shows all three managers' globals too.
	a.jspmTab = 1
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "jspm-globals-tab")
	hasTexts(t, tt, "npm -g", "pnpm -g", "turbo", "yarn global", "serve")
}
