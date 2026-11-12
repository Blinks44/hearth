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

// The page groups the candidates with their reasons and sizes; the caches
// and the build cache are chosen at once, and a check box changes what is
// selected.
func TestCleanupPage(t *testing.T) {
	a := cleanupApp()
	a.router.Push(cleanupPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cleanup")
	hasTexts(t, tt, "CACHES · COME BACK BY THEMSELVES", "pnpm cache", "pnpm store prune", "6.4 GB",
		"EXTRA COPIES AND LEFTOVERS", "Node.js 23.6.0 · brew", "shadowed by fnm · no project asks for it",
		"nvm", "nvm is not set up in the shell · 2 versions of Node.js inside · no project asks for it", "1.0 GB",
		"DOCKER", "Build Cache", "docker builder prune · rebuilt when needed", "Images without containers", "4 images · pulled again when needed",
		"Stopped containers", "6 containers · not running",
		"ABANDONED PROJECTS", "old-landing: node_modules",
		"Found", "22.1 GB", "9 candidates", "Selected", "17.2 GB", "4 candidates", "5 min ago",
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
	hasTexts(t, tt, "18.2 GB", "5 candidates")
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
	hasTexts(t, tt, "Уборка", "КЭШИ · ВОССТАНОВЯТСЯ САМИ", "Кэш pnpm", "6,4 ГБ", "9 кандидатов", "4 кандидата",
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
	hasTexts(t, tt, "Remove selected · 18.2 GB", "5 candidates")
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{brew}})
	tt.Frame()
	if err := tt.Click("Node.js 23.6.0 · brew"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Remove selected · 18.4 GB"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("plan %+v, error %q", a.op, a.planErr)
	}
	screenshot(t, tt, "cleanup-plan")
	hasTexts(t, tt, "Clean up the selected", "pnpm store prune", "remove ~/.bun/install/cache", "remove ~/.cache/uv",
		"/usr/local/bin/docker --context orbstack builder prune --force", "for good", "Folders are removed for good",
		"remove ~/.nvm/versions/node/v20.18.0", "remove ~/.nvm/versions/node/v18.20.4", "/opt/homebrew/bin/brew uninstall --formula node",
		"Packages of the versions go with them")
}
