package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
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
		a.router.Push("/system/packages")
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
			var copyRects []ui.Rect
			for _, port := range []string{"631", "3000", "5432"} {
				copyRects = append(copyRects, accessButtonsByName(tt, locales.PortsCopyAddress(port))...)
			}
			copyX := assertAlignedColumn(t, "port copy", copyRects, 3)
			moreX := assertAlignedColumn(t, "port more", accessButtonsByName(tt, locales.MoreActions()), 3)
			if !(copyX < moreX) {
				t.Fatalf("size %v: copy=%v more=%v, want copy < more", sz, copyX, moreX)
			}
			for _, label := range []string{locales.PortsOpenBrowser(), locales.OpenProject(), locales.PortsStop()} {
				for _, r := range accessButtonsByName(tt, label) {
					if r.X >= copyX {
						t.Fatalf("size %v: contextual %q at X=%v should sit left of copy X=%v", sz, label, r.X, copyX)
					}
				}
			}
		}
	})

	t.Run("databases", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.Shell = &fakeShell{}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{
				{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Source: "brew", Status: "running", Service: "postgresql@17", Path: "/opt/homebrew/Cellar/postgresql@17/17.2"},
				{Kind: "Redis", Name: "cache", Version: "redis:7", Source: "docker", Status: "running", Ports: []string{"6379"}},
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
			copyX := assertAlignedColumn(t, "db copy", accessButtonsByName(tt, locales.DatabaseCopyURL()), 2)
			folderX := assertAlignedColumn(t, "db folder", accessButtonsByName(tt, showInFolder()), 1)
			moreX := assertAlignedColumn(t, "db more", accessButtonsByName(tt, locales.MoreActions()), 1)
			if !(copyX < folderX && folderX < moreX) {
				t.Fatalf("size %v: order copy=%v folder=%v more=%v, want copy < folder < more", sz, copyX, folderX, moreX)
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
