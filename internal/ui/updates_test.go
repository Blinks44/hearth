package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/updates"
	"github.com/Blinks44/hearth/internal/versions"
)

// updatesApp is a computer with the design's kinds of updates: Go from
// mise a fix and a line behind, Homebrew's formulas and an app, APT's
// package and Nix's, which Hearth does not update.
func updatesApp() *App {
	a := NewApp()
	a.Home = home
	goCopy := inventory.Installation{Tool: "go", Version: "1.26.5", Path: home + "/.local/share/mise/installs/go/1.26.5/bin/go", OnPath: 1, Default: true,
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: home + "/.local/share/mise/installs/go/1.26.5"}}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "go", Installations: []inventory.Installation{goCopy}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", UpdatesKnown: true, ReasonsKnown: true,
				UpdatesFetched: time.Now().Add(-2 * time.Hour), Packages: []inventory.SystemPackage{
					{Name: "git", Version: "2.47.1", Available: "2.48.1", Kind: "formula"},
					{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula", Dependency: true},
					{Name: "wget", Version: "1.25.0", Kind: "formula"},
					{Name: "orbstack", Version: "1.9.2", Available: "1.10.0", Kind: "cask"},
					{Name: "cursor", Version: "3.29.9,306d153d51d714882ab7b1bc49649cab624e8a31", Available: "3.29.15,86776fcb52ca50578315563108", Kind: "cask"},
				}},
			{ID: "apt", Name: "APT", Path: "/usr/bin/apt", UpdatesKnown: true, Packages: []inventory.SystemPackage{
				{Name: "curl", Version: "8.5.0", Available: "8.5.1", Kind: "package"},
			}},
			{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env", UpdatesKnown: true, Packages: []inventory.SystemPackage{
				{Name: "hello", Version: "2.12", Available: "2.12.1", Kind: "profile"},
			}},
		}},
	}}
	a.GoVersions = &versions.Go{Releases: []string{"1.27.4", "1.26.9"}}
	a.MiseOutdated = &versions.Mise{Tools: map[string]versions.MiseTool{"go": {Current: "1.26.5", InRange: "1.26.9", Latest: "1.27.4"}}}
	return a
}

// The page groups the versions and the packages by manager with what
// each goes from and to; the fixes are checked and the new lines not, and
// of the packages Hearth updates only those the user installed, not
// dependencies, apps or those of a manager that does not tell; the tabs
// and the filter narrow the table.
func TestUpdatesPage(t *testing.T) {
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "updates")
	hasTexts(t, tt, "VERSIONS · FIXES IN THE SAME LINE", "Go", "mise · a fix in the same line", "1.26.5 → 1.26.9",
		"VERSIONS · NEW LINES", "installs alongside, the default stays", "1.27.4",
		"HOMEBREW · FORMULAS", "git", "installed by you", "2.47.1 → 2.48.1", "jq", "a dependency of other packages",
		"HOMEBREW · APPS", "orbstack", "cask · the app closes while it updates", "cursor", "3.29.9 → 3.29.15",
		"APT", "curl", "8.5.0 → 8.5.1", "NIX", "Hearth does not update through Nix",
		"Found", "8", "2 versions · 6 packages", "Selected", "Update selected · 2", "2 h ago")
	if tt.HasText("wget") {
		t.Error("a package without an update shows")
	}
	if err := tt.Click("orbstack"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 3")
	a.updatesTab = 2 // packages
	tt.Frame()
	if tt.HasText("1.26.5 → 1.26.9") || !tt.HasText("curl") {
		t.Errorf("Packages tab: %q", tt.Texts())
	}
	a.updatesTab, a.updatesFilter = 0, "jq"
	tt.Frame()
	if tt.HasText("curl") || !tt.HasText("1.7.1 → 1.8.2") {
		t.Errorf("filter: %q", tt.Texts())
	}
}

