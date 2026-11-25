package ui

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestPackagesDirectEntryAndLanding(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(sysenv.Env{}, nil)
		done(p, err)
	}
	// Without detected managers, sidebar entry stays on /system/packages landing.
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages"}}}
	if got := a.entryPath(packagesPage); got != packagesPage {
		t.Fatalf("empty entryPath = %q, want %q", got, packagesPage)
	}

	// With Homebrew detected, sidebar entry jumps directly to Homebrew's drill-down.
	a.Snapshot.Tools[0].PackageManagers = []inventory.PackageManager{
		{
			ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew",
			Version: "Homebrew 4.4.20", UpdatesKnown: true, ReasonsKnown: true,
			Packages: []inventory.SystemPackage{
				{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula"},
				{Name: "oniguruma", Version: "6.9.9", Kind: "formula", Dependency: true},
			},
		},
		{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env", Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12", Kind: "profile"}}},
	}
	want := packagesManagerPath("/opt/homebrew/bin/brew")
	if got := a.entryPath(packagesPage); got != want {
		t.Fatalf("entryPath = %q, want %q", got, want)
	}

	// Landing page shows detected managers, quick sections, attention, and Available tab.
	a.router.Push(packagesPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages-landing")
	hasTexts(t, tt, "Package managers", "Detected managers", "Homebrew", "Nix", "Manager sections", "Available", "Open Homebrew")

	// Clicking Available tab displays platform catalog managers.
	if err := tt.Click("Available"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Available package managers")
	avail := pkgmgr.AvailableManagers()
	if len(avail) == 0 || !tt.HasText(avail[0].Name) {
		t.Fatalf("Available tab missing %+v: %q", avail, tt.Texts())
	}

	// Clicking "Open Homebrew" jumps into the drill-down view.
	if err := tt.Click("Open Homebrew"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Location() != want {
		t.Fatalf("Open Homebrew navigated to %q, want %q", a.router.Location(), want)
	}
	// Clicking back returns to /system/packages.
	if err := tt.Click("Back to Package Managers"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != packagesPage {
		t.Fatalf("back navigated to %q, want %q", a.router.Path(), packagesPage)
	}
}

func TestPackagesPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{
			ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew",
			Version: "Homebrew 4.4.20", UpdatesKnown: true, ReasonsKnown: true,
			Taps:     []string{"custom/tools"},
			Settings: []string{"HOMEBREW_NO_AUTO_UPDATE=—"},
			Packages: []inventory.SystemPackage{
				{Name: "postgresql@17", Version: "17.2", Available: "17.3", Kind: "formula", Path: "/opt/homebrew/Cellar/postgresql@17/17.2"},
				{Name: "python@3.13", Version: "3.13.1", Available: "3.13.2", Kind: "formula", Path: "/opt/homebrew/Cellar/python@3.13/3.13.1"},
				{Name: "orbstack", Version: "1.9.5", Available: "1.9.6", Kind: "cask", Path: "/opt/homebrew/Caskroom/orbstack"},
				{Name: "node", Version: "23.6.0", Kind: "formula", Path: "/opt/homebrew/Cellar/node/23.6.0"},
			},
		},
		{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env", Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12", Kind: "profile"}}},
		{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true, Packages: []inventory.SystemPackage{{Name: "Example.CLI", Version: "2.0", Kind: "portable"}}},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages")
	hasTexts(t, tt, "postgresql@17", "orbstack", "node", "Managed by Hearth")

	// Overview view shows taps and settings.
	a.pkg.view = pkgViewOverview
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "custom/tools", "HOMEBREW_NO_AUTO_UPDATE=—", "Taps")

	// Updates view filters to packages with updates only.
	a.pkg.view = pkgViewUpdates
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("node") || !tt.HasText("python@3.13") {
		t.Fatal(tt.Texts())
	}

	// Switching to Nix shows read-only badge and unknown-updates note.
	a.router.Replace(packagesManagerPath("/nix/bin/nix-env"))
	a.pkg.view = pkgViewPackages
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "hello", "read only", "Online Nix update data is not available yet. Flake versions are evaluated without building packages or changing profiles; configured channels are queried as they are.")

	// Switching to WinGet shows local-only note.
	a.router.Replace(packagesManagerPath("/winget"))
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Example.CLI", "Only locally registered portable packages are shown until an online WinGet query succeeds. Sources may require agreements you must accept outside Hearth.")
	screenshot(t, tt, "packages-winget")

	// Clearing managers leaves no stale packages shown.
	a.Snapshot.Tools[0].PackageManagers = nil
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("hello") || tt.HasText("Example.CLI") {
		t.Fatal(tt.Texts())
	}
}

