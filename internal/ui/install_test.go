package ui

import (
	"context"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// cleanApp is a computer with nothing installed but its system's package
// manager, which builds plans in a terminal without mise or Node.
func cleanApp(t *testing.T) *App {
	t.Helper()
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "node"}, {Tool: "npm"}, {Tool: "pnpm"}, {Tool: "yarn"}, wingetState,
	}, Agents: []inventory.Agent{{ID: "codex", Dir: home + "/.codex"}}}
	env := sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh", "PATH="}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	a.DetectTool = func(context.Context, string) (inventory.ToolState, error) { return inventory.ToolState{}, nil }
	a.DetectAgent = func(context.Context, string) (inventory.Agent, error) { return inventory.Agent{}, nil }
	return a
}

// On a clean computer an agent and pnpm still have their install
// button, and the plan installs what they need first: mise and Node for
// pnpm everywhere, and for Codex where npm installs it.
func TestInstallOnACleanComputer(t *testing.T) {
	tests := []struct {
		name, page, button string
		changes            []string
	}{
		{"agent", "/ai/agents", "Install…", []string{"+ Codex"}},
		{"pnpm", "/tools/jspm", "Install…", []string{"+ mise", "+ Node.js LTS", "+ pnpm"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := cleanApp(t)
			tt := openAI(t, a, tc.page)
			screenshot(t, tt, "install-clean-"+tc.name)
			if err := tt.Click(tc.button); err != nil {
				t.Fatalf("%v: %q", err, tt.Texts())
			}
			tt.Advance(time.Second)
			if a.op == nil {
				t.Fatalf("no plan: %q", tt.Texts())
			}
			changes := tc.changes
			if tc.name == "agent" && runtime.GOOS != "darwin" {
				changes = []string{"+ mise", "+ Node.js LTS", "+ Codex"}
			}
			for _, c := range changes {
				if !slices.Contains(a.op.plan.Changes, c) {
					t.Errorf("changes %q, want %q", a.op.plan.Changes, c)
				}
			}
			if a.op.plan.Verify == nil {
				t.Error("the plan checks nothing")
			}
		})
	}
}

// A tool there already offers no install, and its page's empty state
// none either while the scan has not run.
func TestInstallOnlyWhatIsMissing(t *testing.T) {
	a := cleanApp(t)
	a.Snapshot.Tools[0].Installations = []inventory.Installation{{Tool: "node", Version: "24.11.0", Path: "/usr/bin/node", OnPath: 1}}
	if a.canInstallTool("node") {
		t.Error("Node there installs again")
	}
	if !a.canInstallTool("pnpm") {
		t.Error("pnpm cannot install beside Node")
	}
	a.Snapshot = nil
	if a.canInstallTool("pnpm") {
		t.Error("an install before the scan")
	}
}

// PHP's page offers Composer, and Java's its build tools, when the scan
// found none: through Homebrew and mise as the catalog says.
func TestInstallLanguageTools(t *testing.T) {
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	brew := inventory.ToolState{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}}}
	php := inventory.ToolState{Tool: "php", Installations: []inventory.Installation{{Tool: "php", Version: "8.4.4", Path: "/opt/homebrew/bin/php", OnPath: 1}}}
	java := inventory.ToolState{Tool: "java", Installations: []inventory.Installation{{Tool: "java", Version: "25.0.1", Path: home + "/.local/share/mise/installs/java/25/bin/java", OnPath: 1}}}
	// mise installs first, through Homebrew: its step names it by path.
	tests := []struct {
		page, row, tool string
		want            []string
	}{
		{"/tools/php", "Composer is not installed on this computer", "composer", []string{"install", "--formula", "composer"}},
		{"/tools/java", "Maven", "maven", []string{"use", "--global", "maven@latest"}},
	}
	for _, tc := range tests {
		t.Run(tc.page, func(t *testing.T) {
			a := cleanApp(t)
			a.Snapshot.Tools = []inventory.ToolState{brew, php, java, {Tool: "composer"}}
			tt := openAI(t, a, tc.page)
			if !tt.HasText(tc.row) {
				t.Fatalf("no %q: %q", tc.row, tt.Texts())
			}
			if !tt.HasText("Install…") {
				t.Fatalf("no button: %q", tt.Texts())
			}
			a.installTool(tc.tool)
			tt.Advance(time.Second)
			if a.op == nil {
				t.Fatal("no plan")
			}
			steps := a.op.plan.Steps
			if last := steps[len(steps)-1].Argv; !slices.Equal(last[len(last)-len(tc.want):], tc.want) {
				t.Errorf("steps %+v", steps)
			}
		})
	}
}

