package ui

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
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
		"Node 18 is missing",
		"Required by .nvmrc · installed 20.18.1, 22.12.0, 23.6.0 · terminal runs v22.12.0",
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
	// Check the unchecked nested project and add all three.
	box, ok := tt.AccessByRole(ui.RoleCheckBox, "apps/docs")
	if !ok {
		t.Fatal("missing apps/docs checkbox")
	}
	tt.ClickAt(box.Bounds.X+box.Bounds.W/2, box.Bounds.Y+box.Bounds.H/2)
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
	dev := filepath.Join(home, "dev")
	a.OfferFound([]string{dev}, []projects.Suggestion{
		{Project: projects.Project{Dir: filepath.Join(dev, "shop-web"), Files: []string{".nvmrc"}}, Changed: now.Add(-time.Hour)},
		{Project: projects.Project{Dir: filepath.Join(dev, "old-landing"), Files: []string{"package.json"}}, Changed: now.AddDate(-1, 0, 0)},
	}, now)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Projects found in the usual folders", tilde(dev, home)+" · 2 projects", "shop-web", "old-landing",
		"unchanged for over half a year · may not need checking", "Add 1 project")
	if err := tt.Click("Add 1 project"); err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != filepath.Join(dev, "shop-web") {
		t.Errorf("added %q", added)
	}
}

// A Node the project needs and nobody installed installs from its row and
// from the banner, the newest release its file asks for, through fnm.
func TestProjectInstall(t *testing.T) {
	a := NewApp()
	a.NodeVersions = nodeVersions(time.Now())
	a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: "18.20.5"}, versions.Release{Version: "18.20.4"})
	tt := openProjects(t, a, projectPath(home+"/work/legacy-admin"))
	screenshot(t, tt, "project-install")
	for _, button := range []string{"Install Node 18…", "Install Node v18.20.5"} {
		if err := tt.Click(button); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText("fnm install 18.20.5") {
			t.Errorf("%s: %q", button, tt.Texts())
		}
		if err := tt.Click("Cancel"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
}

// The project menu pins the Node the terminal runs in the project's own
// version file, showing the change.
func TestProjectPin(t *testing.T) {
	a := NewApp()
	tt := openProjects(t, a, projectPath(home+"/dev/shop-web"))
	a.Projects[0].Texts = map[string]string{".nvmrc": "22\n"}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Pin Node 22.12.0…"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Pin Node 22.12.0 in shop-web", tilde(filepath.Join(home+"/dev/shop-web", ".nvmrc"), home), "- 22", "+ 22.12.0", "Pin 22.12.0")
	screenshot(t, tt, "project-pin")
}

// A project's Docker with its engine down is not ready, though the CLI
// meets the requirement, and the banner says what to start.
func TestProjectDockerEngineDown(t *testing.T) {
	st := projects.Status{Tool: "docker", Checked: true, EngineDown: true,
		Checks: []projects.Check{{Tool: "docker", File: "compose.yaml", Met: true}}}
	if got := statusProblem(st); got != "Docker not answering" {
		t.Errorf("problem %q", got)
	}
	got := problemBanner([]projects.Status{st})
	if got.Title != "Docker is not running" {
		t.Errorf("banner title %q", got.Title)
	}
	wantMeta := "Needed for compose.yaml"
	if got.Meta != wantMeta {
		t.Errorf("banner meta %q, want %q", got.Meta, wantMeta)
	}
}

func TestProjectReadErrorInAttentionAndColumnAlignment(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{
		testProjects()[0], // shop-web: All set
		{Dir: home + "/work/broken-json", Errs: []error{errors.New("package.json: invalid character")}},
	}
	a.router.Push("/projects")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "broken-json", "could not read")
	// Verify header State column aligns with the rows' status chips (chip text starts ~16px inside chip for padding + dot).
	stateHead, okHead := tt.Find("State")
	chip1, ok1 := tt.Find("All set")
	chip2, ok2 := tt.Find("could not read")
	if !okHead || !ok1 || !ok2 {
		t.Fatalf("missing State (%v), All set (%v), or could not read (%v)", okHead, ok1, ok2)
	}
	if chip1.X != chip2.X {
		t.Errorf("State chips misaligned between rows: row1 X=%v, row2 X=%v", chip1.X, chip2.X)
	}
	if diff := chip1.X - stateHead.X; diff < 0 || diff > 24 {
		t.Errorf("State column misaligned: header X=%v, chip text X=%v", stateHead.X, chip1.X)
	}
	if err := tt.Click("Need attention"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("shop-web") || !tt.HasText("broken-json") {
		t.Errorf("Need attention tab did not include read-error project: %q", tt.Texts())
	}
}

