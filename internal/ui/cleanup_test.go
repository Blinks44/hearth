package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cleanupApp is a computer with the design's candidates: caches, an extra
// Node from brew, nvm left behind, Docker's data and an old project.
func cleanupApp() *App {
	a := NewApp()
	a.Home = home
	vm, pm := inventory.KindVersionManager, inventory.KindPackageManager
	cp := func(version, manager string, kind inventory.Kind, root string, term inventory.PathRank) inventory.Installation {
		return inventory.Installation{Tool: "node", Version: version, Path: root + "/bin/node", OnPath: term,
			Source: inventory.Source{Kind: kind, Manager: manager, Root: root}}
	}
	fnm := cp("22.12.0", "fnm", vm, home+"/.fnm/node-versions/v22.12.0", 1)
	fnm.Default = true
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		fnm,
		cp("23.6.0", "brew", pm, "/opt/homebrew/Cellar/node/23.6.0", 2),
		cp("20.18.0", "nvm", vm, home+"/.nvm/versions/node/v20.18.0", 0),
		cp("18.20.4", "nvm", vm, home+"/.nvm/versions/node/v18.20.4", 0),
	}}}}
	a.Projects = []projects.Project{{Dir: home + "/dev/old-landing", Installs: []string{"node_modules"}, Changed: time.Now().AddDate(0, -8, 0)}}
	a.Sizes = &inventory.Sizes{Taken: time.Now().Add(-5 * time.Minute),
		Paths: map[string]inventory.Usage{
			home + "/Library/pnpm/store":                           {Bytes: 6_400_000_000},
			home + "/.bun/install/cache":                           {Bytes: 2_700_000_000},
			home + "/.cache/uv":                                    {Bytes: 2_600_000_000},
			"/opt/homebrew/Cellar/node/23.6.0":                     {Bytes: 200_000_000},
			home + "/.nvm/versions/node/v20.18.0":                  {Bytes: 500_000_000},
			home + "/.nvm/versions/node/v18.20.4":                  {Bytes: 500_000_000},
			filepath.Join(home+"/dev/old-landing", "node_modules"): {Bytes: 900_000_000},
		},
		Caches: []inventory.Cache{
			{ID: "pnpm", Path: home + "/Library/pnpm/store"},
			{ID: "bun", Path: home + "/.bun/install/cache"},
			{ID: "uv", Path: home + "/.cache/uv"},
		},
		Docker: []inventory.DockerUsage{
			{Type: "Images", Count: 16, Active: 12, Reclaimable: 2_000_000_000},
			{Type: "Containers", Count: 8, Active: 2, Reclaimable: 800_000_000},
			{Type: "Local Volumes", Count: 3, Active: 3, Reclaimable: 400_000_000},
			{Type: "Build Cache", Count: 40, Reclaimable: 5_500_000_000},
		},
	}
	return a
}

