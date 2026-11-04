package ui

import (
	"errors"
	"image/color"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

const home = "/Users/dev"

// nodeSnapshot is fnm with two versions, the default first in PATH, and
// a Homebrew copy after it.
func nodeSnapshot() *inventory.Snapshot {
	fnm := func(v string) inventory.Source {
		return inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm",
			Root: home + "/.local/share/fnm/node-versions/v" + v, Version: v}
	}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		{Tool: "node", Version: "22.12.0", Path: home + "/.local/state/fnm_multishells/48121/bin/node",
			Source: fnm("22.12.0"), OnPath: 1, Default: true, Corepack: true, Packages: []inventory.Package{
				{Name: "corepack", Version: "0.30.0"}, {Name: "npm", Version: "10.9.0"},
				{Name: "typescript", Version: "5.7.2", Commands: []string{"tsc", "tsserver"}},
			}},
		{Tool: "node", Version: "23.6.0", Path: "/opt/homebrew/bin/node",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/node/23.6.0"}, OnPath: 2, InGUI: 1},
		{Tool: "node", Version: "20.18.1", Path: home + "/.local/share/fnm/node-versions/v20.18.1/installation/bin/node",
			Source: fnm("20.18.1")},
	}, Managers: []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".node-version", ".nvmrc"}}}}}}
}

func openNode(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push("/tools/node")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The page lists both copies, fnm's and Homebrew's, and marks the one the
// terminal runs; Homebrew's, later in PATH, is a duplicate.
func TestNodeVersions(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := openNode(t, a)
	for _, s := range []string{
		"Versions managed by fnm · also from brew", "3 versions", "fnm · brew", "which node → fnm",
		"v22.12.0", "v23.6.0", "v20.18.1", "LTS", "Current",
		"~/.local/share/fnm/node-versions/v22.12.0", "/opt/homebrew/bin/node",
		"default", "in PATH", "duplicate in PATH",
	} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	// Only the active row is tinted, behind its path.
	bg := func(text string) color.RGBA {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("no %q", text)
		}
		return tt.Image().RGBAAt(int(r.X)-4, int(r.Y+r.H/2))
	}
	want := Colors(Theme(ui.LightTheme())).ActiveRow
	if got := bg("~/.local/share/fnm/node-versions/v22.12.0"); !same(got, want) {
		t.Errorf("active row %v, want %v", got, want)
	}
	if got := bg("/opt/homebrew/bin/node"); same(got, want) {
		t.Error("a row not in use is tinted")
	}
}

func same(c color.RGBA, want ui.Color) bool {
	return c.R == want.R && c.G == want.G && c.B == want.B
}

// Without a copy to show, the page says why: not checked, failed, none.
func TestNodeEmpty(t *testing.T) {
	a := NewApp()
	tt := openNode(t, a)
	if !tt.HasText("Not checked yet") {
		t.Errorf("before a scan, texts %q", tt.Texts())
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Err: "timeout"}}}
	tt.Frame()
	if !tt.HasText("Could not check Node.js: timeout") {
		t.Errorf("failed, texts %q", tt.Texts())
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node"}}}
	tt.Frame()
	if !tt.HasText("Node.js is not installed on this computer") {
		t.Errorf("none, texts %q", tt.Texts())
	}
}

func TestManagedBy(t *testing.T) {
	src := func(k inventory.Kind, m string) inventory.Installation {
		return inventory.Installation{Source: inventory.Source{Kind: k, Manager: m}}
	}
	brew := src(inventory.KindPackageManager, "brew")
	nvm := src(inventory.KindVersionManager, "nvm")
	tests := []struct {
		ins    []inventory.Installation
		active int
		want   string
	}{
		{[]inventory.Installation{nvm, nvm}, 0, "Versions managed by nvm"},
		{[]inventory.Installation{brew, nvm}, 0, "Versions managed by nvm · also from brew"},
		{[]inventory.Installation{brew, src(inventory.KindInstaller, "")}, -1, "Installed from brew, installer"},
	}
	for _, tt := range tests {
		rows := listed(tt.ins)
		if got := managedBy(tt.ins, rows, tt.active); got != tt.want {
			t.Errorf("managedBy = %q, want %q", got, tt.want)
		}
	}
}