// Find projects is available both on the empty screen and in the populated
// header, and OfferFound surfaces toast feedback when there is nothing new.
func TestProjectFindButtonAndOfferFoundFeedback(t *testing.T) {
	t.Run("buttons trigger FindProjects", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		finds := 0
		a.FindProjects = func() { finds++ }
		a.ChooseFolder = func() {}
		a.router.Push("/projects")
		tt := ui.NewTester(a.View, 1280, 820)
		tt.Advance(time.Second)
		if err := tt.Click("Find projects"); err != nil {
			t.Fatalf("empty Find projects: %v; texts %q", err, tt.Texts())
		}
		if finds != 1 {
			t.Fatalf("empty finds = %d, want 1", finds)
		}

		a.Snapshot = nodeSnapshot()
		a.Projects = testProjects()
		tt.Frame()
		if err := tt.Click("Find projects"); err != nil {
			t.Fatalf("header Find projects: %v; texts %q", err, tt.Texts())
		}
		if finds != 2 {
			t.Fatalf("header finds = %d, want 2", finds)
		}
	})

	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	dev := filepath.Join(home, "dev")

	t.Run("no usual folders", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.OfferFound(nil, nil, now)
		if a.adding != nil || !strings.Contains(a.notice, "usual code folders") {
			t.Errorf("adding=%v notice=%q", a.adding, a.notice)
		}
	})

	t.Run("folders scanned with no projects", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		a.OfferFound([]string{dev}, nil, now)
		if a.adding != nil || !strings.Contains(a.notice, filepath.FromSlash("~/dev")) {
			t.Errorf("adding=%v notice=%q", a.adding, a.notice)
		}
	})

	t.Run("all found projects already added", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		existing := filepath.Join(dev, "shop-web")
		a.Projects = []projects.Project{{Dir: existing}}
		a.OfferFound([]string{dev}, []projects.Suggestion{
			{Project: projects.Project{Dir: existing}, Changed: now.Add(-time.Hour)},
		}, now)
		if a.adding != nil || !strings.Contains(a.notice, "already added") {
			t.Errorf("adding=%v notice=%q", a.adding, a.notice)
		}
	})

	t.Run("skips already added projects and offers remaining", func(t *testing.T) {
		a := NewApp()
		a.Home = home
		existing := filepath.Join(dev, "shop-web")
		fresh := filepath.Join(dev, "new-api")
		a.Projects = []projects.Project{{Dir: existing}}
		a.OfferFound([]string{dev}, []projects.Suggestion{
			{Project: projects.Project{Dir: existing}, Changed: now.Add(-time.Hour)},
			{Project: projects.Project{Dir: fresh}, Changed: now.Add(-time.Hour)},
		}, now)
		if a.adding == nil || len(a.adding.nested) != 1 || a.adding.nested[0].Dir != fresh {
			t.Fatalf("adding=%+v", a.adding)
		}
	})
}

// Filtering /projects matches required tools, titles, versions, config files,
// databases, and AI agents in addition to the folder path.
func TestProjectFilterByToolsAndDatabases(t *testing.T) {
	a := NewApp()
	tt := openProjects(t, a, "/projects")
	a.Projects[0].Databases = []projects.DatabaseReference{
		{Kind: "PostgreSQL", Service: "postgres", File: "compose.yaml", Ports: []string{"5432"}},
	}
	tt.Frame()

	for _, tc := range []struct {
		query string
		want  string
		hide  string
	}{
		{query: "python", want: "ml-notebooks", hide: "shop-web"},
		{query: "Yarn", want: "legacy-admin", hide: "shop-web"},
		{query: "3.12", want: "ml-notebooks", hide: "legacy-admin"},
		{query: "pnpm-lock.yaml", want: "shop-web", hide: "legacy-admin"},
		{query: "postgresql", want: "shop-web", hide: "ml-notebooks"},
		{query: "5432", want: "shop-web", hide: "legacy-admin"},
		{query: "Claude", want: "shop-web", hide: "ml-notebooks"},
	} {
		a.projectFilter = tc.query
		tt.Frame()
		if !tt.HasText(tc.want) || tt.HasText(tc.hide) {
			t.Errorf("filter %q: want %q, hide %q; texts %q", tc.query, tc.want, tc.hide, tt.Texts())
		}
	}
}