func TestPackagesOnlineOverlay(t *testing.T) {
	a := NewApp()
	local := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{local}}}}
	a.PackageUpdates = map[string]inventory.PackageManager{"/winget": {ID: "winget", Name: "WinGet", Path: "/winget", UpdatesKnown: true, UpdatesFetched: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), Packages: []inventory.SystemPackage{{Name: "Example.CLI", Version: "1.0", Available: "2.0"}}}}
	a.router.Push(packagesManagerPath("/winget"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if !tt.HasText("Example.CLI") || !tt.HasText("available 2.0") {
		t.Fatal(tt.Texts())
	}
	screenshot(t, tt, "packages-online")
	// A new local scan must not continue showing a result for its predecessor.
	a.PackageUpdates = nil
	a.PackageChecks = map[string]bool{"/winget": true}
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Example.CLI") || !tt.HasText("Checking…") {
		t.Fatal(tt.Texts())
	}
}

func TestPackagesPartialSources(t *testing.T) {
	a := NewApp()
	m := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{m}}}}
	m.UpdatesFetched = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	m.UpdateError = "some package sources could not be queried"
	m.Packages = []inventory.SystemPackage{{Name: "Microsoft.OneDrive", Version: "22.1", Available: "26.1"}, {Name: "Microsoft.Teams", Version: "26.1"}}
	a.PackageUpdates = map[string]inventory.PackageManager{"/winget": m}
	a.pkg.view = pkgViewOverview
	a.router.Push(packagesManagerPath("/winget"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	for _, s := range []string{"≥ 2", "≥ 1", "not all sources answered"} {
		if !tt.HasText(s) {
			t.Errorf("no %q: %q", s, tt.Texts())
		}
	}
	if tt.HasText("Only locally registered portable packages are shown until an online WinGet query succeeds. Sources may require agreements you must accept outside Hearth.") {
		t.Error("local-only note after an online query")
	}
	if tt.HasText("Available updates have not been checked for this manager.") {
		t.Error("updates of the sources that answered count as unchecked")
	}
}

func TestPackageNameTooltip(t *testing.T) {
	long := "docker-hub.example.org/digital/platform/redis-with-modules"
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Packages: []inventory.SystemPackage{{Name: long, Version: "1.0", Kind: "formula"}}},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	at, ok := tt.Find(long)
	if !ok {
		t.Fatalf("no %s: %q", long, tt.Texts())
	}
	before := countText(tt, long)
	if before < 1 {
		t.Fatalf("%d copies of the name before resting on it", before)
	}
	tt.Move(at.X+40, at.Y+at.H/2)
	tt.Advance(time.Second)
	if n := countText(tt, long); n != before+1 {
		t.Errorf("%d copies of the name after resting on it, want %d (cell plus tooltip)", n, before+1)
	}
}

func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == s {
			n++
		}
	}
	return n
}

func TestPackageReasonsAndDependenciesAutoremove(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "node", Version: "24.0.0", Kind: "formula", Dependencies: []string{"icu4c"}},
			{Name: "icu4c", Version: "76.1", Kind: "formula", Dependency: true},
			{Name: "oniguruma", Version: "6.9.9", Kind: "formula", Dependency: true},
		}},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Installed by you", "installed by you", "needed by node", "dependency · nothing needs it any more")

	// Switch to Dependencies kind chip.
	if err := tt.Click("Dependencies"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if slices.Contains(tt.Texts(), "node") || !tt.HasText("icu4c") || !tt.HasText("oniguruma") {
		t.Errorf("dependencies view: %q", tt.Texts())
	}

	// Switch to Unused sub-tab: only oniguruma remains.
	if err := tt.Click("Unused"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("icu4c") || !tt.HasText("oniguruma") {
		t.Fatalf("unused filter failed: %q", tt.Texts())
	}

	// Clicking Autoremove… opens brew autoremove plan.
	if err := tt.Click("Autoremove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "autoremove"}) {
		t.Fatalf("autoremove plan = %+v", a.op)
	}
}

func TestPackageDependentNamesTruncation(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "app-b", Kind: "formula", Dependencies: []string{"libx"}},
			{Name: "app-a", Kind: "formula", Dependencies: []string{"libx"}},
			{Name: "app-c", Kind: "formula", Dependencies: []string{"libx"}},
			{Name: "dep-d", Kind: "formula", Dependency: true, Dependencies: []string{"libx"}},
			{Name: "libx", Kind: "formula", Dependency: true},
		}},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "needed by app-a, app-b, app-c (+1)")
}

