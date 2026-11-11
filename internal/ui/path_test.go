package ui

import (
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPathPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{
		TerminalPath: []inventory.PathEntry{
			{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: home + "/.zprofile", Line: 3}}},
			{Dir: home + "/.local/bin", Sources: []inventory.PathSource{{File: home + "/.zshrc", Line: 7}}},
			{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: home + "/.zshrc", Line: 41}}},
			{Dir: home + "/gone/bin", Missing: true},
			{Dir: "/usr/local/bin", Missing: true},
		}, GUIPath: []inventory.PathEntry{{Dir: "/usr/bin"}, {Dir: "/bin"}},
		Variables: []inventory.EnvironmentVariable{{Name: "JAVA_HOME", Terminal: "/jdk"}, {Name: "GOPATH", Terminal: home + "/go", GUI: home + "/go"}},
	}
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "path")
	// These Unix paths are not absolute on Windows, and so problems there.
	if runtime.GOOS != "windows" {
		// Only ~/gone/bin counts: macOS lists /usr/local/bin before anything is there.
		if got := pathProblemCount(a.Snapshot.TerminalPath, a.Snapshot.GUIPath); got != 2 {
			t.Fatalf("problems %d, want the repeat and ~/gone/bin", got)
		}
		// path_helper adds /etc/paths.d's entry again after brew shellenv put it first.
		brew := inventory.PathEntry{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: "/etc/paths.d/homebrew", Line: 1}}}
		if got := pathProblemCount([]inventory.PathEntry{brew, {Dir: "/usr/bin"}, brew}, nil); got != 0 {
			t.Fatalf("system repeat counted: %d", got)
		}
		// Ubuntu's profile.d adds /etc/environment's /snap/bin again.
		snap := inventory.PathEntry{Dir: "/snap/bin", Sources: []inventory.PathSource{{File: "/etc/environment", Line: 1}}}
		if got := pathProblemCount([]inventory.PathEntry{snap, snap}, nil); got != 0 {
			t.Fatalf("/etc/environment repeat counted: %d", got)
		}
	}
	// Both environments share the missing entry, as on Windows they share the registry's Path.
	gone := []inventory.PathEntry{{Dir: home + "/gone/bin", Missing: true}}
	if got := pathProblemCount(gone, gone); got != 1 {
		t.Fatalf("shared problem counted %d times", got)
	}
	for _, s := range []string{"Repeats entry #1", "Directory missing", "Terminal only", "Applications only", "JAVA_HOME"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %s: %q", s, tt.Texts())
		}
	}
	a.pathTab = 1
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Directory missing") {
		t.Fatal(tt.Texts())
	}
	a.pathTab = 0
	a.pathFilter = "not-found"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("No matching directories.") {
		t.Fatal(tt.Texts())
	}
	a.pathFilter = ""
	a.pathTab = 2
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Terminal: /jdk") || !tt.HasText("Applications: Not set") {
		t.Fatal(tt.Texts())
	}
	a.Snapshot = nil
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("No PATH entries in this snapshot.") {
		t.Fatal(tt.Texts())
	}
}

// The PATH page fixes the problems of the terminal's PATH that come from
// the user's files: all from the banner, one from its row; a plan that
// cannot be made says why.
func TestPathFix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths and startup files")
	}
	a := NewApp()
	a.Home = home
	zshrc := home + "/.zshrc"
	a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{
		{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: home + "/.zprofile", Line: 3}, {File: zshrc, Line: 41}}},
		{Dir: "/usr/bin", Sources: []inventory.PathSource{{File: "/etc/paths", Line: 1}}},
		{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: home + "/.zprofile", Line: 3}, {File: zshrc, Line: 41}}},
		{Dir: home + "/.nvm/versions/node/v16.20.2/bin", Missing: true, Sources: []inventory.PathSource{{File: zshrc, Line: 12}}},
		{Dir: "/nowhere", Missing: true},
	}}
	var asked [][]string
	var err error
	a.PlanPath = func(_ []inventory.PathEntry, dirs []string, done func(ops.Plan, error)) {
		asked = append(asked, dirs)
		done(ops.Plan{Title: "Fix PATH: 2 entries"}, err)
	}
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "path-fix")
	// The repeat's row and the missing folder's from the user's file can
	// be fixed; the first /opt/homebrew/bin and /nowhere cannot.
	for _, dir := range []string{"/opt/homebrew/bin", "~/.nvm/versions/node/v16.20.2/bin"} {
		if _, ok := tt.Find("Remove " + dir + " from PATH"); !ok {
			t.Errorf("no button removes %s", dir)
		}
	}
	if n := slices.Index(tt.Texts(), "Remove…"); n < 0 || slices.Index(tt.Texts()[n+1:], "Remove…") < 0 || tt.HasText("Remove /nowhere from PATH") {
		t.Errorf("want two rows to remove: %q", tt.Texts())
	}
	if err := tt.Click("Fix…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(asked) != 1 || asked[0] != nil || !tt.HasText("Fix PATH: 2 entries") {
		t.Fatalf("asked %q, texts %q", asked, tt.Texts())
	}
	a.op = nil
	err = ops.ErrNoFix
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Remove ~/.nvm/versions/node/v16.20.2/bin from PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(asked) != 2 || len(asked[1]) != 1 || asked[1][0] != home+"/.nvm/versions/node/v16.20.2/bin" {
		t.Fatalf("asked %q", asked)
	}
	if !tt.HasText("Hearth cannot fix this where it is declared: the line adds other folders too, or Hearth cannot tell where the entry comes from") {
		t.Errorf("no reason: %q", tt.Texts())
	}
}

// An entry of the terminal's PATH after another can be put first; the
// first, a repeat and a missing folder cannot.
func TestPathFirst(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths and startup files")
	}
	dir := t.TempDir()
	a := NewApp()
	a.Home = dir
	a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{
		{Dir: "/opt/homebrew/bin"}, {Dir: dir + "/bin"}, {Dir: "/opt/homebrew/bin"}, {Dir: "/nowhere", Missing: true},
	}}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "SHELL=/bin/zsh"}, nil))
	}
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if n := slices.Index(tt.Texts(), "More"); n < 0 || slices.Contains(tt.Texts()[n+1:], "More") {
		t.Fatalf("want one menu: %q", tt.Texts())
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Put first…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Put "+dir+"/bin first in PATH" || a.op.plan.Steps[0].Edit.Path != dir+"/.zshrc" {
		t.Fatalf("plan %+v", a.op)
	}
}