// Before the first scan the page waits for it; with nothing newer it says
// so, and the sidebar shows no count.
func TestUpdatesWaitingAndEmpty(t *testing.T) {
	a := NewApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Updates show after the first check.")
	a.Snapshot = calmSnapshot()
	tt.Frame()
	hasTexts(t, tt, "Everything is up to date.")
	if tt.HasText("Update selected · 0") {
		t.Error("a button without updates")
	}
}

// The overview counts versions and packages both, as the sidebar does.
func TestUpdatesCounted(t *testing.T) {
	a := updatesApp()
	tt := openOverview(t, a)
	hasTexts(t, tt, "versions and packages", "8")
}

// The header's button updates the chosen versions and packages in one
// plan, through each one's manager in turn: mise moves its default Go
// within the config's range.
func TestUpdatesRun(t *testing.T) {
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	for _, row := range []string{"jq", "curl"} {
		if err := tt.Click(row); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	if err := tt.Click("Update selected · 4"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil {
		t.Fatal("no plan proposed")
	}
	p := a.op.plan
	var argvs [][]string
	for _, s := range p.Steps {
		argvs = append(argvs, s.Shown())
	}
	want := [][]string{
		{"mise", "upgrade", "--prune", "go"},
		{"/opt/homebrew/bin/brew", "upgrade", "--formula", "git", "jq"}, {"/usr/bin/apt-get", "update", "&&", "/usr/bin/apt-get", "install", "--only-upgrade", "-y", "curl"},
	}
	if p.Title != "Update the chosen · 4" || !slices.EqualFunc(argvs, want, slices.Equal) {
		t.Errorf("plan %q: %q", p.Title, argvs)
	}
	if !a.updateChosen(updateRow{key: "/usr/bin/apt\x00curl\x00package\x00\x00", choosable: true}) {
		t.Error("the choice is lost with the plan")
	}
}

// A copy a package manager keeps is its package's to update; mise's own
// tools update in place, and their new versions go in beside; another
// copy of mise's is replaced, and its default only moves within the range
// mise outdated sees.
func TestUpdatesHeldAndMise(t *testing.T) {
	a := updatesApp()
	st := &a.Snapshot.Tools[0]
	st.Installations = append(st.Installations, inventory.Installation{Tool: "go", Version: "1.26.5", Path: "/opt/homebrew/bin/go",
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/go/1.26.5"}})
	old := inventory.Installation{Tool: "go", Version: "1.26.2", Path: home + "/.local/share/mise/installs/go/1.26.2/bin/go",
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: home + "/.local/share/mise/installs/go/1.26.2", Version: "1.26.2"}}
	st.Installations = append(st.Installations, old)
	jq := inventory.Installation{Tool: "jq", Version: "1.7.1", Path: home + "/.local/share/mise/installs/jq/1.7.1/bin/jq",
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: home + "/.local/share/mise/installs/jq/1.7.1", Version: "1.7.1"}}
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "mise", MiseTools: []inventory.MiseTool{{Name: "jq", Installations: []inventory.Installation{jq}}}})
	a.MiseOutdated.Tools["jq"] = versions.MiseTool{Current: "1.7.1", InRange: "1.7.2", Latest: "1.8.2"}
	var c *ui.Context
	tt := ui.NewTester(func(ctx *ui.Context) { c = ctx; a.View(ctx) }, 1280, 820)
	tt.Frame()
	var fixes, lines []string
	for _, g := range a.updateGroups(c) {
		for _, r := range g.rows {
			if r.version == nil {
				continue
			}
			p, ok := a.updatePlan(*r.version)
			if !ok {
				t.Errorf("%s %s: no plan", r.title, r.to)
				continue
			}
			argv := strings.Join(p.Steps[0].Shown(), " ")
			if r.from != "" {
				fixes = append(fixes, argv)
			} else {
				lines = append(lines, argv)
			}
		}
	}
	// Homebrew's Go is not a version to fix: the package's row updates it.
	if want := []string{"mise upgrade --prune go", "mise install go@1.26.9", "mise upgrade --prune jq"}; !slices.Equal(fixes, want) {
		t.Errorf("fixes %q, want %q", fixes, want)
	}
	if want := []string{"mise install go@1.27.4", "mise install jq@1.8.2"}; !slices.Equal(lines, want) {
		t.Errorf("new lines %q, want %q", lines, want)
	}
	// mise use -g would pin a range mise does not move to the release.
	a.MiseOutdated.Tools["go"] = versions.MiseTool{Current: "1.26.5", Latest: "1.27.4"}
	if p, ok := a.updatePlan(updates.Update{Kind: updates.Patch, Tool: "go", Copy: st.Installations[0], Version: "1.26.9"}); ok {
		t.Errorf("the default Go outside mise's range: %q", p.Steps[0].Argv)
	}
}

