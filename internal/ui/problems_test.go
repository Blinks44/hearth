package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

func openProblems(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push(problemsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// Before the first scan the page waits for it; without issues it says
// there are none.
func TestProblemsEmpty(t *testing.T) {
	a := NewApp()
	tt := openProblems(t, a)
	hasTexts(t, tt, "Hearth compares the projects' versions with what is installed and checks PATH.")
	a.Snapshot = calmSnapshot()
	tt.Frame()
	hasTexts(t, tt, "No issues found. Hearth checks again when project files and shell settings change.")
	if tt.HasText("Important · worth fixing") {
		t.Errorf("a group without issues: %q", tt.Texts())
	}
}

// One issue shows whole, with its fix and the page it lives on; the
// sidebar counts it.
func TestProblemsOne(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{testProjects()[1]}
	tt := openProblems(t, a)
	hasTexts(t, tt, "IMPORTANT · WORTH FIXING", "legacy-admin needs Node 18",
		".nvmrc asks for Node 18, and no installed copy fits. Until it is installed, the project may not build or run.", "Open")
	if tt.HasText("FOR INFORMATION · SAFE TO LEAVE") {
		t.Errorf("the info group without info: %q", tt.Texts())
	}
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := projectPath(home + "/work/legacy-admin"); a.router.Location() != want {
		t.Errorf("Open went to %q, want %q", a.router.Location(), want)
	}
}

// Many issues show all, warnings first, by area in the tabs and by the
// filter.
func TestProblemsMany(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Snapshot.TerminalPath = []inventory.PathEntry{{Dir: filepath.Join(t.TempDir(), "gone"), Missing: true}}
	legacy := testProjects()[1]
	for i := range 10 {
		p := legacy
		p.Dir = fmt.Sprintf("%s/work/app-%d", home, i)
		a.Projects = append(a.Projects, p)
	}
	tt := openProblems(t, a)
	screenshot(t, tt, "problems")
	hasTexts(t, tt, "IMPORTANT · WORTH FIXING", "FOR INFORMATION · SAFE TO LEAVE",
		"app-0 needs Node 18", "app-9 needs Node 18", "PATH lists folders that do not exist")
	warnings, _ := tt.Find("IMPORTANT · WORTH FIXING")
	info, _ := tt.Find("FOR INFORMATION · SAFE TO LEAVE")
	if info.Y <= warnings.Y {
		t.Errorf("information at %v before warnings at %v", info.Y, warnings.Y)
	}
	// The sidebar's item counts them all.
	if !tt.HasText("11") {
		t.Errorf("no count of 11 issues: %q", tt.Texts())
	}

	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("app-0 needs Node 18") || !tt.HasText("PATH lists folders that do not exist") {
		t.Errorf("the PATH tab: %q", tt.Texts())
	}
	a.problemsTab = 0
	a.problemsFilter = "app-3"
	tt.Frame()
	if !tt.HasText("app-3 needs Node 18") || tt.HasText("app-4 needs Node 18") {
		t.Errorf("filtered by app-3: %q", tt.Texts())
	}
	a.problemsFilter = "nothing like it"
	tt.Frame()
	hasTexts(t, tt, "No matching issues.")
}

// What is only worth knowing, as tools apps do not find, counts nowhere
// but its group, and hides for good: the choice is saved, and Show hidden
// brings it back.
func TestProblemsHide(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	ts := &a.Snapshot.Tools[0]
	for i := range ts.Installations {
		ts.Installations[i].InGUI = 0
	}
	saved := 0
	a.OnSettings = func() { saved++ }
	tt := openProblems(t, a)
	hasTexts(t, tt, "FOR INFORMATION · SAFE TO LEAVE", "Apps outside the terminal do not see Node")
	// The sidebar's Issues item comes with no count.
	if texts := tt.Texts(); !slices.Contains(texts, "Issues") || texts[slices.Index(texts, "Issues")+1] != "Updates" {
		t.Errorf("the sidebar counts an issue only worth knowing: %q", texts)
	}
	if err := tt.Click("Hide"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if saved != 1 || len(a.Hidden) != 1 || tt.HasText("Apps outside the terminal do not see Node") {
		t.Fatalf("saved %d, hidden %q: %q", saved, a.Hidden, tt.Texts())
	}
	hasTexts(t, tt, "1 hidden item")
	if err := tt.Click("Show hidden"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if saved != 2 || len(a.Hidden) != 0 || !tt.HasText("Apps outside the terminal do not see Node") {
		t.Errorf("saved %d, hidden %q: %q", saved, a.Hidden, tt.Texts())
	}
}
