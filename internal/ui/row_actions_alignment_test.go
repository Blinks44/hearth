package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func accessButtonsByName(tt *ui.Tester, name string) []ui.Rect {
	var out []ui.Rect
	for _, info := range tt.AccessAll() {
		if info.Name == name && (info.Role == ui.RoleButton || info.Role == ui.RoleMenuButton) {
			out = append(out, info.Bounds)
		}
	}
	return out
}

func assertAlignedColumn(t *testing.T, label string, rects []ui.Rect, wantCount int) float32 {
	t.Helper()
	if len(rects) != wantCount {
		t.Fatalf("%s: got %d buttons, want %d (%+v)", label, len(rects), wantCount, rects)
	}
	x := rects[0].X
	for i, r := range rects {
		if r.X != x {
			t.Fatalf("%s[%d].X = %v, want %v (all=%+v)", label, i, r.X, x, rects)
		}
	}
	return x
}

func TestRowActionsAlignment(t *testing.T) {
	sizes := [][2]int{{1280, 820}, {1000, 680}}

	t.Run("path", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.Shell = &fakeShell{}
		a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
		a.Snapshot = &inventory.Snapshot{
			TerminalPath: []inventory.PathEntry{
				{Dir: home + "/.local/share/mise/shims", Missing: true, Sources: []inventory.PathSource{{File: home + "/.zprofile", Line: 9}}},
				{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: home + "/.zprofile", Line: 2}}},
				{Dir: "/usr/local/bin"},
			},
			GUIPath: []inventory.PathEntry{{Dir: "/usr/local/bin"}},
		}
		a.router.Push("/system/path")
		tt := ui.NewTester(a.View, sizes[0][0], sizes[0][1])
		for _, sz := range sizes {
			tt.SetSize(sz[0], sz[1])
			tt.Advance(time.Second)
			openX := assertAlignedColumn(t, "path open-source", accessButtonsByName(tt, locales.PathOpenSource()), 2)
			copyX := assertAlignedColumn(t, "path copy", accessButtonsByName(tt, locales.CopyPath()), 3)
			moreX := assertAlignedColumn(t, "path more", accessButtonsByName(tt, locales.MoreActions()), 2)
			if !(openX < copyX && copyX < moreX) {
				t.Fatalf("size %v: order open=%v copy=%v more=%v, want open < copy < more", sz, openX, copyX, moreX)
			}
		}
	})

	t.Run("packages", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.Shell = &fakeShell{}
		a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
				{Name: "amneziavpn", Version: "5.1.1", Kind: "cask", Path: "/opt/homebrew/Caskroom/amneziavpn"},
				{Name: "brotli", Version: "1.2.1", Kind: "formula", Path: "/opt/homebrew/Cellar/brotli/1.2.1", Dependency: true},
				{Name: "node", Version: "24.0.0", Kind: "formula", Path: "/opt/homebrew/Cellar/node/24.0.0", Dependencies: []string{"brotli"}},
				{Name: "virtual-tool", Version: "1.0.0", Kind: "formula"},
			}},
		}}}}
		a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
		tt := ui.NewTester(a.View, sizes[0][0], sizes[0][1])
		for _, sz := range sizes {
			tt.SetSize(sz[0], sz[1])
			tt.Advance(time.Second)
			lastRow, ok := tt.Find("virtual-tool")
			if !ok {
				t.Fatalf("size %v: missing virtual-tool row", sz)
			}
			inTable := func(rects []ui.Rect) []ui.Rect {
				var out []ui.Rect
				for _, r := range rects {
					if r.Y <= lastRow.Y+lastRow.H+16 {
						out = append(out, r)
					}
				}
				return out
			}
			copyX := assertAlignedColumn(t, "pkg copy", inTable(accessButtonsByName(tt, locales.CopyPath())), 3)
			folderX := assertAlignedColumn(t, "pkg folder", inTable(accessButtonsByName(tt, showInFolder())), 3)
			moreX := assertAlignedColumn(t, "pkg more", inTable(accessButtonsByName(tt, locales.MoreActions())), 3)
			if !(copyX < folderX && folderX < moreX) {
				t.Fatalf("size %v: order copy=%v folder=%v more=%v, want copy < folder < more", sz, copyX, folderX, moreX)
			}
		}
	})

	t.Run("ports", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		shop := home + "/dev/shop"
		a.Projects = []projects.Project{{Dir: shop}}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{{Kind: "PostgreSQL", Name: "PostgreSQL", Source: "brew", Status: "running", Service: "postgresql@17"}},
			Listeners: []inventory.DatabaseListener{
				{Address: "0.0.0.0", Port: "631"},
				{Address: "*", Port: "3000", Process: "node", PID: "4312", Path: "/opt/homebrew/bin/node", Dirs: []string{shop + "/web"}, Own: true},
				{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "812", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Own: true},
			},
		}}}}
		a.router.Push(portsPage)
		tt := ui.NewTester(a.View, sizes[0][0], sizes[0][1])
		for _, sz := range sizes {
			tt.SetSize(sz[0], sz[1])
			tt.Advance(time.Second)
			moreX := assertAlignedColumn(t, "port more", accessButtonsByName(tt, locales.MoreActions()), 3)
			stopX := assertAlignedColumn(t, "port stop", accessButtonsByName(tt, locales.PortsStop()), 2)
			if !(stopX < moreX) {
				t.Fatalf("size %v: stopX=%v should sit left of moreX=%v", sz, stopX, moreX)
			}
			for _, label := range []string{locales.PortsOpenBrowser(), locales.OpenProject()} {
				for _, r := range accessButtonsByName(tt, label) {
					if r.X >= stopX {
						t.Fatalf("size %v: contextual %q at X=%v should sit left of stop X=%v", sz, label, r.X, stopX)
					}
				}
			}
		}
	})

	t.Run("databases", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.Shell = &fakeShell{}
		shop := home + "/dev/shop"
		a.Projects = []projects.Project{{Dir: shop}}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{
				{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Source: "brew", Status: "running", Service: "postgresql@17", Path: "/opt/homebrew/Cellar/postgresql@17/17.2"},
				{Kind: "Redis", Name: "cache", Version: "redis:7", Source: "docker", Status: "running", Ports: []string{"6379"}, Project: shop},
			},
			Listeners: []inventory.DatabaseListener{
				{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "812", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Own: true},
				{Address: "127.0.0.1", Port: "6379", Process: "docker", PID: "900", Own: true},
			},
		}}}}
		a.router.Push("/tools/databases")
		tt := ui.NewTester(a.View, sizes[0][0], sizes[0][1])
		for _, sz := range sizes {
			tt.SetSize(sz[0], sz[1])
			tt.Advance(time.Second)
			assertAlignedColumn(t, "db more", accessButtonsByName(tt, locales.MoreActions()), 2)
			if !tt.HasText("shop") {
				t.Fatalf("size %v: expected inline project chip for docker database", sz)
			}
		}
	})

	t.Run("models", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		tt := ui.NewTester(a.View, sizes[0][0], sizes[0][1])
		now := tt.Now()
		a.Snapshot = &inventory.Snapshot{Taken: now, Tools: []inventory.ToolState{
			{Tool: "models", Models: &inventory.ModelsState{
				Ollama: &inventory.ModelApp{Path: "/opt/homebrew/bin/ollama", Version: "0.5.4", Port: "11434", Models: []inventory.Model{
					{Name: "qwen2.5-coder:32b", Size: 19_000_000_000},
				}},
				LMStudio: &inventory.ModelApp{Port: "1234", Models: []inventory.Model{
					{Name: "Mistral-Nemo-12B-GGUF", Size: 7_500_000_000},
				}},
			}},
		}}
		a.router.Push(modelsPage)
		for _, sz := range sizes {
			tt.SetSize(sz[0], sz[1])
			tt.Advance(time.Second)
			copyX := assertAlignedColumn(t, "model copy", accessButtonsByName(tt, locales.ModelsCopyName()), 2)
			moreX := assertAlignedColumn(t, "model more", accessButtonsByName(tt, locales.MoreActions()), 1)
			if !(copyX < moreX) {
				t.Fatalf("size %v: order copy=%v more=%v, want copy < more", sz, copyX, moreX)
			}
		}
	})
}