// The page groups the candidates with their reasons and sizes; the caches,
// the build cache and the installs of a project not changed for long are
// chosen at once, and a check box changes what is selected.
func TestCleanupPage(t *testing.T) {
	a := cleanupApp()
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cleanup")
	hasTexts(t, tt, "CACHES · COME BACK BY THEMSELVES", "pnpm cache", "~/Library/pnpm/store", "6.4 GB",
		"EXTRA COPIES AND LEFTOVERS", "Node.js 23.6.0 · brew", "shadowed by fnm · no project asks for it",
		"nvm", "nvm is not set up in the shell · 2 versions of Node.js inside · no project asks for it", "1.0 GB",
		"DOCKER", "Build Cache", "docker builder prune · rebuilt when needed", "Images without containers", "4 images · pulled again when needed",
		"Stopped containers", "6 containers · not running",
		"PROJECT DEPENDENCIES", "old-landing", "node_modules",
		"Found", "22.1 GB", "9 candidates", "Selected", "18.1 GB", "5 candidates", "5 min ago",
		"Docker volumes", "come back by themselves · 17.2 GB")
	if tt.HasText("Local Volumes") {
		t.Error("volumes are offered")
	}
	// The sidebar says what can be freed.
	if !tt.HasText("22.1 GB") {
		t.Error("no size in the sidebar")
	}
	if err := tt.Click("nvm"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "19.1 GB", "6 candidates")
	// Docker's check box is mixed, with only the build cache chosen: a
	// click chooses the whole group, the next one none of it.
	for _, want := range []string{"21.9 GB", "13.6 GB"} {
		if err := tt.Click("Select all: Docker"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		hasTexts(t, tt, want)
	}
	a.cleanupTab = 3 // Docker
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("pnpm cache") || !tt.HasText("Build Cache") {
		t.Errorf("Docker tab: %q", tt.Texts())
	}
}

// Before the first count the page waits for it, and the overview says the
// sizes are being counted; then it shows what can be freed.
func TestCleanupCounting(t *testing.T) {
	a := cleanupApp()
	sizes := a.Sizes
	a.Sizes, a.Counting = nil, true
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Sizes are being counted after the check.")
	a.router.Push(overview.path)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Can be freed", "counting sizes…")
	a.Sizes, a.Counting = sizes, false
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Can be freed", "22.1 GB", "caches and extra copies")
}

// In Russian in a narrow window the side cards go under the table and
// sizes take a decimal comma.
func TestCleanupNarrowRussian(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := cleanupApp()
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "cleanup-narrow-ru")
	hasTexts(t, tt, "Уборка", "КЭШИ · ВОССТАНОВЯТСЯ САМИ", "Кэш pnpm", "6,4 ГБ", "9 кандидатов", "5 кандидатов",
		"nvm не подключён к шеллу · внутри 2 версии Node.js · проекты не просят", "Не предлагаем")
}

// The header's button removes the selected the plan removes, which the
// plan made with the terminal's environment lists a step each: the
// command of a cache the terminal finds, the folder of one it does not,
// Docker's prune in the context counted, the versions of a leftover
// whose nvm.sh is gone as folders, and a copy from Homebrew through brew,
// first.
func TestCleanupRemove(t *testing.T) {
	a := cleanupApp()
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "docker",
		Installations: []inventory.Installation{{Tool: "docker", Path: "/usr/local/bin/docker", OnPath: 1}},
		Docker:        &inventory.DockerState{Selected: "orbstack"}})
	brew := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Packages: []inventory.SystemPackage{
		{Name: "node", Kind: "formula", Path: "/opt/homebrew/Cellar/node/23.6.0"},
	}}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pnpm"+exeSuffix), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"PATH=" + bin}, nil))
	}
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	// Without Homebrew's packages known its copy is not removed: it cannot
	// be chosen.
	for _, row := range []string{"Node.js 23.6.0 · brew", "nvm"} {
		if err := tt.Click(row); err != nil {
			t.Fatal(err)
		}
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Remove selected · 19.1 GB", "6 candidates")
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{brew}})
	tt.Frame()
	if err := tt.Click("Node.js 23.6.0 · brew"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Remove selected · 19.3 GB"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("plan %+v, error %q", a.op, a.planErr)
	}
	screenshot(t, tt, "cleanup-plan")
	hasTexts(t, tt, "Clean up the selected", "remove ~/Library/pnpm/store", "remove ~/.bun/install/cache", "remove ~/.cache/uv",
		"/usr/local/bin/docker --context orbstack builder prune --force", "for good", "Folders are removed for good",
		"remove ~/.nvm/versions/node/v20.18.0", "remove ~/.nvm/versions/node/v18.20.4", "/opt/homebrew/bin/brew uninstall --formula node",
		"Packages of the versions go with them")
}

func TestCleanupCountingBanner(t *testing.T) {
	a := cleanupApp()
	a.Counting = true
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.CleanupCounting())
	if tt.HasText(locales.CleanupWaiting()) {
		t.Errorf("unexpected full-width waiting banner while candidates are already shown")
	}
}