// Project detail shows a Project databases card when Compose/.env references
// databases, reflects running/stopped/port-conflict states, and links to Databases.
func TestProjectDatabasesCard(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{
		Tool: "databases",
		Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{
				{Kind: "PostgreSQL", Name: "postgresql@16", Status: "running", Ports: []string{"5432"}},
				{Kind: "Redis", Name: "redis", Status: "stopped", Ports: []string{"6379"}},
			},
			Listeners: []inventory.DatabaseListener{
				{Port: "5432", Process: "postgres", Path: "/opt/homebrew/bin/postgres"},
			},
		},
	})
	dir := home + "/dev/shop-web"
	p := testProjects()[0]
	a.Projects = []projects.Project{p}
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if tt.HasText("Project databases") {
		t.Fatalf("Project databases card should be hidden when project has no databases")
	}

	// Add Postgres (busy port 5432 conflict via Compose) and Redis (stopped).
	a.Projects[0].Databases = []projects.DatabaseReference{
		{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}},
		{Kind: "Redis", Service: "cache", File: ".env", Ports: []string{"6379"}},
	}
	tt.Frame()
	hasTexts(t, tt, "Project databases", "PostgreSQL · db", "compose.yaml · :5432", "Port 5432 busy",
		"Redis · cache", ".env · :6379", "stopped", "Databases")

	// When the port is not busy (e.g. .env reference with running server), show running.
	a.Projects[0].Databases = []projects.DatabaseReference{
		{Kind: "PostgreSQL", File: ".env", Ports: []string{"5432"}},
	}
	tt.Frame()
	hasTexts(t, tt, "Project databases", "PostgreSQL", ".env · :5432", "running")

	if err := tt.Click("Databases"); err != nil {
		t.Fatalf("click Databases button: %v", err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/tools/databases" {
		t.Errorf("Databases button navigated to %q, want /tools/databases", a.router.Path())
	}
}