func TestProjectOverviewServicesCardUniform(t *testing.T) {
	makeApp := func(p projects.Project) *App {
		a := NewApp()
		a.Home = home
		a.Projects = []projects.Project{p}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{
				{Kind: "MariaDB", Name: "MariaDB", Version: "11.4", Source: "docker", Status: "running", Project: p.Dir},
				{Kind: "Redis", Name: "Redis", Version: "7.2", Source: "docker", Status: "stopped", Project: p.Dir},
			},
		}}}}
		a.RunScript = func(ops.ScriptSpec, func(int), func([]containerlog.LogLine), func(error)) func() { return func() {} }
		a.router.Push(projectPath(p.Dir))
		return a
	}

	moduix := projects.Project{
		Dir: home + "/dev/moduix",
		Scripts: []projects.Script{
			{Name: "dev:docs", Source: "package.json", Command: "pnpm --filter docs dev"},
			{Name: "dev:playground:react", Source: "package.json", Command: "pnpm --filter react dev"},
			{Name: "dev:playground:react-tailwind", Source: "package.json", Command: "pnpm --filter react-tw dev"},
			{Name: "dev:playground:solid", Source: "package.json", Command: "pnpm --filter solid dev"},
			{Name: "build", Source: "package.json", Command: "pnpm build"},
		},
	}
	planeta := projects.Project{
		Dir: home + "/dev/planeta",
		Scripts: []projects.Script{
			{Name: "dev", Source: "Makefile", Command: "make dev"},
			{Name: "dev", Source: "package.json", Command: "npm run dev"},
			{Name: "start", Source: "package.json", Command: "npm start"},
			{Name: "storybook", Source: "package.json", Command: "npm run storybook"},
			{Name: "test", Source: "Makefile", Command: "make test"},
		},
		Databases: []projects.DatabaseReference{
			{Kind: "MariaDB", File: "compose.yaml"},
			{Kind: "Redis", File: "compose.yaml"},
		},
	}

	var moduixLinkY, moduixScriptY float32
	for _, tc := range []struct {
		name string
		proj projects.Project
	}{
		{name: "moduix", proj: moduix},
		{name: "planeta", proj: planeta},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := makeApp(tc.proj)
			tt := ui.NewTester(a.View, 1280, 820)
			tt.Advance(time.Second)
			screenshot(t, tt, "proj-ov-"+tc.name)

			allLinkLabel := locales.ProjectScriptsAll() + " · 5"
			linkBox, okLink := tt.Find(allLinkLabel)
			if !okLink {
				t.Fatalf("missing header link %q", allLinkLabel)
			}
			firstScript, okFirst := tt.Find(tc.proj.Scripts[0].Name)
			if !okFirst {
				t.Fatalf("missing first quick script %q", tc.proj.Scripts[0].Name)
			}
			if firstScript.Y <= linkBox.Y {
				t.Errorf("quick scripts (Y=%.1f) should sit below header link (Y=%.1f)", firstScript.Y, linkBox.Y)
			}
			if tc.name == "moduix" {
				moduixLinkY, moduixScriptY = linkBox.Y, firstScript.Y
			} else {
				if linkBox.Y != moduixLinkY || firstScript.Y != moduixScriptY {
					t.Errorf("Run & services geometry differs between projects: planeta=(link=%.1f,script=%.1f) moduix=(link=%.1f,script=%.1f)",
						linkBox.Y, firstScript.Y, moduixLinkY, moduixScriptY)
				}
			}
		})
	}
}