func TestPackageSizes(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "node", Kind: "formula", Path: "/brew/node"},
			{Name: "orbstack", Kind: "cask", Path: "/brew/orbstack", Artifacts: []string{"/Applications/OrbStack.app"}},
			{Name: "icu4c", Kind: "formula", Dependency: true, Path: "/brew/icu4c"},
			{Name: "zlib", Kind: "formula", Dependency: true, Path: "/brew/zlib"},
		}},
	}}}}
	a.Sizes = &inventory.Sizes{Paths: map[string]inventory.Usage{
		"/brew/node":                 {Bytes: 74_000_000},
		"/brew/orbstack":             {Bytes: 2_000_000},
		"/Applications/OrbStack.app": {Bytes: 1_410_000_000},
		"/brew/icu4c":                {Bytes: 900, Partial: true},
	}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages-sizes")
	hasTexts(t, tt, "Size, who installed it and who needs it", "74 MB · installed by you", "1.4 GB · installed by you",
		"≥ 1 KB · dependency · nothing needs it any more", "dependency · nothing needs it any more")
	if tt.HasText("/brew/node") {
		t.Error("the path shows next to who installed it")
	}
	a.packageSort = sortBySize
	tt.Frame()
	tt.Advance(time.Second)
	var order []string
	for _, text := range tt.Texts() {
		if text == "node" || text == "orbstack" || text == "icu4c" || text == "zlib" {
			order = append(order, text)
		}
	}
	order = slices.Compact(order)
	if want := []string{"orbstack", "node", "icu4c", "zlib"}; !slices.Equal(order, want) {
		t.Errorf("by size: %q, want %q", order, want)
	}
}

func TestPackageOperationsAndBulkBar(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", UpdatesKnown: true, ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula"},
			{Name: "oniguruma", Version: "6.9.9", Kind: "formula", Dependency: true},
			{Name: "gh", Version: "2.60.0", Available: "2.63.0", Kind: "formula", Dependencies: []string{"oniguruma"}},
		}},
		{ID: "pacman", Name: "Pacman", Path: "/usr/bin/pacman", UpdatesKnown: true, Packages: []inventory.SystemPackage{
			{Name: "git", Version: "2.47.0", Available: "2.47.1", Kind: "package"},
		}},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages-actions")

	// A dependency has no menu: other packages may need it.
	brew := a.Snapshot.Tools[0].PackageManagers[0]
	if a.packageCan(brew, brew.Packages[1]) {
		t.Error("a dependency offers its removal")
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Update to 1.8.2"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "upgrade", "--formula", "jq"}) {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Switch to Updates view and select all -> floating bulk bar appears.
	a.pkg.view = pkgViewUpdates
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("oniguruma") {
		t.Fatalf("Updates view should not show up-to-date packages: %q", tt.Texts())
	}
	if err := tt.Click("Select all"); err != nil {
		t.Fatalf("click Select all: %v", err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "2 packages selected", "Upgrade 2…", "Remove 2…")
	if err := tt.Click("Upgrade 2…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "upgrade", "--formula", "jq", "gh"}) {
		t.Fatalf("bulk upgrade plan = %+v", a.op)
	}
	// Cancelling preserves selection.
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(a.packageChosen) != 2 {
		t.Fatalf("cancelling proposal cleared packageChosen: %+v", a.packageChosen)
	}
	// Bulk remove proposes uninstall for both packages.
	if err := tt.Click("Remove 2…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "uninstall", "--formula", "jq", "gh"}) {
		t.Fatalf("bulk remove plan = %+v", a.op)
	}
	a.op = nil
	a.packageChosen = nil

	// Pacman updates the whole system only.
	a.router.Replace(packagesManagerPath("/usr/bin/pacman"))
	a.pkg.view = pkgViewUpdates
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Update the whole system…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/usr/bin/pacman", "-Syu", "--noconfirm"}) {
		t.Fatalf("pacman plan = %+v", a.op)
	}
}