// nodeVersions is a version list as of now: lines 24 and 22 in LTS, 20
// and 23 ended.
func nodeVersions(now time.Time) *versions.Node {
	days := func(n int) time.Time { return now.AddDate(0, 0, n) }
	return &versions.Node{
		Fetched: days(-2),
		Releases: []versions.Release{
			{Version: "24.1.0", LTS: "Krypton"}, {Version: "23.11.0"},
			{Version: "22.14.0", LTS: "Jod"}, {Version: "20.19.0", LTS: "Iron"},
		},
		Lines: map[int]versions.Line{
			24: {Start: days(-200), LTS: days(-30), Maintenance: days(300), End: days(900), Codename: "Krypton"},
			23: {Start: days(-400), Maintenance: days(-200), End: days(-1)},
			22: {Start: days(-600), LTS: days(-400), Maintenance: days(-30), End: days(200), Codename: "Jod"},
			20: {Start: days(-900), LTS: days(-800), Maintenance: days(-400), End: days(-1), Codename: "Iron"},
		},
	}
}

func TestChannel(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	nv := nodeVersions(now)
	nv.Lines[26] = versions.Line{Start: now.AddDate(0, -6, 0), LTS: now.AddDate(0, 0, 7), Codename: "Lithium"}
	tests := []struct {
		nv      *versions.Node
		version string
		want    string
		ended   bool
	}{
		{nv, "22.12.0", "LTS · Jod", false},
		{nv, "20.18.1", "LTS · Iron", true},
		{nv, "23.6.0", "Current", true},
		{nv, "26.0.0", "Current", false}, // LTS next week
		{nil, "22.12.0", "LTS", false},
		{nil, "23.6.0", "Current", false},
		{nv, "", "", false},
	}
	for _, tt := range tests {
		got, ended := channel(tt.nv, tt.version, now)
		if got != tt.want || ended != tt.ended {
			t.Errorf("channel(%q) = %q, %v, want %q, %v", tt.version, got, ended, tt.want, tt.ended)
		}
	}
}

func TestReleaseNote(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	nv := nodeVersions(now)
	tests := []struct {
		release   string
		installed []string
		want      string
	}{
		{"22.14.0", []string{"22.9.0", "22.12.0", "20.18.1"}, "update for v22.12.0"},
		{"22.14.0", []string{"22.14.0"}, "installed"},
		{"22.14.0", nil, "support until May 2027"},
		{"24.1.0", nil, ""},
	}
	for _, tt := range tests {
		r := versions.Release{Version: tt.release}
		if got := releaseNote(r, nv.Lines[versions.Major(tt.release)], tt.installed, now); got != tt.want {
			t.Errorf("releaseNote(%s, %q) = %q, want %q", tt.release, tt.installed, got, tt.want)
		}
	}
}

// The list of versions shows beside the table, saying how old it is, and
// the table takes the lines' names and ends from it.
func TestNodeAvailable(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := openNode(t, a)
	if !tt.HasText("Getting the version list…") {
		t.Errorf("before the list, texts %q", tt.Texts())
	}
	a.NodeVersionsErr = "no connection"
	tt.Frame()
	if !tt.HasText("Could not get the version list: no connection") {
		t.Errorf("failed, texts %q", tt.Texts())
	}
	a.NodeVersions, a.NodeVersionsErr = nodeVersions(tt.Now()), ""
	tt.Frame()
	for _, s := range []string{"Available to install", "v24.1.0", "LTS · Krypton", "recommended",
		"v22.14.0", "LTS · Jod · update for v22.12.0", "LTS · Jod", "support ended"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("v20.19.0") || tt.HasText("v23.11.0") {
		t.Error("an ended line offered")
	}
	if err := tt.Click("Available"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("v24.1.0") || tt.HasText("Path and use") {
		t.Errorf("Available tab, texts %q", tt.Texts())
	}
}

func TestDisplayPath(t *testing.T) {
	vm := inventory.Installation{Path: home + "/.nvm/versions/node/v22.0.0/bin/node",
		Source: inventory.Source{Kind: inventory.KindVersionManager, Root: home + "/.nvm/versions/node/v22.0.0"}}
	for in, want := range map[*inventory.Installation]string{
		&vm:                              "~/.nvm/versions/node/v22.0.0",
		{Path: "/opt/homebrew/bin/node"}: "/opt/homebrew/bin/node",
		{Path: home + "x/bin/node"}:      home + "x/bin/node",
	} {
		if got := displayPath(*in, home); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", in.Path, got, want)
		}
	}
}

// The terminal's PATH shows in order, the first used; apps outside the
// terminal that find Homebrew's node first get a warning.
func TestNodePath(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := openNode(t, a)
	for _, s := range []string{"How the terminal finds node", "which -a node",
		"~/.local/state/fnm_multishells/48121/bin/node", "used", "shadowed",
		"Apps opened outside the terminal (VS Code, IDEs) find brew first and run v23.6.0 instead of v22.12.0."} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}

	// The same version in apps as in the terminal: no warning, on the
	// PATH tab too.
	a.Snapshot.Tools[0].Installations[0].InGUI = 1
	a.Snapshot.Tools[0].Installations[1].InGUI = 2
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("which -a node") || tt.HasText("v20.18.1") {
		t.Errorf("PATH tab, texts %q", tt.Texts())
	}
	used, _ := tt.Find("~/.local/state/fnm_multishells/48121/bin/node")
	shadowed, _ := tt.Find("/opt/homebrew/bin/node")
	if used.Y >= shadowed.Y {
		t.Error("PATH entries out of order")
	}
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, "Apps opened") {
			t.Errorf("warning without a difference: %q", s)
		}
	}
}

