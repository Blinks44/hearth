package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
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

func TestStartActions(t *testing.T) {
	for _, tc := range []struct {
		name, label, path string
		bare, project     bool
	}{
		{name: "choose a folder", label: "Add project", path: overview.path},
		{name: "check added projects", label: "Check a project", path: projectsPage.path, project: true},
		{name: "review issues", label: "Review issues", path: problemsPage},
		{name: "install on an existing computer", label: "Install tools", path: setupPage},
		{name: "set up a clean computer", label: "Install tools", path: setupPage, bare: true},
		{name: "set up agents", label: "Set up AI", path: "/ai/agents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApp()
			a.Snapshot, a.Counting = calmSnapshot(), true
			if tc.bare {
				a.Snapshot = &inventory.Snapshot{}
			}
			if tc.project {
				a.Projects = testProjects()[:1]
			}
			chosen, saved := 0, 0
			a.ChooseFolder = func() { chosen++ }
			a.OnSettings = func() { saved++ }
			tt := openOverview(t, a)
			if err := tt.Click(tc.label); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if a.router.Path() != tc.path {
				t.Errorf("opened %q, want %q", a.router.Path(), tc.path)
			}
			if tc.label == "Add project" && chosen != 1 {
				t.Error("did not ask for a folder")
			}
			if tc.path == setupPage && a.setup.wizard != tc.bare {
				t.Error("installation did not follow the computer's state")
			}
			if a.StartHidden || saved != 0 {
				t.Error("following a link dismissed the other introductory actions")
			}
			a.router.Push(overview.path)
			tt.Advance(time.Second)
			hasTexts(t, tt, "Getting started", "Hide tips")
		})
	}
}