// Quick install in a project's requirement table works across all Hearth-supported
// languages, not just Node.
func TestProjectInstallAllLanguages(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		setup    func(a *App)
		req      projects.Requirement
		wantCmd  string
		wantText string
	}{
		{
			name: "python via uv",
			setup: func(a *App) {
				a.Snapshot = pythonSnapshot()
				a.PythonVersions = pythonLines(now)
			},
			req:      projects.Requirement{Tool: "python", Want: "3.14", File: ".python-version"},
			wantCmd:  "uv python install 3.14.0",
			wantText: "Install Python 3.14.0",
		},
		{
			name: "go via mise",
			setup: func(a *App) {
				a.Snapshot = goSnapshot()
				a.GoVersions = &versions.Go{Releases: []string{"1.24.1", "1.23.5"}}
			},
			req:      projects.Requirement{Tool: "go", Want: "1.24", File: "go.mod"},
			wantCmd:  "mise install go@1.24.1",
			wantText: "Install Go 1.24.1",
		},
		{
			name: "java via mise",
			setup: func(a *App) {
				a.Snapshot = javaSnapshot()
				a.JavaVersions = &versions.Java{Lines: []versions.Cycle{{Cycle: "23", Latest: "23.0.2+7"}, {Cycle: "21", Latest: "21.0.6+7"}}}
			},
			req:      projects.Requirement{Tool: "java", Want: "23", File: "pom.xml"},
			wantCmd:  "mise install java@temurin-23",
			wantText: "Install Java Temurin 23.0.2+7",
		},
		{
			name: "ruby via rbenv",
			setup: func(a *App) {
				a.Snapshot = rubySnapshot()
				a.RubyVersions = &versions.Ruby{Lines: []versions.Cycle{{Cycle: "3.4", Latest: "3.4.2"}}}
			},
			req:      projects.Requirement{Tool: "ruby", Want: "3.4", File: ".ruby-version"},
			wantCmd:  "rbenv install 3.4.2",
			wantText: "Install Ruby 3.4.2",
		},
		{
			name: "bun via mise",
			setup: func(a *App) {
				a.Snapshot = bunSnapshot()
				a.Snapshot.Tools[0].Managers = []inventory.Manager{{Name: "mise", SwitchOnCD: true}}
				a.BunLatest = &versions.Latest{Version: "1.3.0"}
			},
			req:      projects.Requirement{Tool: "bun", Want: "1.3", File: ".bun-version"},
			wantCmd:  "mise install bun@1.3.0",
			wantText: "Install Bun 1.3.0",
		},
		{
			name: "dotnet sdk via mise",
			setup: func(a *App) {
				a.Snapshot = dotnetSnapshot()
				a.Snapshot.Tools[0].Managers = []inventory.Manager{{Name: "mise", SwitchOnCD: true}}
				a.DotnetVersions = &versions.Dotnet{Lines: []versions.DotnetLine{{Version: "10.0.101", Support: "lts", Phase: "active"}}}
			},
			req: projects.Requirement{
				Tool: "dotnet", Want: "10.0.100", File: "global.json",
				SDK: &projects.SDKPolicy{Version: "10.0.100", RollForward: "latestFeature"},
			},
			wantCmd:  "mise install dotnet@10.0.101",
			wantText: "Install .NET SDK 10.0.101",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApp()
			a.Home = home
			tc.setup(a)
			dir := home + "/work/polyglot"
			a.Projects = []projects.Project{{
				Dir: dir, Files: []string{tc.req.File}, Requirements: []projects.Requirement{tc.req},
			}}
			a.router.Push(projectPath(dir))
			tt := ui.NewTester(a.View, 1280, 820)
			tt.Advance(time.Second)
			if err := tt.Click(tc.wantText); err != nil {
				t.Fatalf("click %q: %v; texts %q", tc.wantText, err, tt.Texts())
			}
			tt.Advance(time.Second)
			if !tt.HasText(tc.wantCmd) {
				t.Errorf("plan missing %q; texts %q", tc.wantCmd, tt.Texts())
			}
		})
	}
}

// At the minimum window width (1000x680), the Tools column on /projects must
// retain enough width for tool badges instead of collapsing to an ellipsis.
func TestProjectListNarrowToolsVisible(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	a.Projects = testProjects()
	a.ChooseFolder, a.FindProjects = func() {}, func() {}
	a.router.Push("/projects")
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)

	tag, ok := tt.Find("node 22")
	if !ok {
		t.Fatalf("missing node 22 tag in %q", tt.Texts())
	}
	if tag.W < 40 {
		t.Errorf("node 22 tag collapsed at 1000px width: W=%v", tag.W)
	}
}

func TestProjectFileRowLayoutAndDeduplication(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	dir := home + "/work/moduix"
	a.Projects = []projects.Project{{
		Dir:   dir,
		Files: []string{"package.json", "pnpm-lock.yaml"},
		AI:    []string{".github/copilot-instructions.md"},
		Requirements: []projects.Requirement{
			{Tool: "node", Want: ">=24", File: "package.json", Key: "engines.node"},
			{Tool: "pnpm", Want: ">=12.14.0", File: "package.json", Key: "engines.pnpm"},
			{Tool: "pnpm", Want: "12.14.0", File: "package.json", Key: "packageManager"},
			{Tool: "node", Want: "^24.0.0", File: "package.json", Key: "devEngines.runtime"},
			{Tool: "pnpm", Want: "12.14.0", File: "package.json", Key: "devEngines.packageManager"},
			{Tool: "pnpm", File: "pnpm-lock.yaml"},
		},
	}}
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)

	hasTexts(t, tt, "package.json", "node >=24 · ^24.0.0", "pnpm >=12.14.0 · 12.14.0", "Copilot CLI")
	// Scroll filesCard into view on the narrow 1000x680 viewport; package.json
	// must keep full readable width and never collapse to "p...".
	tt.Scroll(500, 400, 0, 260)
	tt.Frame()
	pkg, ok := tt.Find("package.json")
	if !ok {
		t.Fatalf("package.json not found in %q", tt.Texts())
	}
	if pkg.W < 70 {
		t.Errorf("package.json collapsed: W=%v", pkg.W)
	}
	if got := projectAgents(a.Projects[0]); !slices.Contains(got, "copilot") {
		t.Errorf("projectAgents = %q, want copilot", got)
	}
}