func TestBalanceProjectCards(t *testing.T) {
	makeSlots := func(ws ...int) []projectCardSlot {
		out := make([]projectCardSlot, len(ws))
		for i, w := range ws {
			out[i] = projectCardSlot{weight: w}
		}
		return out
	}
	sum := func(slots []projectCardSlot) int {
		total := 0
		for _, s := range slots {
			total += s.weight
		}
		return total
	}
	for _, tc := range []struct {
		name    string
		weights []int
		maxDiff int
	}{
		{"full stack (6 cards)", []int{2, 3, 3, 4, 3, 7}, 0},
		{"medium stack (4 cards)", []int{3, 4, 3, 7}, 3},
		{"no infra (3 cards)", []int{4, 3, 7}, 0},
		{"minimal (2 cards)", []int{2, 4}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := balanceProjectCards(makeSlots(tc.weights...))
			if len(left) == 0 || len(right) == 0 {
				t.Fatalf("both columns must be non-empty: left=%d right=%d", len(left), len(right))
			}
			diff := sum(left) - sum(right)
			if diff < 0 {
				diff = -diff
			}
			if diff > tc.maxDiff {
				t.Fatalf("weight diff = %d (left=%d right=%d), want <= %d", diff, sum(left), sum(right), tc.maxDiff)
			}
		})
	}
}