func TestGUIWarning(t *testing.T) {
	node := func(v string, term, gui inventory.PathRank) inventory.Installation {
		return inventory.Installation{Version: v, OnPath: term, InGUI: gui,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
	}
	tests := []struct {
		name string
		ins  []inventory.Installation
		want string
	}{
		{"same copy", []inventory.Installation{node("22.0.0", 1, 1)}, ""},
		{"same version elsewhere", []inventory.Installation{node("22.0.0", 1, 0), node("22.0.0", 0, 1)}, ""},
		{"other version", []inventory.Installation{node("22.0.0", 1, 0), node("20.0.0", 2, 1)},
			"Apps opened outside the terminal (VS Code, IDEs) find brew first and run v20.0.0 instead of v22.0.0."},
		{"not found", []inventory.Installation{node("22.0.0", 1, 0)},
			"Apps opened outside the terminal (VS Code, IDEs) do not find node, though the terminal does."},
		{"not in the terminal", []inventory.Installation{node("22.0.0", 0, 1)}, ""},
	}
	for _, tt := range tests {
		if got := guiWarning("node", tt.ins, inventory.TerminalCopy(tt.ins)); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The version manager's card says how fnm is set up, and the globals card
// lists the packages of the version in use, not those Node ships.
func TestNodeManager(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := openNode(t, a)
	for _, s := range []string{"Version manager · fnm", "Default version", "Switch on cd", "reads .node-version, .nvmrc",
		"Corepack", "Global packages", "npm -g · v22.12.0", "typescript", "commands: tsc, tsserver", "5.7.2"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("10.9.0") || tt.HasText("0.30.0") {
		t.Error("npm or Corepack listed as a global package")
	}
}

// Node only from Homebrew: no version manager, and mise will add versions.
func TestNodeNoManager(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{
		{Tool: "node", Version: "24.22.1", Path: "/opt/homebrew/bin/node", OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}},
	}}}}
	tt := openNode(t, a)
	for _, s := range []string{"No version manager: Node comes from brew. Hearth will install other versions through mise.",
		"not bundled", "No global packages"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("Switch on cd") {
		t.Error("a manager's settings without a manager")
	}
}

// fakeShell records what the page asks the system to show or open.
type fakeShell struct {
	revealed, opened []string
	err              error
}

func (f *fakeShell) ShowItemInFolder(path string) { f.revealed = append(f.revealed, path) }

func (f *fakeShell) OpenPath(path string) error {
	f.opened = append(f.opened, path)
	return f.err
}

// A row's actions copy its path, with a toast, show it in the file
// manager and open npm's settings, saying when that fails.
func TestNodeRowActions(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	a.Snapshot.Tools[0].Installations[0].RealPath = home + "/.local/share/fnm/node-versions/v22.12.0/installation/bin/node"
	shell := &fakeShell{}
	a.Shell = shell
	tt := openNode(t, a)

	if err := tt.Click("Copy path"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != home+"/.local/share/fnm/node-versions/v22.12.0" {
		t.Errorf("clipboard %q", got)
	}
	if !tt.HasText("Path copied") {
		t.Errorf("no toast in %q", tt.Texts())
	}

	if err := tt.Click(showInFolder()); err != nil {
		t.Fatal(err)
	}
	if want := []string{a.Snapshot.Tools[0].Installations[0].RealPath}; !reflect.DeepEqual(shell.revealed, want) {
		t.Errorf("revealed %q, want %q", shell.revealed, want)
	}

	shell.err = errors.New("no such file")
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open npm settings (.npmrc)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	npmrc := filepath.Join(home, ".npmrc")
	if want := []string{npmrc}; !reflect.DeepEqual(shell.opened, want) {
		t.Errorf("opened %q, want %q", shell.opened, want)
	}
	if !tt.HasText("Could not open " + tilde(npmrc, home) + ": no such file") {
		t.Errorf("no error toast in %q", tt.Texts())
	}
}

// The sidebar shows the version the terminal runs beside a language
// found.
func TestSidebarVersion(t *testing.T) {
	a := NewApp()
	a.Snapshot = nodeSnapshot()
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("22.12") {
		t.Errorf("texts %q", tt.Texts())
	}
}