// A new line of a tool no version manager keeps, as Node from Homebrew,
// goes in through mise, set up first, instead of a row with nothing to do.
func TestUpdatesLineWithoutManager(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		{Tool: "node", Version: "24.22.2", Path: "/opt/homebrew/bin/node", OnPath: 1, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
	}}}}
	u := updates.Update{Kind: updates.NewLine, Tool: "node", Version: "26.13.1", LTS: "26"}
	if _, ok := a.updatePlan(u); ok {
		t.Error("a plan without a way to set mise up")
	}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	p, ok := a.updatePlan(u)
	if !ok || !a.newMise(u) {
		t.Fatalf("plan %v, mise first %v", ok, a.newMise(u))
	}
	if got := strings.Join(p.Steps[0].Shown(), " "); got != "mise install node@26.13.1" {
		t.Errorf("step %q", got)
	}
}

// The note of global packages is for those the user put in: Node's npm
// and Corepack come with any version.
func TestGlobals(t *testing.T) {
	node := func(names ...string) inventory.Installation {
		in := inventory.Installation{Tool: "node"}
		for _, n := range names {
			in.Packages = append(in.Packages, inventory.Package{Name: n})
		}
		return in
	}
	for _, tc := range []struct {
		in   inventory.Installation
		want bool
	}{
		{node("npm", "corepack"), false},
		{node("npm", "corepack", "typescript"), true},
		{inventory.Installation{Tool: "ruby"}, true},
		{inventory.Installation{Tool: "go", Packages: []inventory.Package{{Name: "gopls"}}}, false},
	} {
		if got := globals(tc.in); got != tc.want {
			t.Errorf("globals(%s %v) = %v", tc.in.Tool, tc.in.Packages, got)
		}
	}
}

// In Russian the counts take their plural forms.
func TestUpdatesRussian(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "updates-narrow-ru")
	hasTexts(t, tt, "Обновления", "2 версии · 6 пакетов", "Обновить выбранные · 2", "ВЕРСИИ · ИСПРАВЛЕНИЯ В ТОЙ ЖЕ ЛИНИИ")
}

// A Flatpak commit, standing for a version there is none of, shows as a
// build; versions and other managers' strings stay as they are.
func TestFlatpakBuild(t *testing.T) {
	for _, tt := range []struct{ manager, v, want string }{
		{"flatpak", "40a4a730925d", "build 40a4a730925d"},
		{"flatpak", "51.1", "51.1"},
		{"apt", "40a4a730925d", "40a4a730925d"},
	} {
		if got := flatpakBuild(tt.manager, tt.v); got != tt.want {
			t.Errorf("%s %s: %q, want %q", tt.manager, tt.v, got, tt.want)
		}
	}
}

