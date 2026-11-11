package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
	return a
}

// The page groups the versions and the packages by manager with what
// each goes from and to; of the packages Hearth updates only those the
// user installed are checked, not dependencies, apps or those of a manager
// that does not tell, and the tabs and the filter narrow the table.
func TestUpdatesPage(t *testing.T) {
	a := updatesApp()
	a.router.Push(updatesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "updates")
	hasTexts(t, tt, "VERSIONS · FIXES IN THE SAME LINE", "Go", "mise · a fix in the same line", "1.26.5 → 1.26.9",
		"VERSIONS · NEW LINES", "installs alongside, the default stays", "1.27.4",
		"HOMEBREW · FORMULAS", "git", "installed by you", "2.47.1 → 2.48.1", "jq", "a dependency of other packages",
		"HOMEBREW · APPS", "orbstack", "cask · the app closes while it updates",
		"APT", "curl", "8.5.0 → 8.5.1", "NIX", "Hearth does not update through Nix",
		"Found", "7", "2 versions · 5 packages", "Selected", "Update selected · 1", "2 h ago")
	if tt.HasText("wget") {
		t.Error("a package without an update shows")
	}
	if err := tt.Click("orbstack"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Update selected · 2")
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
	hasTexts(t, tt, "versions and packages", "7")
}

// The header's button updates the chosen packages in one plan, through
// each one's manager in turn.
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
	if err := tt.Click("Update selected · 3"); err != nil {
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
	want := [][]string{{"/opt/homebrew/bin/brew", "upgrade", "--formula", "git", "jq"}, {"/usr/bin/apt-get", "install", "--only-upgrade", "-y", "curl"}}
	if p.Title != "Update through 2 managers" || !slices.EqualFunc(argvs, want, slices.Equal) {
		t.Errorf("plan %q: %q", p.Title, argvs)
	}
	if !a.updateChosen(updateRow{key: "/usr/bin/apt\x00curl\x00package\x00\x00", choosable: true}) {
		t.Error("the choice is lost with the plan")
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
	hasTexts(t, tt, "Обновления", "2 версии · 5 пакетов", "Обновить выбранные · 1", "ВЕРСИИ · ИСПРАВЛЕНИЯ В ТОЙ ЖЕ ЛИНИИ")
}