func TestProjectOverviewGridAndSinglePrepareButton(t *testing.T) {
	dir := home + "/dev/planeta"
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	a.LinkKube = func(string, *store.KubeLink) {}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {}
	a.RunScript = func(ops.ScriptSpec, func(int), func([]containerlog.LogLine), func(error)) func() { return func() {} }
	a.Projects = []projects.Project{{
		Dir: dir,
		Requirements: []projects.Requirement{
			{Tool: "php", Want: "~8.3.36", File: "composer.json", Key: "require.php"},
			{Tool: "composer", File: "composer.json"},
			{Tool: "docker", File: "compose.yaml"},
		},
		Scripts: []projects.Script{
			{Name: "dev", Source: "Makefile", Command: "make dev"},
			{Name: "start", Source: "package.json", Command: "npm start"},
			{Name: "storybook", Source: "package.json", Command: "npm run storybook"},
		},
		Files:     []string{"compose.yaml", "package.json", "composer.json"},
		Databases: []projects.DatabaseReference{{Kind: "MariaDB", Service: "db", File: "compose.yaml"}, {Kind: "Redis", Service: "redis", File: "compose.yaml"}},
		Kube:      []string{"deploy/"},
		Installs:  []string{"node_modules", "vendor", "build"},
		AI:        []string{"AGENTS.md", ".agents"},
	}}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "php"},
		{Tool: "composer"},
		{Tool: "docker", Installations: []inventory.Installation{{Tool: "docker", Version: "29.10.0", OnPath: 1}}, Docker: &inventory.DockerState{
			Selected: "colima",
			Contexts: []inventory.DockerContext{{Name: "colima", Local: true, Status: "available", Engine: "Colima"}},
		}},
		{Tool: "cli", Cli: &inventory.CliState{KubeContexts: []inventory.KubeContext{{Name: "dev-cluster"}}}},
	}}
	a.router.Push(projectPath(dir))

	tt := ui.NewTester(a.View, 1280, 1180)
	tt.Advance(time.Second)
	screenshot(t, tt, "proj-ov-full-grid")

	prepareBtns := accessButtonsByName(tt, locales.PrepareButton())
	if len(prepareBtns) != 1 {
		t.Fatalf("Prepare button count = %d (%v), want exactly 1 (header only, no duplicate in warning banner)", len(prepareBtns), prepareBtns)
	}
	// Secondary cards must be arranged across two columns (different X origins).
	dockerBox, okDocker := tt.Find(locales.OpenInDocker())
	filesBox, okFiles := tt.Find(locales.ProjectFiles())
	if !okDocker || !okFiles {
		t.Fatalf("expected Docker (%v) and Files (%v) cards on overview", okDocker, okFiles)
	}
	if dockerBox.X == filesBox.X {
		t.Fatalf("expected 2-column grid on wide window, got identical X=%.1f", dockerBox.X)
	}
	// Manifests folder action uses an icon button rather than a wide text button.
	if btns := accessButtonsByName(tt, locales.PrepareOpenFolder()); len(btns) != 1 || btns[0].W > 32 {
		t.Fatalf("expected compact iconButton for Kube manifests folder, got %+v", btns)
	}
}
