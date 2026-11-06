package ui

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/projects"
)

// testProjects are a project fnm switches to an installed Node by .nvmrc,
// one needing a Node not installed, and one needing Python, which the
// scan does not check.
func testProjects() []projects.Project {
	return []projects.Project{
		{
			Dir: home + "/dev/shop-web", Files: []string{".nvmrc", "pnpm-lock.yaml"}, AI: []string{"CLAUDE.md", "AGENTS.md"},
			Requirements: []projects.Requirement{
				{Tool: "node", Want: "22", File: ".nvmrc"},
				{Tool: "pnpm", File: "pnpm-lock.yaml"},
			},
		},
		{
			Dir: home + "/work/legacy-admin", Files: []string{".nvmrc", "package.json"}, AI: []string{"CLAUDE.md", ".mcp.json"},
			Requirements: []projects.Requirement{
				{Tool: "node", Want: "18", File: ".nvmrc"},
				{Tool: "yarn", Want: "1.22.22", File: "package.json", Key: "packageManager"},
			},
		},
		{
			Dir: home + "/lab/ml-notebooks", Files: []string{".python-version"},
			Requirements: []projects.Requirement{{Tool: "python", Want: "3.12", File: ".python-version"}},
		},
	}
}

func openProjects(t *testing.T, a *App, path string) *ui.Tester {
	t.Helper()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	a.Projects = testProjects()
	a.router.Push(path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func hasTexts(t *testing.T, tt *ui.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

// The list shows each project's tools from its files and its state; the
// tab of the ones needing attention and the filter narrow it.
func TestProjectList(t *testing.T) {
	a := NewApp()
	tt := openProjects(t, a, "/projects")
	hasTexts(t, tt, "shop-web", "~/dev/shop-web", "node 22", "pnpm", "All set",
		"legacy-admin", "node 18", "yarn 1.22.22", "no Node 18",
		"ml-notebooks", "python 3.12", "not checked", "3")
	if err := tt.Click("Need attention"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("shop-web") || !tt.HasText("legacy-admin") {
		t.Errorf("attention tab: %q", tt.Texts())
	}
	a.projectsTab = 0
	a.projectFilter = "lab"
	tt.Frame()
	if tt.HasText("legacy-admin") || !tt.HasText("ml-notebooks") {
		t.Errorf("filtered by lab: %q", tt.Texts())
	}
	a.projectFilter = "nothing like it"
	tt.Frame()
	hasTexts(t, tt, "No projects match")
}

// A project's page shows what it needs against what is installed and
// what the terminal runs there, explains its problem, and lists its AI
// settings and files.
func TestProjectPage(t *testing.T) {
	a := NewApp()
	tt := openProjects(t, a, "/projects")
	if err := tt.Click("legacy-admin"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Title() != "legacy-admin" {
		t.Fatalf("page %q titled %q", a.router.Path(), a.router.Title())
	}
	hasTexts(t, tt, "Projects · ~/work/legacy-admin",
		"2 tools", "0 of 2", "Claude Code",
		"The project needs Node 18 (.nvmrc). Installed: 20.18.1, 22.12.0, 23.6.0. In this folder the terminal runs v22.12.0.",
		"Node", "18", ".nvmrc", "20.18.1 · 22.12.0 · 23.6.0", "18 is not installed", "v22.12.0", "fnm",
		"Yarn", "1.22.22", "package.json · packageManager", "Hearth does not check Yarn yet",
		"Project AI settings", "instructions · Claude Code", "project MCP servers",
		"Project files", "node 18", "yarn 1.22.22")
}

// The back button before the title goes to the projects.
func TestProjectBack(t *testing.T) {
	a := NewApp()
	tt := openProjects(t, a, projectPath(home+"/dev/shop-web"))
	if err := tt.Click("Back to projects"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/projects" || !tt.HasText("Need attention") {
		t.Errorf("page %q, texts %q", a.router.Path(), tt.Texts())
	}
}

// removing makes RemoveProject take the project off a.Projects at once,
// as the app does, and records which it took.
func removing(a *App) *[]string {
	var removed []string
	a.RemoveProject = func(dir string) {
		removed = append(removed, dir)
		a.Projects = slices.DeleteFunc(slices.Clone(a.Projects), func(p projects.Project) bool { return p.Dir == dir })
	}
	return &removed
}

// Removing a project from its menu takes it off the list and goes back
// to the projects. The last one is removed: the page must not look it up
// by its place in the shortened list.
func TestProjectRemove(t *testing.T) {
	a := NewApp()
	removed := removing(a)
	tt := openProjects(t, a, projectPath(home+"/lab/ml-notebooks"))
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Remove from list"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !reflect.DeepEqual(*removed, []string{home + "/lab/ml-notebooks"}) || a.router.Path() != "/projects" {
		t.Errorf("removed %q, page %q", *removed, a.router.Path())
	}
	if tt.HasText("ml-notebooks") {
		t.Errorf("still listed: %q", tt.Texts())
	}
}

// On the list, the trash button and the row's context menu remove a
// project and stay on the list; the menu also opens it.
func TestProjectListRemove(t *testing.T) {
	a := NewApp()
	removed := removing(a)
	tt := openProjects(t, a, "/projects")
	r, ok := tt.Find("shop-web")
	if !ok {
		t.Fatalf("no shop-web in %q", tt.Texts())
	}
	// The first "Remove from list" button is shop-web's.
	if err := tt.Click("Remove from list"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !reflect.DeepEqual(*removed, []string{home + "/dev/shop-web"}) || a.router.Path() != "/projects" || tt.HasText("shop-web") {
		t.Errorf("trash: removed %q, page %q, texts %q", *removed, a.router.Path(), tt.Texts())
	}

	tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2) // now legacy-admin's row
	if err := tt.ChooseMenuItem("Remove from list"); err != nil {
		t.Fatalf("%v, menu %q", err, tt.Menu())
	}
	tt.Advance(time.Second)
	if len(*removed) != 2 || (*removed)[1] != home+"/work/legacy-admin" || a.router.Path() != "/projects" {
		t.Errorf("context menu: removed %q, page %q", *removed, a.router.Path())
	}

	tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2) // ml-notebooks
	if err := tt.ChooseMenuItem("Open project"); err != nil {
		t.Fatalf("%v, menu %q", err, tt.Menu())
	}
	tt.Advance(time.Second)
	if want := projectPath(home + "/lab/ml-notebooks"); a.router.Location() != want {
		t.Errorf("open: at %q, want %q", a.router.Location(), want)
	}
}

// Without projects the page asks for a folder.
func TestProjectsEmpty(t *testing.T) {
	a := NewApp()
	chosen := false
	a.ChooseFolder = func() { chosen = true }
	a.router.Push("/projects")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Add a folder with code: Hearth shows which versions it needs and whether they are installed.")
	if err := tt.Click("Add folder"); err != nil {
		t.Fatal(err)
	}
	if !chosen {
		t.Error("Add folder did not ask for one")
	}
}

// The add dialog shows what the folder has and its nested projects, the
// ones with their own versions chosen; it adds the root and the chosen.
func TestAddDialog(t *testing.T) {
	a := NewApp()
	var added []string
	a.AddProjects = func(dirs []string) { added = dirs }
	tt := openProjects(t, a, "/projects")
	root := projects.Project{
		Dir: home + "/work/moduix", Files: []string{".nvmrc", "package.json"}, AI: []string{"AGENTS.md"},
		Requirements: []projects.Requirement{
			{Tool: "node", Want: "22", File: ".nvmrc"},
			{Tool: "pnpm", Want: "10.4.1", File: "package.json", Key: "packageManager"},
		},
	}
	nested := []projects.Project{
		{Dir: home + "/work/moduix/apps/web", Files: []string{".nvmrc"},
			Requirements: []projects.Requirement{{Tool: "node", Want: "20", File: ".nvmrc"}}},
		{Dir: home + "/work/moduix/apps/docs", Files: []string{"package.json"}},
	}
	a.Offer(root, nested)
	tt.Frame()
	hasTexts(t, tt, "Add project", "~/work/moduix", "packageManager", "10.4.1",
		"apps/web", "own .nvmrc: Node 20", "apps/docs", "same versions as the root", "Add 2 projects")
	// The switch ends the row, where the button ends the footer.
	row, _ := tt.Find("apps/docs")
	add, _ := tt.Find("Add 2 projects")
	tt.ClickAt(add.X+add.W-10, row.Y+row.H)
	tt.Frame()
	if err := tt.Click("Add 3 projects"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := []string{root.Dir, nested[0].Dir, nested[1].Dir}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("added %q, want %q", added, want)
	}
	if a.adding != nil || tt.HasText("Add project") {
		t.Error("the dialog stays open")
	}
}

// A folder of projects, as ~/code, is no project itself: only what it
// holds is added, all of it at first.
func TestAddDialogContainer(t *testing.T) {
	a := NewApp()
	var added []string
	a.AddProjects = func(dirs []string) { added = dirs }
	tt := openProjects(t, a, "/projects")
	nested := []projects.Project{{Dir: home + "/code/yarn-app", Files: []string{"package.json"}}}
	a.Offer(projects.Project{Dir: home + "/code"}, nested)
	tt.Frame()
	if err := tt.Click("Add 1 project"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if want := []string{nested[0].Dir}; !reflect.DeepEqual(added, want) {
		t.Errorf("added %q, want %q", added, want)
	}
}

// Corepack downloads the packageManager version on first run: not a
// warning that the tool is missing.
func TestInstalledCellCorepackDownloads(t *testing.T) {
	st := projects.Status{Tool: "yarn", Checked: true, Downloads: true}
	if value, caption, warn := installedCell(st); value != "—" || caption != "not in Corepack's cache yet" || warn {
		t.Errorf("%q %q %v", value, caption, warn)
	}
}

// Projects Hearth found by itself show by folder; the ones long unchanged
// are offered switched off, and adding takes the rest.
func TestOfferFound(t *testing.T) {
	a := NewApp()
	a.Home = home
	var added []string
	a.AddProjects = func(dirs []string) { added = dirs }
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	a.OfferFound([]string{home + "/dev"}, []projects.Suggestion{
		{Project: projects.Project{Dir: home + "/dev/shop-web", Files: []string{".nvmrc"}}, Changed: now.Add(-time.Hour)},
		{Project: projects.Project{Dir: home + "/dev/old-landing", Files: []string{"package.json"}}, Changed: now.AddDate(-1, 0, 0)},
	}, now)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Projects found in the usual folders", "~/dev · 2 projects", "shop-web", "old-landing",
		"unchanged for over half a year · may not need checking", "Add 1 project")
	if err := tt.Click("Add 1 project"); err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != home+"/dev/shop-web" {
		t.Errorf("added %q", added)
	}
}