func TestProblemBannerCountAndEmptyChecks(t *testing.T) {
	// EngineDown with no checks must not panic.
	banner := problemBanner([]projects.Status{{Tool: "docker", Checked: true, EngineDown: true}})
	if banner.Title != "Docker is not running" || banner.Meta == "" {
		t.Errorf("engine down banner = %+v", banner)
	}
	// Multiple unmet statuses summarize count in Title and list concrete tools in Meta.
	multi := problemBanner([]projects.Status{
		{Tool: "node", Checked: true, Checks: []projects.Check{{Tool: "node", Want: "18", File: ".nvmrc"}}},
		{Tool: "python", Checked: true, Checks: []projects.Check{{Tool: "python", Want: "3.12", File: ".python-version"}}},
	})
	if multi.Title != "Environment not ready · 2 issues" {
		t.Errorf("multi-problem banner title = %q", multi.Title)
	}
	if !strings.Contains(multi.Meta, "Node 18 not installed") || !strings.Contains(multi.Meta, "Python 3.12 not installed") {
		t.Errorf("multi-problem banner meta = %q", multi.Meta)
	}
	// Installed matching version with wrong active terminal copy states what runs vs what is needed.
	installed := inventory.Installation{Version: "18.20.0"}
	terminal := inventory.Installation{Version: "22.12.0"}
	wrong := problemBanner([]projects.Status{{
		Tool:          "node",
		Checked:       true,
		Installations: []inventory.Installation{installed, terminal},
		Terminal:      &terminal,
		Checks:        []projects.Check{{Tool: "node", Want: "18", File: ".nvmrc", Installed: &installed}},
	}})
	if wrong.Title != "Terminal runs Node v22.12.0 instead of 18" {
		t.Errorf("wrong-active banner title = %q", wrong.Title)
	}
	if wrong.Meta != "Required by .nvmrc · 18 is already installed" {
		t.Errorf("wrong-active banner meta = %q", wrong.Meta)
	}
}