func TestCleanupRecountDockerErrAndReveal(t *testing.T) {
	a := cleanupApp()
	a.Sizes.DockerErr = "daemon not running"
	recounted := 0
	a.RecountSizes = func() { recounted++ }
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	sh := &fakeShell{}
	a.Shell = sh
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	hasTexts(t, tt, "Recalculate sizes", "Could not read Docker disk usage: daemon not running")
	screenshot(t, tt, "s13-04-cleanup-1280")
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "s13-04-cleanup-1000")
	if err := tt.Click("Recalculate sizes"); err != nil {
		t.Fatal(err)
	}
	if recounted != 1 {
		t.Fatalf("RecountSizes called %d times, want 1", recounted)
	}
	if err := tt.Click(showInFolder()); err != nil {
		t.Fatalf("reveal button click failed: %v", err)
	}
	if len(sh.revealed) != 1 || sh.revealed[0] == "" {
		t.Fatalf("expected ShowItemInFolder to be called with candidate path, got %q", sh.revealed)
	}
}

func TestCleanupVisibleOnlyAndCountingDisabled(t *testing.T) {
	a := cleanupApp()
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "docker",
		Installations: []inventory.Installation{{Tool: "docker", Path: "/usr/local/bin/docker", OnPath: 1}},
		Docker:        &inventory.DockerState{Selected: "orbstack"}})
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"PATH=/usr/bin"}, nil))
	}
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Side card "Selected at once" matches the default Selected total (18.1 GB = 17.2 GB caches/build + 900 MB stale project).
	hasTexts(t, tt, "Selected at once", "18.1 GB", "Stale project dependencies", "unchanged for long · 900 MB")

	// While sizes are counting, Remove selected is disabled and clicking it does not propose a plan.
	a.Counting = true
	tt.Frame()
	if err := tt.Click("Remove selected · 18.1 GB"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op != nil {
		t.Fatalf("Remove selected proposed a plan while Counting: %+v", a.op)
	}
	a.Counting = false

	// Switching to Docker tab narrows Selected and Remove selected to visible Docker items only.
	a.cleanupTab = 3 // Docker
	tt.Frame()
	hasTexts(t, tt, "Remove selected · 5.5 GB", "1 candidate")
	if err := tt.Click("Remove selected · 5.5 GB"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("expected Docker-only plan, got %+v err=%q", a.op, a.planErr)
	}
	if len(a.op.plan.Steps) != 1 {
		t.Fatalf("Docker-only plan has %d steps, want 1: %+v", len(a.op.plan.Steps), a.op.plan.Steps)
	}
	a.op = nil

	// Filtering to "bun" removes only the visible bun cache, not hidden pnpm/uv/Docker/project selections.
	a.cleanupTab = 0
	a.cleanupFilter = "bun"
	tt.Frame()
	hasTexts(t, tt, "Remove selected · 2.7 GB", "1 candidate")
	if err := tt.Click("Remove selected · 2.7 GB"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("expected bun-only plan, got %+v err=%q", a.op, a.planErr)
	}
	if len(a.op.plan.Steps) != 1 || a.op.plan.Steps[0].Remove != home+"/.bun/install/cache" {
		t.Fatalf("bun-only plan steps = %+v", a.op.plan.Steps)
	}
}

func TestCleanupMultiPathAndOptInWorktrees(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	sh := &fakeShell{}
	a.Shell = sh
	p1 := home + "/.claude/debug"
	p2 := home + "/.claude/file-history"
	p3 := home + "/.claude/shell-snapshots"
	wt := home + "/.codex/worktrees"
	a.Sizes = &inventory.Sizes{
		Taken: time.Now().Add(-time.Minute),
		Paths: map[string]inventory.Usage{
			p1: {Bytes: 100_000_000},
			p2: {Bytes: 80_000_000},
			p3: {Bytes: 70_000_000},
			wt: {Bytes: 400_000_000},
		},
		Caches: []inventory.Cache{
			{ID: "claude-cache", Paths: []string{p1, p2, p3}},
			{ID: "codex-worktrees", Path: wt, OptIn: true},
		},
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"PATH=/usr/bin"}, nil))
	}
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// claude-cache is selected by default (250 MB) and shows "+2 more"; codex-worktrees is opt-in (unchecked).
	hasTexts(t, tt, "Claude Code cache", "~/.claude/debug · +2 more", "250 MB",
		"Codex worktrees", "abandoned worktrees", "400 MB",
		"Found", "650 MB", "2 candidates", "Selected", "250 MB", "1 candidate", "Remove selected · 250 MB")

	// Filter to Claude Code cache so its multi-path reveal button is the target, then open its menu.
	a.cleanupFilter = "claude"
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click(showInFolder()); err != nil {
		t.Fatalf("click multi-path reveal: %v", err)
	}
	if menu := tt.Menu(); len(menu) != 3 || menu[0] != "~/.claude/debug" || menu[1] != "~/.claude/file-history" || menu[2] != "~/.claude/shell-snapshots" {
		t.Fatalf("multi-path menu = %q, want 3 Claude cache paths", menu)
	}
	if err := tt.ChooseMenuItem("~/.claude/file-history"); err != nil {
		t.Fatalf("choose menu path: %v", err)
	}
	if len(sh.revealed) != 1 || sh.revealed[0] != p2 {
		t.Fatalf("revealed = %q, want [%q]", sh.revealed, p2)
	}
}