func TestPackageDetailSplitView(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{
			ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", UpdatesKnown: true, ReasonsKnown: true,
			Packages: []inventory.SystemPackage{
				{
					Name: "gh", Version: "2.60.0", Available: "2.63.0", Kind: "formula",
					Path: "/opt/homebrew/Cellar/gh/2.60.0", Description: "GitHub command-line tool",
					Homepage: "https://cli.github.com/", License: "MIT", Tap: "homebrew/core",
					Dependencies: []string{"oniguruma"},
				},
				{Name: "oniguruma", Version: "6.9.9", Kind: "formula", Dependency: true},
			},
		},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Clicking anywhere on a package row (e.g. its usage column) opens the split-view inspector.
	if err := tt.Click("installed by you"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := packageKey(a.Snapshot.Tools[0].PackageManagers[0].Packages[0]); a.pkg.detailKey != want {
		t.Fatalf("detailKey = %q after clicking row usage cell, want %q", a.pkg.detailKey, want)
	}
	screenshot(t, tt, "packages-detail")
	hasTexts(t, tt, "GitHub command-line tool", "homebrew/core", "MIT", "Upgrade to 2.63.0…", "Open folder", "Website")

	// Switch to Dependencies tab inside detail panel and jump to oniguruma.
	a.pkg.detailTab = 1
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "oniguruma")
	a.pkg.detailKey = packageKey(a.Snapshot.Tools[0].PackageManagers[0].Packages[1])
	a.pkg.detailTab = 2
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "gh")

	// Close button clears detailKey.
	if err := tt.Click("Close details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.pkg.detailKey != "" {
		t.Fatalf("detailKey = %q after close, want empty", a.pkg.detailKey)
	}
}

func TestPackageInstallSearchView(t *testing.T) {
	a := NewApp()
	a.Home = home
	var searched []string
	a.SearchPackages = func(m inventory.PackageManager, query, kind string, done func([]pkgmgr.SearchResult, error)) func() {
		searched = append(searched, m.ID+":"+query+":"+kind)
		if query == "fail" {
			done(nil, errors.New("network unreachable"))
			return func() {}
		}
		done([]pkgmgr.SearchResult{
			{Name: "ripgrep", Version: "14.1.1", Kind: "formula", Description: "Fast recursive grep"},
			{Name: "jq", Version: "1.8.2", Kind: "formula", Description: "JSON processor"},
		}, nil)
		return func() {}
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{
			ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", UpdatesKnown: true,
			Packages: []inventory.SystemPackage{{Name: "jq", Version: "1.7.1", Kind: "formula"}},
		},
		{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env"},
	}}}}
	a.pkg.view = pkgViewInstall
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Must never query before explicit Search action.
	a.pkg.searchQuery = "ripgrep"
	tt.Frame()
	if len(searched) != 0 {
		t.Fatalf("searched while typing: %v", searched)
	}
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(searched) != 1 || searched[0] != "brew:ripgrep:" {
		t.Fatalf("searched = %v, want [brew:ripgrep:]", searched)
	}
	hasTexts(t, tt, "ripgrep", "Fast recursive grep", "installed", "Install…")

	// Clicking anywhere on a not-installed row (e.g. description) opens its install plan.
	if err := tt.Click("Fast recursive grep"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "install", "--formula", "ripgrep"}) {
		t.Fatalf("install plan from row click = %+v", a.op)
	}
	a.op = nil
	tt.Frame()

	// Clicking Install… button also opens the install plan.
	if err := tt.Click("Install…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "install", "--formula", "ripgrep"}) {
		t.Fatalf("install plan = %+v", a.op)
	}
	a.op = nil
	tt.Frame()

	// Clicking anywhere on an already-installed row jumps to its details in Packages.
	if err := tt.Click("JSON processor"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.pkg.view != pkgViewPackages || a.pkg.detailKey != packageKey(a.Snapshot.Tools[0].PackageManagers[0].Packages[0]) {
		t.Fatalf("installed row click: view=%d detailKey=%q", a.pkg.view, a.pkg.detailKey)
	}
	a.pkg.view = pkgViewInstall
	tt.Frame()

	// Error response shows warning.
	a.pkg.searchQuery = "fail"
	tt.Frame()
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "network unreachable")

	// Read-only Nix shows unsupported note and clears previous search query.
	a.router.Replace(packagesManagerPath("/nix/bin/nix-env"))
	tt.Frame()
	tt.Advance(time.Second)
	if a.pkg.searchQuery != "" {
		t.Fatalf("searchQuery = %q after switching manager, want empty", a.pkg.searchQuery)
	}
	hasTexts(t, tt, "Catalog search is not available for Nix.")
}