func TestToolProjectCheckPartsDeduplicates(t *testing.T) {
	pst := projects.Status{
		Tool:    "pnpm",
		Checked: true,
		Checks: []projects.Check{
			{File: "package.json", Want: ">=12.14.0"},
			{File: "package.json", Want: "12.14.0"},
			{File: "package.json", Want: "12.14.0"},
			{File: "pnpm-lock.yaml"},
		},
	}
	got := toolProjectCheckParts(pst)
	want := []string{"package.json · >=12.14.0, 12.14.0", "pnpm-lock.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProjectDetailTabsGitDiskAIAndQuickActions(t *testing.T) {
	dir := "/Users/alice/code/moduix"
	var openedEditor, openedTerminal string
	a := NewApp()
	a.Home = "/Users/alice"
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Installations: []inventory.Installation{{Path: "/usr/local/bin/claude"}}},
			{ID: "codex", Installations: []inventory.Installation{{Path: "/usr/local/bin/codex"}}},
		},
		Tools: []inventory.ToolState{
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Servers: []inventory.Database{
						{Kind: "PostgreSQL", Status: "stopped", Source: "brew", Service: "postgresql@16"},
						{Kind: "Redis", Status: "stopped", Source: "brew", Service: "redis"},
					},
					Listeners: []inventory.DatabaseListener{
						{Port: "5432", PID: "99", Process: "other-proc", Address: "127.0.0.1"},
					},
				},
			},
		},
	}
	a.Sizes = &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			filepath.Join(dir, "node_modules"): {Bytes: 420 * 1024 * 1024},
		},
	}
	a.EditorApps = []run.App{{Name: "VS Code", Path: "/Applications/Visual Studio Code.app"}}
	a.Editor = "VS Code"
	a.OpenInEditor = func(_, _, path string, done func(error)) {
		openedEditor = path
		done(nil)
	}
	a.OpenTerminalInDir = func(d string, done func(error)) {
		openedTerminal = d
		done(nil)
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(nil, nil)
		done(p, err)
	}
	a.LoadCatalog = func(skillcatalog.Repository, bool, func([]skillcatalog.Skill, map[string]string, error)) func() {
		return func() {}
	}
	a.Projects = []projects.Project{{
		Dir:      dir,
		Readme:   "README.md",
		Installs: []string{"node_modules"},
		Files:    []string{"package.json"},
		AI:       []string{"AGENTS.md", ".agents"},
		Git: &projects.GitInfo{
			Branch:       "feat/ui",
			Dirty:        true,
			Modified:     3,
			Remote:       "github.com/acme/moduix",
			RemoteURL:    "https://github.com/acme/moduix",
			LastCommit:   "feat: tabs",
			LastCommitAt: time.Now().Add(-2 * time.Hour),
		},
		Databases: []projects.DatabaseReference{
			{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}},
			{Kind: "Redis", Service: "cache", File: "compose.yaml", Ports: []string{"6379"}},
		},
		Skills: []inventory.Skill{{
			Name:        "docs-workflow",
			Description: "Keep docs in sync",
			Project:     dir,
			Dir:         filepath.Join(dir, ".agents", "skills", "docs-workflow"),
			Paths: []inventory.SkillPath{{
				Path:   filepath.Join(dir, ".agents", "skills", "docs-workflow"),
				Agents: []string{"codex"},
			}},
		}},
		MCP: []inventory.MCPServer{{
			Name:      "github",
			Agent:     "claude",
			Transport: "stdio",
			Command:   "gh-mcp",
			Missing:   true,
		}},
		Instructions: projects.Instructions{
			Agents: projects.InstructionFile{Path: filepath.Join(dir, "AGENTS.md"), Exists: true, Text: "# Rules"},
		},
		Rules: []inventory.AgentRule{{Agent: "claude", Kind: inventory.RuleAllow, Rule: "Bash(pnpm test)", File: filepath.Join(dir, ".claude/settings.json")}},
		Hooks: []inventory.AgentHook{{Agent: "claude", Event: "PreToolUse", Command: "check.sh", File: filepath.Join(dir, ".claude/settings.json")}},
	}}
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Overview shows Git strip, Disk space card, compact AI agent count, quick actions, DB Start button, and port conflict link.
	// Prepare button is hidden when the project has no unmet requirements.
	hasTexts(t, tt, "feat/ui", "3 changes", "github.com/acme/moduix", "2 agents", "Disk space", "440 MB", locales.ServiceStart(), locales.ProjectDatabaseViewPort("5432"))
	if tt.HasText(locales.PrepareButton()) {
		t.Errorf("Prepare button should be hidden in header when project is ready")
	}

	// Quick actions: Terminal and README work with project directory.
	if err := tt.Click(locales.ProjectOpenTerminal()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if openedTerminal != dir {
		t.Errorf("openedTerminal = %q, want %q", openedTerminal, dir)
	}
	if err := tt.Click(locales.ProjectOpenReadme("README.md")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if openedEditor != filepath.Join(dir, "README.md") {
		t.Errorf("openedEditor = %q, want README path", openedEditor)
	}

	// Switch to AI tab via Manage AI: shows project skills, native badge, MCP, instructions, standalone agent permissions & hooks card, and Install skill opens catalog for this project.
	if err := tt.Click(locales.ProjectAiManage()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "docs-workflow", locales.TagNative(), "github", locales.CommandMissing(), "Agent permissions", "Open in Agents", "Claude Code", ".claude/settings.json · 1 allowed", "Claude Code · Hooks", ".claude/settings.json · 1 hook")

	if err := tt.Click(locales.ProjectSkillsInstall()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.skillCatalogOpen || a.skillCatalog == nil || a.skillCatalog.project != dir {
		t.Fatalf("Install skill did not preselect project %q: open=%v state=%+v", dir, a.skillCatalogOpen, a.skillCatalog)
	}
	a.skillCatalogOpen = false

	// Narrow window (1000x680) renders compact header buttons and port link navigates to /system/ports?filter=5432.
	a.projectTab = 0
	a.router.Push(projectPath(dir))
	narrowTT := ui.NewTester(a.View, 1000, 680)
	narrowTT.Advance(time.Second)
	hasTexts(t, narrowTT, "feat/ui", locales.ProjectDatabaseViewPort("5432"))
	if err := narrowTT.Click(locales.ProjectDatabaseViewPort("5432")); err != nil {
		t.Fatal(err)
	}
	narrowTT.Advance(time.Second)
	if got := a.router.Location(); got != portsPathWithFilter("5432") {
		t.Errorf("current route = %q, want %q", got, portsPathWithFilter("5432"))
	}
	if a.portsFilterQuery != "5432" {
		t.Errorf("portsFilterQuery = %q, want 5432", a.portsFilterQuery)
	}
}

func TestProjectsCloneDialog(t *testing.T) {
	parent := t.TempDir()
	a := NewApp()
	a.Home = parent
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{{
			Tool: "git",
			Installations: []inventory.Installation{{
				Tool:    "git",
				Version: "2.49.0",
				Path:    "/opt/homebrew/bin/git",
				OnPath:  1,
			}},
		}},
	}
	var added []string
	a.AddProjects = func(dirs []string) { added = append(added, dirs...) }
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(sysenv.Env{"HOME=" + parent, "PATH=/opt/homebrew/bin:/usr/bin"}, nil)
		done(p, err)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		update(ops.State{Phase: ops.Succeeded, Steps: []ops.StepStatus{ops.Done}, Verified: "Cloned moduix"})
		return func() {}
	}
	a.router.Push(projectsPage.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	if err := tt.Click(locales.GitCloneButton()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.GitCloneTitle(), locales.GitCloneUrlLabel())

	// Typing URL auto-populates folder name and destination preview.
	a.cloneForm.repoURL = "git@github.com:acme/moduix.git"
	tt.Advance(time.Second)
	if a.cloneForm.folder != "moduix" {
		t.Fatalf("auto folder = %q, want moduix", a.cloneForm.folder)
	}
	hasTexts(t, tt, locales.GitCloneDestination("~/moduix"))

	if err := tt.Click(locales.GitCloneAction()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil {
		t.Fatal("expected clone operation plan dialog to open")
	}
	wantTarget := filepath.Join(parent, "moduix")
	if len(a.op.plan.Steps) != 1 || a.op.plan.Steps[0].Argv[0] != "/opt/homebrew/bin/git" {
		t.Fatalf("unexpected clone plan steps: %+v", a.op.plan.Steps)
	}

	// Confirm the plan: project is added and router navigates to it.
	if err := tt.Click(locales.GitCloneConfirm()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(added) != 1 || added[0] != wantTarget {
		t.Fatalf("added projects = %v, want [%s]", added, wantTarget)
	}
	if got := a.router.Location(); got != projectPath(wantTarget) {
		t.Fatalf("route after clone = %q, want %q", got, projectPath(wantTarget))
	}

	// Close the completed operation dialog, switch to stub Git, and verify
	// that stub Git is treated as missing and builds a composite install+clone plan.
	if err := tt.Click(locales.OpDone()); err != nil {
		t.Fatal(err)
	}
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "git",
				Installations: []inventory.Installation{{
					Tool:   "git",
					Path:   "/usr/bin/git",
					OnPath: 1,
					Stub:   true,
				}},
			},
			{
				Tool: "packages",
				PackageManagers: []inventory.PackageManager{
					{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"},
					{ID: "apt", Name: "APT", Path: "/usr/bin/apt-get"},
					{ID: "winget", Name: "WinGet", Path: `C:\Windows\System32\winget.exe`},
				},
			},
		},
	}
	if path, ok := a.realGitPath(); ok {
		t.Fatalf("realGitPath() = (%q, true) for macOS stub, want false", path)
	}
	a.router.Push(projectsPage.path)
	tt.Frame()
	if err := tt.Click(locales.GitCloneButton()); err != nil {
		t.Fatal(err)
	}
	a.cloneForm.repoURL = "https://github.com/acme/demo.git"
	tt.Frame()
	hasTexts(t, tt, locales.GitCloneNoGitTitle(), locales.GitCloneInstallAction())
	if err := tt.Click(locales.GitCloneInstallAction()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.op == nil {
		t.Fatal("expected composite install+clone plan to open")
	}
	if len(a.op.plan.Steps) < 2 {
		t.Fatalf("composite plan steps = %d, want >= 2 (install git + clone)", len(a.op.plan.Steps))
	}
	last := a.op.plan.Steps[len(a.op.plan.Steps)-1]
	if len(last.Argv) < 2 || last.Argv[0] != "git" || last.Argv[1] != "clone" {
		t.Fatalf("last step = %v, want git clone", last.Argv)
	}
}