func TestCleanup2BreakdownAgeSortAndGrouping(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	now := time.Now()
	fresh := now.AddDate(0, 0, -5)
	mid := now.AddDate(0, -2, 0) // ~60d (>30d, <90d)
	old := now.AddDate(0, -7, 0) // ~210d (>180d)

	webDir := home + "/dev/web-app"
	monoDir := home + "/dev/moduix"
	monoSolid := filepath.Join(monoDir, "packages", "solid")
	monoVue := filepath.Join(monoDir, "packages", "vue")
	apiDir := home + "/dev/api-svc"
	legacyDir := home + "/dev/legacy"

	a.Sizes = &inventory.Sizes{
		Taken: now.Add(-2 * time.Minute),
		Paths: map[string]inventory.Usage{
			home + "/.bun/install/cache":             {Bytes: 2_000_000_000},
			filepath.Join(webDir, "node_modules"):    {Bytes: 900_000_000},
			filepath.Join(webDir, ".next"):           {Bytes: 400_000_000},
			filepath.Join(monoSolid, "node_modules"): {Bytes: 500_000_000},
			filepath.Join(monoSolid, "dist"):         {Bytes: 100_000_000},
			filepath.Join(monoVue, "node_modules"):   {Bytes: 400_000_000},
			filepath.Join(apiDir, "target"):          {Bytes: 600_000_000},
			filepath.Join(legacyDir, "node_modules"): {Bytes: 300_000_000},
		},
		Caches: []inventory.Cache{
			{ID: "bun", Path: home + "/.bun/install/cache"},
		},
		Installs: []inventory.ProjectInstall{
			{Project: webDir, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: fresh},
			{Project: webDir, Name: ".next", Role: inventory.InstallRoleBuild, Changed: fresh},
			{Project: monoSolid, Root: monoDir, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: fresh},
			{Project: monoSolid, Root: monoDir, Name: "dist", Role: inventory.InstallRoleBuild, Changed: fresh},
			{Project: monoVue, Root: monoDir, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: fresh},
			{Project: apiDir, Name: "target", Role: inventory.InstallRoleBuild, Changed: mid},
			{Project: legacyDir, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: old},
		},
	}

	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Stacked breakdown bar legend shows Caches (2.0 GB), Dependencies (2.1 GB), Build (1.1 GB).
	hasTexts(t, tt,
		"Caches", "2.0 GB",
		"Dependencies", "2.1 GB",
		"Build", "1.1 GB",
		// 2-folder project starts expanded with folder count badge.
		"web-app", "1.3 GB", locales.EditorsFolders(2), "node_modules", ".next",
		locales.CleanupLastActive(fresh),
		// 3-folder monorepo renders one parent header with folder count badge.
		"moduix", "1.0 GB", locales.EditorsFolders(3),
		// Single-folder projects also render with a uniform project header + indented folder row.
		"api-svc", "target", locales.CleanupLastActive(mid),
		"legacy",
		// Age filter and sort controls.
		locales.CleanupAgeAll(), locales.CleanupAge30(), locales.CleanupAge90(), locales.CleanupAge180(),
		locales.CleanupSortLargest(), locales.CleanupSortOldest(),
	)

	// Project header chevron slot and indented child checkboxes share the same X column.
	toggleRect, okT := tt.Find("web-app")
	childCheckRect, okC := tt.Find("web-app: node_modules")
	if !okT || !okC {
		t.Fatalf("expected web-app toggle (%v) and child checkbox (%v)", okT, okC)
	}
	if toggleRect.X != childCheckRect.X {
		t.Errorf("chevron slot X (%.1f) != child checkbox X (%.1f)", toggleRect.X, childCheckRect.X)
	}

	// Monorepo with 3+ folders starts collapsed by default.
	if tt.HasText("packages/solid/node_modules") {
		t.Errorf("3-folder monorepo should start collapsed: %q", tt.Texts())
	}

	// Project-level checkbox selects both web-app folders (1.3 GB + 2.0 GB bun + 300 MB legacy = 3.6 GB).
	if err := tt.Click(locales.CleanupSelectGroup("web-app")); err != nil {
		t.Fatalf("click web-app group checkbox: %v", err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "3.6 GB")

	// Project-level checkbox selects all collapsed monorepo folders (+1.0 GB = 4.6 GB) without expanding.
	if err := tt.Click(locales.CleanupSelectGroup("moduix")); err != nil {
		t.Fatalf("click moduix group checkbox: %v", err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "4.6 GB")
	if tt.HasText("packages/solid/node_modules") {
		t.Errorf("clicking project checkbox should not expand collapsed monorepo")
	}

	// Clicking the project header toggles expansion and reveals relative subpackage paths.
	if err := tt.Click("moduix"); err != nil {
		t.Fatalf("expand moduix: %v", err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "packages/solid/node_modules", "packages/solid/dist", "packages/vue/node_modules")

	// Clicking again collapses it back, and searching auto-expands matching rows.
	if err := tt.Click("moduix"); err != nil {
		t.Fatalf("collapse moduix: %v", err)
	}
	tt.Advance(time.Second)
	if tt.HasText("packages/solid/node_modules") {
		t.Errorf("moduix should be collapsed after second click")
	}
	a.cleanupFilter = "solid"
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "moduix", "packages/solid/node_modules", "packages/solid/dist")
	a.cleanupFilter = ""

	// Age filter >30d hides caches and 5-day-old web-app/moduix, keeping 60d api-svc and 210d legacy.
	a.cleanupAge = 1 // >30d
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Bun cache") || tt.HasText("web-app") || tt.HasText("moduix") {
		t.Errorf(">30d filter should hide Bun cache and fresh projects: %q", tt.Texts())
	}
	hasTexts(t, tt, "api-svc", "target", "legacy", "node_modules")

	// Age filter >180d keeps only legacy.
	a.cleanupAge = 3 // >180d
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("api-svc") || !tt.HasText("legacy") {
		t.Errorf(">180d filter mismatch: %q", tt.Texts())
	}

	// Sort Oldest puts legacy (210d) above api-svc (60d) when >30d is active.
	a.cleanupAge = 1
	a.cleanupSort = 1
	tt.Frame()
	tt.Advance(time.Second)
	texts := tt.Texts()
	legacyIdx, apiIdx := -1, -1
	for i, s := range texts {
		if s == "legacy" && legacyIdx < 0 {
			legacyIdx = i
		}
		if s == "api-svc" && apiIdx < 0 {
			apiIdx = i
		}
	}
	if legacyIdx < 0 || apiIdx < 0 || legacyIdx >= apiIdx {
		t.Errorf("Oldest sort order wrong: legacyIdx=%d apiIdx=%d texts=%q", legacyIdx, apiIdx, texts)
	}

	// Verify narrow window renders cleanly with all controls.
	a.cleanupAge = 0
	a.cleanupSort = 0
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	hasTexts(t, ttNarrow, "web-app", "1.3 GB", "moduix", locales.EditorsFolders(3), locales.CleanupAge180(), locales.CleanupSortOldest())
}