func TestKubeContextIconKindCluster(t *testing.T) {
	if got := kubeContextIcon(inventory.KubeContext{Name: "limited", Cluster: "kind-s10"}); got != "lucide/container" {
		t.Fatalf("kubeContextIcon(limited / kind-s10) = %q, want lucide/container", got)
	}
	if got := kubeContextIcon(inventory.KubeContext{Name: "prod-eu", Cluster: "prod-eu"}); got != "lucide/globe" {
		t.Fatalf("kubeContextIcon(prod-eu) = %q, want lucide/globe", got)
	}
}

func TestPackagesShowMoreAndRefresh(t *testing.T) {
	pkgs := make([]inventory.SystemPackage, 650)
	for i := range pkgs {
		pkgs[i] = inventory.SystemPackage{
			Name:    "pkg-" + string(rune('a'+i/100)) + "-" + time.Duration(i).String(),
			Version: "1.0.0",
			Kind:    "formula",
		}
		if i == 0 {
			pkgs[i].Available = "1.1.0"
		}
	}
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Version: "4.4.20", UpdatesKnown: true, Packages: pkgs},
		{ID: "pacman", Name: "pacman", Path: "/usr/bin/pacman", Version: "7.0", Packages: pkgs[:2]},
	}}}}
	a.router.Push(packagesManagerPath("/opt/homebrew/bin/brew"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Refresh index button is shown for brew and proposes brew update.
	if !tt.HasText("Refresh index…") {
		t.Fatalf("missing Refresh index button: %q", tt.Texts())
	}
	if err := tt.Click("Refresh index…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "update"}) {
		t.Fatalf("refresh plan = %+v", a.op)
	}
	a.op.open = false
	tt.Frame()

	// Initially shows 300 of 650 packages with "Show 300 more".
	if !tt.HasText("Showing 300 of 650 packages.") || !tt.HasText("Show 300 more") {
		t.Fatalf("initial pagination missing: %q", tt.Texts())
	}
	if a.packageLimit != 300 {
		t.Fatalf("packageLimit = %d, want 300", a.packageLimit)
	}
	scrollTo := func(label string) {
		t.Helper()
		b, ok := tt.Find(label)
		if !ok {
			t.Fatalf("cannot find %q", label)
		}
		tt.Scroll(500, 400, 0, b.Y-400)
		tt.Frame()
	}
	scrollTo("Show 300 more")
	if err := tt.Click("Show 300 more"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.packageLimit != 600 || !tt.HasText("Showing 600 of 650 packages.") || !tt.HasText("Show 50 more") {
		t.Fatalf("after first expand: limit=%d", a.packageLimit)
	}
	scrollTo("Show 50 more")
	if err := tt.Click("Show 50 more"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("Show 50 more") || tt.HasText("Showing 600 of 650 packages.") {
		t.Fatal("pagination footer should disappear once all rows are shown")
	}

	// Filtering resets packageLimit back to 300.
	a.packageFilter = "pkg-"
	tt.Frame()
	if a.packageLimit != 300 {
		t.Fatalf("filter did not reset packageLimit: %d", a.packageLimit)
	}
	a.packageFilter = ""
	tt.Frame()

	// Narrow window check (1000x680).
	narrow := ui.NewTester(a.View, 1000, 680)
	narrow.Advance(time.Second)
	if !narrow.HasText("Refresh index…") {
		t.Fatalf("Refresh index missing at 1000x680: %q", narrow.Texts())
	}
	screenshot(t, narrow, "packages-narrow")

	// Switching to pacman hides Refresh index (Arch forbids -Sy without -u).
	a.router.Replace(packagesManagerPath("/usr/bin/pacman"))
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Refresh index…") {
		t.Fatal("pacman should not show Refresh index button")
	}
}

func TestPackagesLandingCompactControls(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "packages", PackageManagers: []inventory.PackageManager{
			{
				ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew",
				Version: "Homebrew 4.4.20", UpdatesKnown: true, ReasonsKnown: true,
				Packages: []inventory.SystemPackage{
					{Name: "jq", Version: "1.7.1", Available: "1.8.2", Kind: "formula"},
				},
			},
		}},
		{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{{Kind: "PostgreSQL", Name: "PostgreSQL", Source: "brew", Status: "running", Service: "postgresql@17"}},
		}},
	}}
	a.router.Push(packagesPage)

	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)

	var refreshIcon *ui.Rect
	for _, r := range accessButtonsByName(tt, locales.PkgRefresh()) {
		if r.Y > 80 {
			rect := r
			refreshIcon = &rect
			break
		}
	}
	if refreshIcon == nil {
		t.Fatalf("missing Maintenance %q icon button (all=%+v)", locales.PkgRefresh(), accessButtonsByName(tt, locales.PkgRefresh()))
	}
	if refreshIcon.W > 32 || refreshIcon.X+refreshIcon.W > 1000-20 {
		t.Fatalf("refresh icon button unexpected bounds: %+v", *refreshIcon)
	}
	cleanupRow, ok := tt.AccessByRole(ui.RoleButton, locales.PkgOpenCleanup())
	if !ok {
		t.Fatalf("missing %q nav row", locales.PkgOpenCleanup())
	}
	if cleanupRow.Bounds.W < 180 {
		t.Fatalf("expected full-width clickable nav row for cleanup, got %+v", cleanupRow.Bounds)
	}
	if cleanupRow.Bounds.Y+cleanupRow.Bounds.H > 680 {
		tt.Scroll(500, 400, 0, cleanupRow.Bounds.Y-300)
		tt.Frame()
	}
	if err := tt.Click(locales.PkgOpenCleanup()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage {
		t.Fatalf("clicking maintenance cleanup row navigated to %q, want %q", a.router.Path(), cleanupPage)
	}
}

