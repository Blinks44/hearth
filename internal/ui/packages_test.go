package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestPackagesPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", Version: "Homebrew 4.4.20", UpdatesKnown: true, Taps: []string{"custom/tools"}, Settings: []string{"HOMEBREW_NO_AUTO_UPDATE=—"}, Packages: []inventory.SystemPackage{
			{Name: "postgresql@17", Version: "17.2", Available: "17.3", Kind: "formula", Path: "/opt/homebrew/Cellar/postgresql@17/17.2"},
			{Name: "python@3.13", Version: "3.13.1", Available: "3.13.2", Kind: "formula", Path: "/opt/homebrew/Cellar/python@3.13/3.13.1"},
			{Name: "orbstack", Version: "1.9.5", Available: "1.9.6", Kind: "cask", Path: "/opt/homebrew/Caskroom/orbstack"},
			{Name: "node", Version: "23.6.0", Kind: "formula", Path: "/opt/homebrew/Cellar/node/23.6.0"},
		}},
		{ID: "nix", Name: "Nix", Path: "/nix/bin/nix-env", Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12", Kind: "profile"}}},
		{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true, Packages: []inventory.SystemPackage{{Name: "Example.CLI", Version: "2.0", Kind: "portable"}}},
	}}}}
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "packages")
	for _, text := range []string{"postgresql@17", "orbstack", "custom/tools", "node"} {
		if !tt.HasText(text) {
			t.Fatalf("no %s: %q", text, tt.Texts())
		}
	}
	// The sidebar's Updates comes before the tab.
	a.packageTab = updatesTabIndex(a.Snapshot.Tools[0].PackageManagers[0])
	tt.Frame()
	if tt.HasText("node") || !tt.HasText("python@3.13") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// Updates not checked show as unknown, not as none.
	if !tt.HasText("Online Nix update data is not available yet. Flake versions are evaluated without building packages or changing profiles; configured channels are queried as they are.") || !tt.HasText("—") {
		t.Fatal(tt.Texts())
	}
	a.packageTab = 0
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("hello") {
		t.Fatal(tt.Texts())
	}
	a.packageManager = "/winget"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Only locally registered portable packages are shown until an online WinGet query succeeds. Sources may require agreements you must accept outside Hearth.") {
		t.Fatal(tt.Texts())
	}
	if !tt.HasText("Example.CLI") || !tt.HasText("—") {
		t.Fatal(tt.Texts())
	}
	screenshot(t, tt, "packages-winget")
	a.Snapshot.Tools[0].PackageManagers = nil
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("hello") {
		t.Fatal(tt.Texts())
	}
}

func TestPackagesOnlineOverlay(t *testing.T) {
	a := NewApp()
	local := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{local}}}}
	a.PackageUpdates = map[string]inventory.PackageManager{"/winget": {ID: "winget", Name: "WinGet", Path: "/winget", UpdatesKnown: true, UpdatesFetched: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), Packages: []inventory.SystemPackage{{Name: "Example.CLI", Version: "1.0", Available: "2.0"}}}}
	a.router.Push("/system/packages")
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

// msstore did not answer: what winget's source listed counts as at least,
// and the note on local packages no longer applies.
func TestPackagesPartialSources(t *testing.T) {
	a := NewApp()
	m := inventory.PackageManager{ID: "winget", Name: "WinGet", Path: "/winget", Unlisted: true}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{m}}}}
	m.UpdatesFetched = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	m.UpdateError = "some package sources could not be queried"
	m.Packages = []inventory.SystemPackage{{Name: "Microsoft.OneDrive", Version: "22.1", Available: "26.1"}, {Name: "Microsoft.Teams", Version: "26.1"}}
	a.PackageUpdates = map[string]inventory.PackageManager{"/winget": m}
	a.router.Push("/system/packages")
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

// A name too long for its column is cut, and the pointer resting on it
// shows it whole.
func TestPackageNameTooltip(t *testing.T) {
	long := "docker-hub.example.org/digital/platform/redis-with-modules"
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Packages: []inventory.SystemPackage{{Name: long, Version: "1.0", Kind: "formula"}}},
	}}}}
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	at, ok := tt.Find(long)
	if !ok {
		t.Fatalf("no %s: %q", long, tt.Texts())
	}
	if before := countText(tt, long); before != 1 {
		t.Fatalf("%d copies of the name before resting on it", before)
	}
	tt.Move(at.X+at.W/2, at.Y+at.H/2)
	tt.Advance(time.Second)
	if n := countText(tt, long); n != 2 {
		t.Errorf("%d copies of the name after resting on it, want the cell and its tip", n)
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

// Where the manager knows why a package is there, the page splits what the
// user installed from dependencies, and says who needs each.
func TestPackageReasons(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "packages", PackageManagers: []inventory.PackageManager{
		{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "node", Kind: "formula", Dependencies: []string{"icu4c"}},
			{Name: "icu4c", Kind: "formula", Dependency: true},
			{Name: "oniguruma", Kind: "formula", Dependency: true},
		}},
	}}}}
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Installed by you", "installed by you", "needed by node", "dependency · nothing needs it any more")
	if err := tt.Click("Dependencies"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if slices.Contains(tt.Texts(), "node") || !tt.HasText("icu4c") {
		t.Errorf("dependencies tab: %q", tt.Texts())
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
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "needed by app-a, app-b, app-c (+1)")
}