func TestStartActionsDismissPersists(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.OnSettings = func() {
		s := store.DefaultSettings()
		s.StartHidden = a.StartHidden
		if err := store.SaveSettings(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	tt := openOverview(t, a)
	if err := tt.Click("Hide tips"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("Getting started") || !a.StartHidden {
		t.Fatal("introductory actions did not disappear")
	}
	s, err := store.LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewApp()
	restarted.StartHidden, restarted.Snapshot = s.StartHidden, calmSnapshot()
	tt = openOverview(t, restarted)
	if tt.HasText("Getting started") {
		t.Error("dismissed actions returned after restarting")
	}
}

func TestStartActionsDuringScan(t *testing.T) {
	a := NewApp()
	a.Scanning = true
	a.ChooseFolder = func() {}
	tt := openOverview(t, a)
	hasTexts(t, tt, "Getting started", "Results appear after the check.")
	box, ok := tt.Find("Review issues")
	if !ok {
		t.Fatal("missing diagnosis action")
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	if a.router.Path() != overview.path {
		t.Error("unavailable diagnosis opened before a snapshot existed")
	}
	a.Snapshot, a.Counting = calmSnapshot(), true
	tt.Frame()
	if err := tt.Click("Review issues"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != problemsPage {
		t.Error("counting disk space blocked diagnosis")
	}
}

func TestStartActionsAppearance(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	for _, tc := range []struct {
		name          string
		width, height int
		dark, bare    bool
	}{
		{"start-light", 1280, 820, false, false},
		{"start-dark", 1280, 820, true, false},
		{"start-narrow", 1000, 680, false, false},
		{"start-clean", 1280, 820, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApp()
			a.Snapshot = calmSnapshot()
			a.Projects = testProjects()
			a.ChooseFolder = func() {}
			if tc.bare {
				a.Snapshot, a.Projects = &inventory.Snapshot{}, nil
			}
			tt := ui.NewTesterWith(a.View, tc.width, tc.height, ui.TesterOptions{Dark: tc.dark})
			tt.Advance(time.Second)
			hasTexts(t, tt, "С чего начать", "Скрыть подсказки", "Установить инструменты", "Настроить AI")
			screenshot(t, tt, tc.name)
			project, ok := tt.Find("Проверить проект")
			if tc.bare {
				project, ok = tt.Find("Добавить проект")
			}
			ai, aiOK := tt.Find("Настроить AI")
			if !ok || !aiOK || project.X < 0 || ai.X+ai.W > float32(tc.width) {
				t.Fatal("actions do not fit the viewport")
			}
			if tc.width == 1000 && ai.Y <= project.Y {
				t.Error("narrow actions did not wrap to a second row")
			}
		})
	}
}

func TestOverviewFindProjectsExplicit(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	calls := 0
	a.FindProjects = func() { calls++ }
	tt := ui.NewTester(a.View, 1280, 1200)
	tt.Advance(time.Second)
	if calls != 0 {
		t.Fatal("the overview searched projects without an action")
	}
	if err := tt.Click("Find projects"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Error("the explicit search did not run")
	}
}

// Before the first scan the overview says it is getting to know the
// computer, with skeletons for the tools and a way to add a project.
func TestOverviewFirstScan(t *testing.T) {
	a := NewApp()
	a.Scanning = true
	a.ChooseFolder = func() {}
	tt := openOverview(t, a)
	hasTexts(t, tt, "Checking", "Getting to know this computer", "worth fixing", "counting sizes…",
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

// A computer with only what the system brings leads to the new computer
// wizard.
func TestOverviewBare(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "ruby", Installations: []inventory.Installation{
		{Tool: "ruby", Version: "2.6.10", Path: "/usr/bin/ruby", OnPath: 1, Source: inventory.Source{Kind: inventory.KindSystem}},
	}}}}
	tt := openOverview(t, a)
	screenshot(t, tt, "overview-bare")
	hasTexts(t, tt, "Ready to set up", "A clean computer", "Hearth found only what comes with the system. The Install page sets up languages, tools and AI agents in one plan.")
	if err := tt.Click("Set up this computer"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != setupPage {
		t.Errorf("the button went to %q", a.router.Location())
	}
	a.Snapshot = calmSnapshot()
	tt.Frame()
	if tt.HasText("A clean computer") {
		t.Error("a computer with Node is bare")
	}
}

// With three issues or more the hero asks for attention, the card lists
// the first three with their pages, and links to all of them.
func TestOverviewManyIssues(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	a.Snapshot.TerminalPath = []inventory.PathEntry{{Dir: filepath.Join(t.TempDir(), "gone"), Missing: true}}
	a.Projects = testProjects()
	tt := openOverview(t, a)
	screenshot(t, tt, "overview-issues")
	// fnm's third version, on no PATH, is an extra one, and the folder
	// that does not exist has no fix here: worth knowing, not counted.
	hasTexts(t, tt, "3 issues", "Needs attention", "Node: 2 copies on PATH", "Node differs outside the terminal",
		"legacy-admin needs Node 18", "All issues", "2 copies", "other in apps", "no 18", "no Node 18", "2 projects")
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
	a.router.Push(overview.path)
	tt.Advance(time.Second)
	if tt.HasText("PATH lists folders that do not exist") {
		t.Error("the card lists more than three issues")
	}
	if err := tt.Click("All issues"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != problemsPage {
		t.Errorf("All issues went to %q", a.router.Path())
	}
}

// The hero's counts open the Issues, Updates and Cleanup pages.
func TestOverviewCountsOpenPages(t *testing.T) {
	// By their captions: the sidebar's Issues comes before the hero's.
	for _, tc := range []struct{ label, path string }{
		{"worth fixing", problemsPage}, {"versions and packages", updatesPage}, {"counting sizes…", cleanupPage},
	} {
		a := NewApp()
		a.Snapshot = calmSnapshot()
		tt := openOverview(t, a)
		if err := tt.Click(tc.label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if a.router.Path() != tc.path {
			t.Errorf("%s went to %q, want %q", tc.label, a.router.Path(), tc.path)
		}
	}
}

func TestOverviewSummaryLongAmount(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	for _, width := range []int{1280, 1000} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			a := NewApp()
			a.Snapshot, a.StartHidden = calmSnapshot(), true
			a.Sizes = &inventory.Sizes{
				Caches: []inventory.Cache{{ID: "pnpm", Path: home + "/store"}},
				Paths:  map[string]inventory.Usage{home + "/store": {Bytes: 29_900_000_000}},
			}
			tt := ui.NewTester(a.View, width, 820)
			tt.Advance(time.Second)
			amount, ok := tt.Find("29,9 ГБ")
			card, cardOK := tt.Find("Можно освободить")
			if !ok || !cardOK {
				t.Fatal("missing amount or cleanup card")
			}
			if amount.X+amount.W > card.X+card.W-14-8-14 {
				t.Errorf("estimate reaches the chevron: amount=%v card=%v", amount, card)
			}
			screenshot(t, tt, "summary-long-"+strconv.Itoa(width))
		})
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

// Only warnings, and what Hearth can fix, ask for attention: tools apps
// do not find are worth knowing, as a setup through mise activate has for
// each, and wait on the Issues page.
func TestOverviewInfoIssues(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Snapshot.TerminalPath = []inventory.PathEntry{{Dir: filepath.Join(t.TempDir(), "gone"), Missing: true}, {Dir: "."}}
	ts := &a.Snapshot.Tools[0]
	for i := range ts.Installations {
		ts.Installations[i].InGUI = 0
	}
	tt := openOverview(t, a)
	hasTexts(t, tt, "1 issue", "Almost ready", "not in apps", "PATH includes the current folder")
	if tt.HasText("Apps outside the terminal do not see Node") {
		t.Errorf("an issue only worth knowing needs attention: %q", tt.Texts())
	}
}

// A narrow window shows what needs attention right under the summary,
// before the tools.
func TestOverviewNarrow(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	issues, ok := tt.Find("Needs attention")
	tools, ok2 := tt.Find("fnm · 3 versions") // Node's card
	if !ok || !ok2 {
		t.Fatalf("no issues or tools: %q", tt.Texts())
	}
	if issues.Y >= tools.Y {
		t.Errorf("issues at %v, tools at %v: want issues first", issues, tools)
	}
}

// Issues of one kind that only add up, as skills copied to several
// folders, take one line of the card and count once; Hearth merges them,
// so they need attention.
func TestOverviewGroupsIssues(t *testing.T) {
	a := NewApp()
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.SkillBackups = home + "/backups"
	a.Snapshot = calmSnapshot()
	for _, name := range []string{"pdf", "docx", "xlsx"} {
		for _, dir := range []string{"/Users/dev/.claude/skills/", "/Users/dev/.agents/skills/"} {
			a.Snapshot.Skills = append(a.Snapshot.Skills, inventory.Skill{Name: name, Dir: dir + name, Sum: name})
		}
	}
	tt := openOverview(t, a)
	hasTexts(t, tt, "1 issue", "3 skills are copied to several folders", "pdf, docx, xlsx")
	if tt.HasText("Skill pdf: 2 copies") {
		t.Errorf("a skill's copies show apart from the group: %q", tt.Texts())
	}
}

// The overview's button copies the report: the home as ~, projects by
// number, and it says so.
func TestCopyReport(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	a.Projects = testProjects()
	a.OS, a.AppVersion = "macOS 15.4 (arm64)", "0.1.0"
	tt := openOverview(t, a)
	if err := tt.Click("Copy report"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	got := tt.Clipboard()
	for _, want := range []string{"Hearth 0.1.0 · macOS 15.4 (arm64)", "## Tools", "- project 1:", "## Issues"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, home) || strings.Contains(got, "shop-web") {
		t.Errorf("personal paths in\n%s", got)
	}
	hasTexts(t, tt, "Report copied")
}

// The sidebar lists the languages by title and the sections in their own
// order, and the overview its tools cards by name, after those with issues.
func TestAlphabeticalOrder(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	for _, tool := range []string{"python", "go", "bun"} {
		a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: tool, Installations: []inventory.Installation{
			{Tool: tool, Version: "1.0.0", Path: "/usr/local/bin/" + tool, OnPath: 1, InGUI: 1, Source: inventory.Source{Kind: inventory.KindInstaller}},
		}})
	}
	tt := openOverview(t, a)
	texts := tt.Texts()
	at := func(s string) int {
		i := slices.Index(texts, s)
		if i < 0 {
			t.Fatalf("no %q in %q", s, texts)
		}
		return i
	}
	// The sidebar's, then the cards'.
	for _, order := range [][]string{
		{"Bun", "Go", "Node.js", "Python"},
		{"Overview", "Projects", "Maintenance", "Issues", "Updates", "Cleanup", "Install", "Languages & runtimes"},
		{"Infrastructure", "Ports", "AI", "Agents", "Skills", "MCP", "Instructions", "Usage", "System", "PATH & variables", "Package managers", "Not found"},
	} {
		for i := 1; i < len(order); i++ {
			if at(order[i-1]) > at(order[i]) {
				t.Errorf("%q after %q", order[i-1], order[i])
			}
		}
	}
	var cards []string
	for i, s := range texts {
		if i > 0 && texts[i-1] == s && slices.Contains([]string{"Bun", "Go", "Node.js", "Python"}, s) {
			cards = append(cards, s)
		}
	}
	if want := []string{"Bun", "Go", "Node.js", "Python"}; !slices.Equal(cards, want) {
		t.Errorf("cards %q, want %q", cards, want)
	}
}

// In Russian the sidebar keeps its sections' order; MCP servers and
// skills go by name within a group.
func TestAlphabeticalOrderRussian(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := NewApp()
	a.Locale = "ru"
	a.Snapshot = calmSnapshot()
	tt := openOverview(t, a)
	texts := tt.Texts()
	for _, pair := range [][2]string{{"Обслуживание", "Языки и рантаймы"}, {"Агенты", "Skills"}, {"Skills", "MCP"}, {"Инструкции", "Расход"}} {
		if i, j := slices.Index(texts, pair[0]), slices.Index(texts, pair[1]); i < 0 || j < 0 || i > j {
			t.Errorf("%q should come before %q: %q", pair[0], pair[1], texts)
		}
	}
	a.Locale = "en"
	a.Snapshot = aiSnapshot()
	var names []string
	for _, m := range a.mcpItems() {
		if m.project == "" {
			names = append(names, m.name)
		}
	}
	if !slices.IsSorted(names) {
		t.Errorf("servers %q", names)
	}
	var skills []string
	for _, g := range a.skillGroups() {
		for _, sk := range g.skills {
			skills = append(skills, sk.Name)
		}
	}
	if !slices.IsSorted(skills) {
		t.Errorf("skills %q", skills)
	}
}

func TestOverviewToolCardsClickable(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "docker", Installations: []inventory.Installation{{Tool: "docker", Version: "28.0.1", Path: "/usr/local/bin/docker", OnPath: 1}}},
		{Tool: "git", Installations: []inventory.Installation{{Tool: "git", Version: "2.48.0", Path: "/opt/homebrew/bin/git", OnPath: 1}}},
		{Tool: "cpp", Installations: []inventory.Installation{{Tool: "cpp", Version: "16.0.0", Path: "/usr/bin/clang", OnPath: 1}}},
	}}
	// Every tool that populates Installations must route to a page.
	for _, id := range []string{"node", "bun", "python", "php", "cpp", "rust", "go", "java", "ruby", "dotnet", "npm", "pnpm", "yarn", "corepack", "git", "docker"} {
		if _, path := a.toolCardTarget(id); path == "" {
			t.Errorf("toolCardTarget(%q) returned empty path", id)
		}
	}
	tt := openOverview(t, a)
	if err := tt.Click("Docker"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != dockerPage {
		t.Errorf("clicking Docker card went to %q, want %q", a.router.Path(), dockerPage)
	}
}

func TestOverviewAttentionMore(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	legacy := testProjects()[1]
	for i := range 5 {
		p := legacy
		p.Dir = home + "/work/app-" + strconv.Itoa(i)
		a.Projects = append(a.Projects, p)
	}
	tt := openOverview(t, a)
	hasTexts(t, tt, "And 2 more")
	if err := tt.Click("And 2 more"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != problemsPage {
		t.Errorf("clicking And 2 more went to %q, want %q", a.router.Path(), problemsPage)
	}
}

func TestIssueKeyUniqueness(t *testing.T) {
	list := []issues.Issue{
		{Kind: issues.ProjectMissing, Tool: "node", Project: "/p", Check: projects.Check{File: ".nvmrc", Want: "18"}},
		{Kind: issues.ProjectMissing, Tool: "node", Project: "/p", Check: projects.Check{File: "package.json", Want: "20"}},
		{Kind: issues.SkillCopies, Skills: []inventory.Skill{{Name: "review"}}},
		{Kind: issues.SkillCopies, Skills: []inventory.Skill{{Name: "testing"}}},
		{Kind: issues.MCPMissing, MCP: []inventory.MCPServer{{Agent: "claude", Name: "db", Command: "uvx"}}},
		{Kind: issues.MCPMissing, MCP: []inventory.MCPServer{{Agent: "claude", Name: "git", Command: "npx"}}},
		{Kind: issues.PlainSecret, MCP: []inventory.MCPServer{{Agent: "claude", Name: "s1"}}},
		{Kind: issues.PlainSecret, MCP: []inventory.MCPServer{{Agent: "claude", Name: "s2"}}},
		{Kind: issues.PlainSecret}, // must not panic on empty MCP slice
		{Kind: issues.SlowStartup, Startup: &inventory.ShellStartup{Shell: "zsh"}},
		{Kind: issues.SlowStartup, Startup: &inventory.ShellStartup{Shell: "bash"}},
		{Kind: issues.PathMissing, Dirs: []string{"/a"}},
		{Kind: issues.PathMissing, Dirs: []string{"/b"}},
	}
	seen := map[string]int{}
	for i, is := range list {
		k := issueKey(is)
		if prev, dup := seen[k]; dup {
			t.Errorf("duplicate issueKey %q for items %d and %d", k, prev, i)
		}
		seen[k] = i
	}
}

func TestOverviewLiveSummary(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	liveSnap := func() *inventory.Snapshot {
		return &inventory.Snapshot{
			Tools: []inventory.ToolState{
				{Tool: "node", Installations: []inventory.Installation{{Tool: "node", Version: "22.12.0", Path: "/opt/homebrew/bin/node", OnPath: 1, InGUI: 1}}},
				{Tool: "docker", Installations: []inventory.Installation{{Tool: "docker", Version: "28.0.1", Path: "/usr/local/bin/docker", OnPath: 1, InGUI: 1}},
					Docker: &inventory.DockerState{
						Selected: "orbstack", Running: 4, Containers: 7,
						Contexts: []inventory.DockerContext{{Name: "orbstack", Engine: "OrbStack", Endpoint: "unix:///tmp/orbstack.sock", Local: true, Status: "available"}},
					}},
				{Tool: "cli", Cli: &inventory.CliState{
					Tools: []inventory.Installation{{Tool: "kubectl", Version: "1.34.1", Path: "/opt/homebrew/bin/kubectl", OnPath: 1}},
					KubeContexts: []inventory.KubeContext{
						{Name: "staging-eu", Cluster: "staging-eu", Namespace: "default", File: home + "/.kube/config"},
						{Name: "dev-local", Cluster: "dev-local", Namespace: "app", File: home + "/.kube/config"},
					},
					Contexts: []inventory.CliContext{{Tool: "kubectl", Name: "staging-eu", File: home + "/.kube/config"}},
				}},
				{Tool: "databases", Databases: &inventory.DatabaseState{
					Servers: []inventory.Database{
						{Kind: "PostgreSQL", Version: "17.2", Status: "running", Ports: []string{"5432"}},
						{Kind: "Redis", Version: "7.4", Status: "running", Ports: []string{"6379"}},
					},
					Listeners: []inventory.DatabaseListener{
						{Port: "5432", Process: "postgres", PID: "101"},
						{Port: "6379", Process: "redis-server", PID: "102"},
						{Port: "3000", Process: "node", PID: "103"},
					},
				}},
			},
			Sessions: []inventory.AgentSession{
				{Agent: "claude", ID: "s1", Usage: []inventory.TokenUsage{{Day: today, Model: "claude-sonnet-4-5-20250929", Input: 120_000, Output: 24_000}}},
			},
			MCP: []inventory.MCPServer{
				{Name: "postgres", Agent: "claude", Transport: "stdio", Command: "npx"},
				{Name: "github", Agent: "cursor", Transport: "stdio", Command: "npx"},
				{Name: "broken", Agent: "claude", Transport: "stdio", Command: "missing-bin", Missing: true},
			},
		}
	}

	t.Run("all six tiles render and navigate", func(t *testing.T) {
		a := NewApp()
		a.StartHidden = true
		a.Snapshot = liveSnap()
		tt := openOverview(t, a)
		hasTexts(t, tt,
			"Right now",
			"4 running", "OrbStack · 7 total",
			"staging-eu", "default · 2 contexts",
			"Databases", "2 running", "PostgreSQL, Redis",
			"Ports", "3 ports", "TCP ports",
			"AI today", "144.0K tokens",
			"3 servers", "2 agents · 1 issue",
		)
		for _, tc := range []struct{ click, want string }{
			{"4 running", dockerPage},
			{"staging-eu", kubePage},
			{"2 running", "/tools/databases"},
			{"3 ports", portsPage},
			{"AI today", usagePage},
			{"3 servers", mcpPage},
		} {
			a.router.Push(overview.path)
			tt.Advance(time.Second)
			if err := tt.Click(tc.click); err != nil {
				t.Fatalf("click %q: %v", tc.click, err)
			}
			tt.Advance(time.Second)
			if a.router.Path() != tc.want {
				t.Errorf("click %q went to %q, want %q", tc.click, a.router.Path(), tc.want)
			}
		}
	})

	t.Run("ports-only and stopped docker and month fallback", func(t *testing.T) {
		a := NewApp()
		a.StartHidden = true
		yesterday := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
		a.Snapshot = &inventory.Snapshot{
			Tools: []inventory.ToolState{
				{Tool: "docker", Installations: []inventory.Installation{{Tool: "docker", Version: "28.0.1", Path: "/usr/local/bin/docker", OnPath: 1}},
					Docker: &inventory.DockerState{
						Selected: "colima",
						Contexts: []inventory.DockerContext{{Name: "colima", Engine: "Colima", Endpoint: "unix:///tmp/colima.sock", Local: true, Status: "unavailable"}},
					}},
				{Tool: "cli", Cli: &inventory.CliState{
					Tools: []inventory.Installation{{Tool: "kubectl", Version: "1.34.1", Path: "/opt/homebrew/bin/kubectl", OnPath: 1}},
				}},
				{Tool: "databases", Databases: &inventory.DatabaseState{
					Listeners: []inventory.DatabaseListener{{Port: "3000", Process: "node", PID: "103"}},
				}},
			},
			Sessions: []inventory.AgentSession{
				{Agent: "claude", ID: "s1", Usage: []inventory.TokenUsage{{Day: yesterday, Model: "claude-sonnet-4-5-20250929", Input: 50_000, Output: 5_000}}},
			},
		}
		tt := openOverview(t, a)
		hasTexts(t, tt, "Stopped", "Colima", "No context", "1 port", "in 30d")
		if err := tt.Click("1 port"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if a.router.Path() != portsPage {
			t.Errorf("ports tile went to %q, want %q", a.router.Path(), portsPage)
		}
	})

	t.Run("hidden on bare computer and first scan", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = &inventory.Snapshot{}
		tt := openOverview(t, a)
		if tt.HasText("Right now") {
			t.Error("live summary should not appear on a bare computer")
		}
	})

	t.Run("ru screenshots wide and narrow", func(t *testing.T) {
		i18n.SetLocaleFunc(func() string { return "ru" })
		t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
		for _, tc := range []struct {
			name          string
			width, height int
		}{
			{"overview-live", 1280, 820},
			{"overview-live-narrow", 1000, 680},
		} {
			a := NewApp()
			a.StartHidden = true
			a.Home = home
			a.Snapshot = liveSnap()
			tt := ui.NewTester(a.View, tc.width, tc.height)
			tt.Advance(time.Second)
			screenshot(t, tt, tc.name)
		}
	})
}

func TestOverviewProjectsCompactCard(t *testing.T) {
	a := NewApp()
	a.StartHidden = true
	a.Snapshot = calmSnapshot()
	a.ChooseFolder = func() {}
	// 8 projects: 2 problematic (need Node 18) placed at the end of input;
	// Overview must lift problematic ones first, cap at 4 compact rows in the
	// right column so Tools stay immediately visible in the main column, and
	// link the remaining 4 via the footer.
	okProj := testProjects()[0]
	badProj := testProjects()[1]
	for i := range 6 {
		p := okProj
		p.Dir = home + "/dev/ok-" + strconv.Itoa(i)
		a.Projects = append(a.Projects, p)
	}
	for i := range 2 {
		p := badProj
		p.Dir = home + "/work/bad-" + strconv.Itoa(i)
		a.Projects = append(a.Projects, p)
	}
	tt := openOverview(t, a)
	hasTexts(t, tt, "Projects", "All projects", "bad-0", "bad-1", "ok-0", "ok-1", "And 4 more")
	if tt.HasText("ok-2") || tt.HasText("ok-5") {
		t.Fatalf("overview showed more than 4 projects in compact card: %q", tt.Texts())
	}
	// Check side-column placement: Tools sit in the main column on the left,
	// Projects rows stack vertically in the right column to the right of Tools.
	toolsRect, okTools := tt.Find("Tools")
	b0, ok0 := tt.Find("bad-0")
	b1, ok1 := tt.Find("bad-1")
	if !okTools || !ok0 || !ok1 {
		t.Fatalf("missing rects: tools=%v b0=%v b1=%v", okTools, ok0, ok1)
	}
	if b0.X <= toolsRect.X+200 {
		t.Errorf("expected Projects card (%+v) in the right column to the right of Tools (%+v)", b0, toolsRect)
	}
	if b1.Y <= b0.Y || b1.X != b0.X {
		t.Errorf("expected bad-1 (%+v) stacked directly under bad-0 (%+v)", b1, b0)
	}
	// Clicking "And 4 more" navigates to /projects.
	tt.SetSize(1280, 960)
	tt.Advance(time.Second)
	if err := tt.Click("And 4 more"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != projectsPage.path {
		t.Errorf("projects overflow went to %q, want %q", a.router.Path(), projectsPage.path)
	}

	if os.Getenv("HEARTH_SCREENSHOTS") != "" {
		i18n.SetLocaleFunc(func() string { return "ru" })
		defer i18n.SetLocaleFunc(nil)
		today := time.Now().Format("2006-01-02")

		// 1. Realistic 5 live tiles + 1 project + Tools + Attention + AI agents.
		a1 := NewApp()
		a1.StartHidden = true
		a1.Home = home
		a1.ChooseFolder = func() {}
		a1.Snapshot = &inventory.Snapshot{
			Tools: []inventory.ToolState{
				calmSnapshot().Tools[0],
				{Tool: "go", Installations: []inventory.Installation{{Tool: "go", Version: "1.27.0", Path: "/opt/homebrew/bin/go", OnPath: 1, InGUI: 1}}},
				{Tool: "python", Installations: []inventory.Installation{{Tool: "python", Version: "3.13.2", Path: "/opt/homebrew/bin/python3", OnPath: 1, InGUI: 1}}},
				{Tool: "docker", Installations: []inventory.Installation{{Tool: "docker", Version: "29.8.2", Path: "/usr/local/bin/docker", OnPath: 1, InGUI: 1}},
					Docker: &inventory.DockerState{Selected: "orbstack", Running: 0, Containers: 10, Contexts: []inventory.DockerContext{{Name: "orbstack", Engine: "OrbStack", Endpoint: "unix:///tmp/orbstack.sock", Local: true, Status: "available"}}}},
				{Tool: "cli", Cli: &inventory.CliState{
					Tools: []inventory.Installation{{Tool: "kubectl", Version: "1.34.1", Path: "/opt/homebrew/bin/kubectl", OnPath: 1}},
					KubeContexts: []inventory.KubeContext{
						{Name: "staging-n3", Cluster: "staging-n3", Namespace: "digital", File: home + "/.kube/config"},
						{Name: "dev-local", Cluster: "dev-local", Namespace: "default", File: home + "/.kube/config"},
					},
					Contexts: []inventory.CliContext{{Tool: "kubectl", Name: "staging-n3", File: home + "/.kube/config"}},
				}},
				{Tool: "databases", Databases: &inventory.DatabaseState{
					Servers:   []inventory.Database{{Kind: "PostgreSQL", Version: "17.2", Status: "stopped", Ports: []string{"5432"}}},
					Listeners: []inventory.DatabaseListener{{Port: "3000", Process: "node", PID: "101"}},
				}},
			},
			Agents: []inventory.Agent{
				{ID: "claude", Installations: []inventory.Installation{{Tool: "claude", Version: "2.1.311", Path: "/opt/homebrew/bin/claude", OnPath: 1}}},
				{ID: "codex", Installations: []inventory.Installation{{Tool: "codex", Version: "0.171.0", Path: "/opt/homebrew/bin/codex", OnPath: 1}}},
			},
			Skills: []inventory.Skill{
				{Name: "golang-testing", Paths: []inventory.SkillPath{{Path: home + "/.claude/skills/golang-testing", Agents: []string{"claude"}}}},
				{Name: "mygo-maintenance", Paths: []inventory.SkillPath{{Path: home + "/.claude/skills/mygo-maintenance", Agents: []string{"claude"}}}},
				{Name: "review", Paths: []inventory.SkillPath{{Path: home + "/.codex/skills/review", Agents: []string{"codex"}}}},
			},
			MCP: []inventory.MCPServer{
				{Name: "figma", Agent: "claude", Transport: "stdio", Command: "npx"},
				{Name: "github", Agent: "codex", Transport: "stdio", Command: "npx"},
			},
			Sessions: []inventory.AgentSession{
				{Agent: "claude", ID: "s1", Usage: []inventory.TokenUsage{{Day: today, Model: "claude-sonnet-4-5-20250929", Input: 200_000, Output: 50_000}}},
			},
		}
		a1.Projects = []projects.Project{{
			Dir: home + "/dev/planeta",
			Requirements: []projects.Requirement{
				{Tool: "node", Want: ">=22.13", File: "package.json"},
				{Tool: "php", Want: "~8.3.36", File: "composer.json"},
				{Tool: "composer", Want: "*", File: "composer.json"},
				{Tool: "docker", File: "compose.yaml"},
			},
		}}
		for _, sz := range []struct {
			name string
			w, h int
		}{{"overview-one-project-five-live-ru", 1280, 820}, {"overview-one-project-five-live-ru-narrow", 1000, 680}} {
			st := ui.NewTester(a1.View, sz.w, sz.h)
			st.Advance(time.Second)
			screenshot(t, st, sz.name)
		}

		// 2. Many projects (8): right column shows 4 + "И ещё 4", Tools stay at top.
		a.router.Push(overview.path)
		a.Snapshot.Agents = a1.Snapshot.Agents
		a.Snapshot.Skills = a1.Snapshot.Skills
		a.Snapshot.MCP = a1.Snapshot.MCP
		for _, sz := range []struct {
			name string
			w, h int
		}{{"overview-many-projects-ru", 1280, 820}, {"overview-many-projects-ru-narrow", 1000, 680}} {
			st := ui.NewTester(a.View, sz.w, sz.h)
			st.Advance(time.Second)
			screenshot(t, st, sz.name)
		}
	}
}

func TestLiveSummaryBalancedRows(t *testing.T) {
	makeTiles := func(n int) []liveTile {
		out := make([]liveTile, n)
		for i := range n {
			out[i] = liveTile{key: strconv.Itoa(i)}
		}
		return out
	}
	rowLens := func(rows [][]liveTile) []int {
		out := make([]int, len(rows))
		for i, r := range rows {
			out[i] = len(r)
		}
		return out
	}
	for _, tc := range []struct {
		n      int
		narrow bool
		want   []int
	}{
		{1, false, []int{1}},
		{2, false, []int{2}},
		{3, false, []int{3}},
		{4, false, []int{4}},
		{5, false, []int{5}},
		{6, false, []int{3, 3}},
		{1, true, []int{1}},
		{2, true, []int{2}},
		{3, true, []int{3}},
		{4, true, []int{2, 2}},
		{5, true, []int{3, 2}},
		{6, true, []int{2, 2, 2}},
	} {
		got := rowLens(liveSummaryRows(makeTiles(tc.n), tc.narrow))
		if !slices.Equal(got, tc.want) {
			t.Errorf("liveSummaryRows(n=%d, narrow=%t) = %v, want %v", tc.n, tc.narrow, got, tc.want)
		}
	}
}

func TestOverviewHideAndHiddenIssuesConsistency(t *testing.T) {
	a := NewApp()
	a.StartHidden = true
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{{Tool: "node", Version: "22.12.0", Path: "/usr/local/bin/node", OnPath: 1, InGUI: 1}}},
		},
		MCP: []inventory.MCPServer{
			{Name: "broken", Agent: "claude", Transport: "stdio", Command: "missing-bin", Missing: true},
		},
	}
	tt := openOverview(t, a)
	hasTexts(t, tt, "MCP: missing-bin not found", "1 agent · 1 issue", "Hide")

	// Hiding directly from Overview removes the issue from Attention, Live Summary, and sidebar.
	if err := tt.Click("Hide"); err != nil {
		t.Fatalf("click Hide on Overview: %v", err)
	}
	tt.Advance(time.Second)
	if len(a.Hidden) != 1 {
		t.Fatalf("Hidden = %q, want 1 entry", a.Hidden)
	}
	if tt.HasText("MCP: missing-bin not found") || tt.HasText("1 agent · 1 issue") {
		t.Fatalf("hidden issue still reflected on Overview: %q", tt.Texts())
	}
	hasTexts(t, tt, "Everything is in order", "No issues found")

	// Group-level key ("mcp-missing") hides even a single MCPMissing issue consistently.
	a.Hidden = []string{string(issues.MCPMissing)}
	tt.Frame()
	if tt.HasText("MCP: missing-bin not found") || tt.HasText("1 agent · 1 issue") {
		t.Fatalf("group-hidden key did not suppress single MCP issue: %q", tt.Texts())
	}
}