func TestPackagesInstallWinGetKindAgnosticMatch(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "packages",
		PackageManagers: []inventory.PackageManager{{
			ID:   "winget",
			Name: "WinGet",
			Path: `C:\Users\dev\AppData\Local\Microsoft\WindowsApps\winget.exe`,
			Packages: []inventory.SystemPackage{
				{Name: "jqlang.jq", Version: "1.7.1", Kind: "portable"},
			},
		}},
	}}}
	a.SearchPackages = func(_ inventory.PackageManager, _ string, _ string, done func([]pkgmgr.SearchResult, error)) func() {
		done([]pkgmgr.SearchResult{
			{Name: "jqlang.jq", Version: "1.7.1", Kind: "package", Description: "jq", Origin: "winget"},
			{Name: "BurntSushi.rg", Version: "14.1.1", Kind: "package", Description: "ripgrep", Origin: "winget"},
		}, nil)
		return func() {}
	}
	a.pkg.view = pkgViewInstall
	a.router.Push(packagesManagerPath(`C:\Users\dev\AppData\Local\Microsoft\WindowsApps\winget.exe`))

	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	a.pkg.searchQuery = "jq"
	tt.Frame()
	if err := tt.Click(locales.PkgSearchButton()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText(locales.PkgSearchInstalled()) {
		t.Fatalf("expected installed chip for WinGet portable package, texts: %q", tt.Texts())
	}
	if err := tt.Click("jqlang.jq"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.pkg.view != pkgViewPackages || a.pkg.detailKey == "" {
		t.Fatalf("clicking installed search row did not open package detail: view=%d detailKey=%q", a.pkg.view, a.pkg.detailKey)
	}
}