// installPageApp is a Mac with Node through mise and Claude Code, on the
// Install page.
func installPageApp(t *testing.T) *App {
	t.Helper()
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	a := cleanApp(t)
	mise := inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise"}
	a.Snapshot.Tools = []inventory.ToolState{
		{Tool: "node", Installations: []inventory.Installation{{Tool: "node", Version: "24.11.0", Path: home + "/.local/share/mise/installs/node/24.11.0/bin/node", OnPath: 1, Source: mise}},
			Managers: []inventory.Manager{{Name: "mise", Version: "2026.11.1", Activated: true}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Version: "Homebrew 4.6.3"}}},
	}
	a.Snapshot.Agents = []inventory.Agent{
		{ID: "claude", Installations: []inventory.Installation{{Tool: "claude", Version: "2.1.322", Path: home + "/.local/bin/claude", OnPath: 1}}},
		{ID: "cursor", App: "/Applications/Cursor.app", AppVersion: "3.29.9"},
	}
	a.router.Push(setupPage)
	return a
}

// Out of the wizard the page is the catalog: what is there with its
// version and source, nothing chosen until the user says, the rarer
// tools behind "N more", and a filter.
func TestInstallPage(t *testing.T) {
	a := installPageApp(t)
	tt := newTester(a)
	screenshot(t, tt, "install-page")
	hasTexts(t, tt, "Install", "Everything Hearth can install", "All", "Not installed", "Installed",
		"installed · 24.11.0", "mise · ~/.local/share/mise/installs/node/24.11.0/bin/node", "installed · 4.6.3", "installed · 2026.11.1", "installed · 2.1.322", "installed · 3.29.9", "/Applications/Cursor.app",
		"5 more", "Bun, PHP, Java, Ruby, .NET", "6 more", "installed: 1 of 9",
		"Switch on what to install. Hearth adds what it needs first, such as mise for languages, and shows the plan before anything changes.")
	for _, text := range []string{"Let's set up this computer", "required", "The package manager of PHP"} {
		if tt.HasText(text) {
			t.Errorf("%q on the page out of the wizard: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Install Go"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Clear selection", "Next: plan · 1", "Chosen 1: Go. Hearth installs them with what they need and shows the plan first. It leaves what is installed alone: its updates are on the Updates page.")
	if in := a.setupInput(); len(in.Parts()) != 1 || in.Parts()[0].Tool != "go" {
		t.Errorf("parts %+v", in.Parts())
	}
	if err := tt.Click("Next: plan · 1"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.setup.step != 1 || !tt.HasText("Step 2 of 4 · plan") {
		t.Fatalf("step %d: %q", a.setup.step, tt.Texts())
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Clear selection"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(a.setupChosenTools()) != 0 {
		t.Errorf("chosen %v", a.setupChosenTools())
	}
	// The rest of a group shows on asking.
	if err := tt.Click("show ›"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Bun", "Java")
	// The filter leaves what is there alone.
	a.setup.filter = 2
	tt.Frame()
	if tt.HasText("Install Go") || !tt.HasText("installed · 24.11.0") {
		t.Errorf("installed filter: %q", tt.Texts())
	}
}

// On a computer with nothing set up the page is the wizard, which offers
// a set to start with and the base.
func TestInstallPageWizardOnBareComputer(t *testing.T) {
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	a := cleanApp(t)
	a.Snapshot.Agents = nil
	a.router.Push(setupPage)
	tt := newTester(a)
	hasTexts(t, tt, "Let's set up this computer", "Step 1 of 4 · choice", "required", "Cancel")
	if !a.setupChosen("node") || !slices.ContainsFunc(a.setupInput().Parts(), func(p ops.SetupPart) bool { return p.Tool == "mise" }) {
		t.Errorf("the wizard's set: %+v", a.setupInput().Parts())
	}
}

// A language only the system brings shows on the page with its version,
// beside the switch that installs one to work with.
func TestInstallPageSystemCopy(t *testing.T) {
	a := installPageApp(t)
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "ruby", Installations: []inventory.Installation{
		{Tool: "ruby", Version: "2.6.10", Path: "/usr/bin/ruby", OnPath: 1, Source: inventory.Source{Kind: inventory.KindSystem}}}})
	tt := newTester(a)
	hasTexts(t, tt, "The system has its own 2.6.10: Hearth installs one to work with beside it.", "4 more", "Install Ruby")
}
