package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

func openOverview(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// calmSnapshot is one Node the terminal and apps both run.
func calmSnapshot() *inventory.Snapshot {
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		{Tool: "node", Version: "22.12.0", Path: "/opt/homebrew/bin/node", OnPath: 1, InGUI: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
	}}}}
}

// Before the first scan the overview says it is getting to know the
// computer, with skeletons for the tools and a way to add a project.
func TestOverviewFirstScan(t *testing.T) {
	a := NewApp()
	a.Scanning = true
	a.ChooseFolder = func() {}
	tt := openOverview(t, a)
	hasTexts(t, tt, "Checking", "Getting to know this computer", "found so far", "no projects yet",
		"Issues show after the check", "No projects yet", "Add folder")
	if tt.HasText("Node.js 22.12") || tt.HasText("v22.12.0") {
		t.Errorf("a tool before the scan: %q", tt.Texts())
	}
	// Projects read before the scan ends show as not checked.
	a.Projects = testProjects()
	tt.Frame()
	hasTexts(t, tt, "legacy-admin", "not checked")
	a.router.Push(projectsPage.path)
	tt.Advance(time.Second)
	hasTexts(t, tt, "legacy-admin")
}

// Without issues the hero is green and the issues card says so; the
// tool's card and the project's row show where they stand.
func TestOverviewAllGood(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Projects = testProjects()[:1]
	tt := openOverview(t, a)
	hasTexts(t, tt, "Ready to work", "Everything is in order",
		"Found 1 tool. No issues: projects get the versions they need, and copies on PATH do not clash.",
		"No issues found", "v22.12.0", "brew · 1 version", "in PATH", "1 project", "shop-web", "node 22")
	if err := tt.Click("shop-web"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := projectPath(home + "/dev/shop-web"); a.router.Location() != want {
		t.Errorf("clicking the project went to %q, want %q", a.router.Location(), want)
	}
}

// With three issues or more the hero asks for attention, the card lists
// the first three with their pages, and the rest go in a line.
func TestOverviewManyIssues(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	a.Snapshot.TerminalPath = []inventory.PathEntry{{Dir: "/opt/gone", Missing: true}}
	a.Projects = testProjects()
	tt := openOverview(t, a)
	hasTexts(t, tt, "4 issues", "Needs attention", "Node: 2 copies on PATH", "Node differs outside the terminal",
		"legacy-admin needs Node 18", "1 more: PATH lists folders that do not exist",
		"2 copies", "other in apps", "no 18", "no Node 18", "2 projects")
	// The PATH's item counts its issue.
	if !tt.HasText("1") {
		t.Errorf("no count on the PATH's item: %q", tt.Texts())
	}
	// The first is the duplicate's: warnings come first, in the rules'
	// order.
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/tools/node" {
		t.Errorf("Details went to %q", a.router.Path())
	}
}

// One or two issues are almost ready.
func TestOverviewFewIssues(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{testProjects()[1]}
	tt := openOverview(t, a)
	hasTexts(t, tt, "1 issue", "Almost ready", "legacy-admin needs Node 18")
}