func TestUpdatesGroupSelection(t *testing.T) {
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 1100)
	tt.Advance(time.Second)
	if err := tt.Click("Select group: Homebrew · apps"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 4")
	if err := tt.Click("orbstack"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 3")
	if err := tt.Click("Select group: Homebrew · apps"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 4")
	if err := tt.Click("Select group: Homebrew · apps"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 2")
	a.updatesFilter = "cursor"
	tt.Frame()
	if err := tt.Click("Select group: Homebrew · apps"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	a.updatesFilter = ""
	tt.Frame()
	hasTexts(t, tt, "Update selected · 3")
	screenshot(t, tt, "updates-groups")
}

func TestUpdatesStatusStates(t *testing.T) {
	t.Run("checking", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = calmSnapshot()
		a.VersionChecks = map[string]bool{"node": true}
		a.router.Push(updatesPage)
		tt := ui.NewTester(a.View, 1280, 820)
		tt.Advance(time.Second)
		hasTexts(t, tt, "Checking for updates…", "Checking…")
		if tt.HasText("Everything is up to date.") {
			t.Errorf("showed up-to-date while checking: %q", tt.Texts())
		}
	})
	t.Run("offline", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = calmSnapshot()
		a.VersionLists = false
		a.router.Push(updatesPage)
		tt := ui.NewTester(a.View, 1280, 820)
		tt.Advance(time.Second)
		hasTexts(t, tt, "Online update checks are turned off in Settings.")
		if tt.HasText("Everything is up to date.") {
			t.Errorf("showed up-to-date while offline: %q", tt.Texts())
		}
	})
	t.Run("errors", func(t *testing.T) {
		a := NewApp()
		a.Snapshot = calmSnapshot()
		a.NodeVersionsErr = "could not reach nodejs.org"
		a.router.Push(updatesPage)
		tt := ui.NewTester(a.View, 1280, 820)
		tt.Advance(time.Second)
		hasTexts(t, tt, "Node.js: could not reach nodejs.org", "Could not check all sources for updates.")
		if tt.HasText("Everything is up to date.") {
			t.Errorf("showed up-to-date despite check error: %q", tt.Texts())
		}
	})
}

func TestUpdatesCheckNowAndJSAgents(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{{
			Tool: "pnpm",
			Installations: []inventory.Installation{{
				Tool: "pnpm", Version: "9.12.0", Path: "/corepack/pnpm", OnPath: 1, Default: true,
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "corepack"},
			}},
		}},
		Agents: []inventory.Agent{{
			ID: "claude",
			Installations: []inventory.Installation{{
				Tool: "claude", Version: "2.1.0",
				Path:     "/Users/me/.npm-global/bin/claude",
				RealPath: "/Users/me/.npm-global/lib/node_modules/@anthropic-ai/claude-code/cli.js",
				OnPath:   1,
				Source:   inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm"},
			}},
		}},
	}
	a.JSVersions = &versions.JSManagers{Pnpm: versions.Latest{Version: "9.15.0"}}
	a.AgentVersions = &versions.Agents{Latest: map[string]versions.Latest{"claude": {Version: "2.2.0"}}}
	checked := 0
	a.CheckUpdates = func() { checked++ }
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	hasTexts(t, tt, "Check updates now", "pnpm", "9.12.0 → 9.15.0", "Claude Code", "2.1.0 → 2.2.0", "Update selected · 2")
	screenshot(t, tt, "s13-04-updates-1280")
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "s13-04-updates-1000")
	if err := tt.Click("Check updates now"); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("CheckUpdates called %d times, want 1", checked)
	}
	if err := tt.Click("Update selected · 2"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("expected proposed update plan: %+v", a.op)
	}
	// Corepack update is a single step (no duplicate default step) + npm update for Claude Code.
	if len(a.op.plan.Steps) != 2 ||
		!slices.Equal(a.op.plan.Steps[0].Argv, []string{"corepack", "install", "-g", "pnpm@9.15.0"}) ||
		!slices.Equal(a.op.plan.Steps[1].Argv, []string{"npm", "install", "--global", "--prefix", filepath.FromSlash("/Users/me/.npm-global"), "@anthropic-ai/claude-code@latest"}) {
		t.Fatalf("steps = %+v", a.op.plan.Steps)
	}
}