// A package's size comes from the manager or from the count, a cask's
// with its app; one partly read is "at least", and the biggest sort first.
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
	a.router.Push("/system/packages")
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
	if want := []string{"orbstack", "node", "icu4c", "zlib"}; !slices.Equal(order, want) {
		t.Errorf("by size: %q, want %q", order, want)
	}
}

// A package's menu updates and removes it through its manager; the
// Updates tab updates the packages checked, never all at once.
func TestPackageOperations(t *testing.T) {
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
	a.router.Push("/system/packages")
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
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/opt/homebrew/bin/brew", "uninstall", "--formula", "jq"}) {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("Updating all 2 packages at once can break the projects that rely on them. Hearth updates only those you choose.") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Choose packages…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "packages-choose")
	if a.packageTab != 3 || tt.HasText("oniguruma") {
		t.Fatalf("tab %d: %q", a.packageTab, tt.Texts())
	}
	if err := tt.Click("Update selected"); err != nil {
		t.Fatal(err)
	}
	if a.op.open {
		t.Fatal("updated with none checked")
	}
	if err := tt.Click("jq"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Update selected"); err != nil {
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
	// Arch updates the whole system: nothing to choose.
	a.packageManager, a.packageTab = "/usr/bin/pacman", 0
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Choose packages…") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Update the whole system…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"/usr/bin/pacman", "-Syu", "--noconfirm"}) {
		t.Fatalf("plan %+v", a.op)
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
	a.router.Push("/system/packages")
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
	screenshot(t, tt, "packages-more")
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

	// Narrow window check: Installed hides Update selected; Updates shows both.
	narrow := ui.NewTester(a.View, 1000, 680)
	narrow.Advance(time.Second)
	if !narrow.HasText("Refresh index…") {
		t.Fatalf("Refresh index missing at 1000x680: %q", narrow.Texts())
	}
	if narrow.HasText("Update selected") {
		t.Fatalf("Update selected should be hidden outside Updates tab: %q", narrow.Texts())
	}
	a.packageTab = 1
	narrow.Frame()
	narrow.Advance(time.Second)
	screenshot(t, narrow, "packages-narrow")
	if !narrow.HasText("Refresh index…") || !narrow.HasText("Update selected") {
		t.Fatalf("header actions missing on Updates tab at 1000x680: %q", narrow.Texts())
	}
	a.packageTab = 0
	tt.Frame()

	// Switching to pacman hides Refresh index (Arch forbids -Sy without -u).
	a.packageManager = "/usr/bin/pacman"
	tt.Frame()
	if tt.HasText("Refresh index…") {
		t.Fatal("pacman should not show Refresh index button")
	}
}

func TestPackagesManagedSelectAllAndCancel(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "packages",
		PackageManagers: []inventory.PackageManager{
			{
				ID: "brew", Name: "Homebrew", Version: "4.4.20", Path: "/opt/homebrew/bin/brew",
				UpdatesKnown: true, ReasonsKnown: true,
				Packages: []inventory.SystemPackage{
					{Name: "git", Version: "2.48.0", Available: "2.49.0", Kind: "formula"},
					{Name: "ripgrep", Version: "14.1.0", Available: "14.1.1", Kind: "formula"},
				},
			},
			{
				ID: "nix", Name: "Nix", Version: "2.24", Path: "/nix/var/nix/profiles/default/bin/nix",
				Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12"}},
			},
		},
	}}}
	a.router.Push("/system/packages")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Managed brew must not claim read-only.
	if tt.HasText("view only") || tt.HasText("read-only") {
		t.Fatalf("managed Homebrew must not display read-only labels: %q", tt.Texts())
	}
	// Switch to Updates tab (index 3 when ReasonsKnown is true: All/Explicit/Deps/Updates).
	a.packageTab = 3
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Update selected") {
		t.Fatalf("Updates tab should show Update selected: %q", tt.Texts())
	}
	// Select all via header checkbox.
	if err := tt.Click("Select all"); err != nil {
		t.Fatalf("click Select all: %v", err)
	}
	tt.Advance(time.Second)
	brew := a.Snapshot.Tools[0].PackageManagers[0]
	if len(a.packageChosen) != 2 || !a.packageChosen[brew.Path+"\x00"+packageKey(brew.Packages[0])] || !a.packageChosen[brew.Path+"\x00"+packageKey(brew.Packages[1])] {
		t.Fatalf("Select all did not select both updates: %+v", a.packageChosen)
	}
	// Opening proposal and cancelling preserves selection.
	if err := tt.Click("Update selected"); err != nil {
		t.Fatalf("click Update selected: %v", err)
	}
	tt.Advance(time.Second)
	if a.op == nil {
		t.Fatal("expected upgrade proposal dialog")
	}
	a.op = nil
	tt.Frame()
	if len(a.packageChosen) != 2 {
		t.Fatalf("cancelling proposal cleared packageChosen: %+v", a.packageChosen)
	}

	// Unmanaged nix still shows read-only label.
	a.packageManager = "/nix/var/nix/profiles/default/bin/nix"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("read only") {
		t.Fatalf("unmanaged nix should show read-only label: %q", tt.Texts())
	}
}
