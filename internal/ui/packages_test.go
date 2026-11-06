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
	if err := tt.Click("Updates"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
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
	hasTexts(t, tt, "Installed by you", "installed by you", "dependency · needed by 1 package", "dependency · nothing needs it any more")
	if err := tt.Click("Dependencies"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("node") || !tt.HasText("icu4c") {
		t.Errorf("dependencies tab: %q", tt.Texts())
	}
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