func TestUpdatesFilterVersionManagerAndVisibleOnly(t *testing.T) {
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Search by target version matches Go 1.26.9 and excludes packages.
	a.updatesFilter = "1.26.9"
	tt.Frame()
	if !tt.HasText("1.26.5 → 1.26.9") || tt.HasText("git") {
		t.Fatalf("version filter 1.26.9: %q", tt.Texts())
	}
	hasTexts(t, tt, "Update selected · 1")

	// Search by manager name matches Homebrew packages only.
	a.updatesFilter = "homebrew"
	tt.Frame()
	if !tt.HasText("git") || tt.HasText("curl") || tt.HasText("1.26.5 → 1.26.9") {
		t.Fatalf("manager filter homebrew: %q", tt.Texts())
	}
	// Clicking Update selected under filter updates only visible chosen rows (git), not hidden Go.
	if err := tt.Click("Update selected · 1"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("expected filtered plan, got %+v", a.op)
	}
	if len(a.op.plan.Steps) != 1 || !slices.Equal(a.op.plan.Steps[0].Shown(), []string{"/opt/homebrew/bin/brew", "upgrade", "--formula", "git"}) {
		t.Fatalf("filtered plan steps = %+v", a.op.plan.Steps)
	}
	a.op = nil

	// Switching to Versions tab updates only visible Go, excluding hidden git.
	a.updatesFilter = ""
	a.updatesTab = 1
	tt.Frame()
	if err := tt.Click("Update selected · 1"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || len(a.op.plan.Steps) != 1 || !slices.Equal(a.op.plan.Steps[0].Shown(), []string{"mise", "upgrade", "--prune", "go"}) {
		t.Fatalf("versions-tab plan = %+v", a.op)
	}
}

func TestUpdatesRowNavigation(t *testing.T) {
	a := updatesApp()
	a.Snapshot.Agents = []inventory.Agent{{
		ID: "claude",
		Installations: []inventory.Installation{{
			Tool: "claude", Version: "2.1.0", Path: "/Users/me/.npm-global/bin/claude", OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm"},
		}},
	}}
	a.AgentVersions = &versions.Agents{Latest: map[string]versions.Latest{"claude": {Version: "2.2.0"}}}
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 900)
	tt.Advance(time.Second)

	// Clicking Go row navigates to /tools/go.
	if err := tt.Click("Open Go"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != "/tools/go" {
		t.Fatalf("Go row navigated to %q, want /tools/go", a.router.Location())
	}

	// Clicking Claude Code row navigates to /ai/agents/claude.
	a.router.Push(updatesPage)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Open Claude Code"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != agentPath("claude") {
		t.Fatalf("Agent row navigated to %q, want %q", a.router.Location(), agentPath("claude"))
	}

	// Clicking git package row navigates to /system/packages filtered to brew + git.
	a.router.Push(updatesPage)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Open git"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != "/system/packages" || a.packageManager != "/opt/homebrew/bin/brew" || a.packageTab != 0 || a.packageFilter != "git" {
		t.Fatalf("Package row navigated to %q mgr=%q tab=%d filter=%q", a.router.Location(), a.packageManager, a.packageTab, a.packageFilter)
	}
}

func TestUpdatesHearthAppBanner(t *testing.T) {
	a := updatesApp()
	var clicked int
	a.CheckAppUpdate = func() { clicked++ }
	a.router.Push(updatesPage)

	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if tt.HasText("Hearth 1.1.0 is available") {
		t.Fatal("app update banner must not show when AppUpdateAvailable is empty")
	}

	a.AppUpdateAvailable = "1.1.0"
	tt.Frame()
	hasTexts(t, tt, "Hearth 1.1.0 is available", "Update…")
	if err := tt.Click("Update…"); err != nil {
		t.Fatal(err)
	}
	if clicked != 1 {
		t.Fatalf("clicked = %d, want 1", clicked)
	}
}
