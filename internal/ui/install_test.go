package ui

import (
	"context"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
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

// Out of the wizard the page is the categorized catalog: available tools
// first, installed tools summarized cleanly at the bottom of each category
// (and expandable or openable), a right-hand selection queue, and category
// and status filters.
func TestInstallPage(t *testing.T) {
	a := installPageApp(t)
	a.SaveSet = func([]byte, func(string, error)) {}
	a.OpenSet = func(func(string, []byte, error)) {}
	tt := newTester(a)
	screenshot(t, tt, "install-page")
	hasTexts(t, tt, "Install", "Everything Hearth can install", "All", "Not installed", "Installed",
		"Languages", "Packages & Git", "Containers & cloud", "Databases", "AI agents", "Terminal & CLI",
		"Installed (1): Node.js LTS 24.11.0", "Installed (2): Claude Code 2.1.322, Cursor 3.29.9",
		"installed · 4.6.3", "installed · 2026.11.1",
		"3 more", "Deno, Zig, Elixir", "installed: 1 of 12",
		"Temurin 25", "17", "8.4", "latest",
		"To install", "Computer set",
		"Switch on what to install. Hearth adds what it needs first, such as mise for languages, and shows the plan before anything changes.")
	for _, text := range []string{"Let's set up this computer", "required", "Manages Ruby gems"} {
		if tt.HasText(text) {
			t.Errorf("%q on the page out of the wizard: %q", text, tt.Texts())
		}
	}
	// Choosing Go highlights it, shows it in the queue card, and enables Next.
	if err := tt.Click("Install Go"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "install-page-selected")
	hasTexts(t, tt, "Clear selection", "Next: plan · 1", "Remove Go",
		"Chosen 1: Go. Hearth installs them with what they need and shows the plan first. It leaves what is installed alone: its updates are on the Updates page.")
	if in := a.setupInput(); len(in.Parts()) != 1 || in.Parts()[0].Tool != "go" {
		t.Errorf("parts %+v", in.Parts())
	}
	// Removing from the right-hand queue card clears the tool cleanly.
	if err := tt.Click("Remove Go"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(a.setupChosenTools()) != 0 {
		t.Errorf("Remove Go left chosen %v", a.setupChosenTools())
	}
	// Choose Go again and walk to Step 2 and back.
	if err := tt.Click("Install Go"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
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
	hasTexts(t, tt, "Deno", "Zig", "Elixir")
	// Expanding the compact installed summary shows full path and tool link.
	if err := tt.Click("Installed (1): Node.js LTS 24.11.0"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "installed · 24.11.0", "mise · ~/.local/share/mise/installs/node/24.11.0/bin/node", "hide")
	// Category chip filters down to one category.
	if err := tt.Click("Terminal & CLI"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "ripgrep", "direnv")
	if tt.HasText("Install Python") {
		t.Errorf("category chip left Python visible: %q", tt.Texts())
	}
	a.setup.cat = catAll
	tt.Frame()
	// On the Not installed tab, items are not collapsed behind "N more".
	a.setup.filter = 1
	tt.Frame()
	hasTexts(t, tt, "ripgrep", "k9s", "direnv")
	if tt.HasText("show ›") {
		t.Errorf("filtered tab still collapses behind show ›: %q", tt.Texts())
	}
	// The Installed filter leaves what is there alone and opens a tool page on click.
	a.setup.filter = 2
	tt.Frame()
	screenshot(t, tt, "install-page-installed")
	if tt.HasText("Install Go") || !tt.HasText("installed · 24.11.0") || !tt.HasText("installed · 2.1.322") || !tt.HasText("/Applications/Cursor.app") {
		t.Errorf("installed filter: %q", tt.Texts())
	}
	if err := tt.Click("Open Node.js LTS"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/tools/node" {
		t.Errorf("Open Node.js LTS navigated to %q, want /tools/node", a.router.Path())
	}
	a.router.Push(setupPage)
	// Empty search renders full-width Nothing matches note.
	a.setup.filter, a.setup.query = 0, "no-such-tool-xyz"
	tt.Frame()
	hasTexts(t, tt, "Nothing matches")
	if r, ok := tt.Find("Nothing matches"); !ok || r.X > 360 {
		t.Errorf("Nothing matches rendered at %+v (found=%v), expected full-width left-aligned card", r, ok)
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
	hasTexts(t, tt, "The system has its own 2.6.10: Hearth installs one to work with beside it.", "3 more", "Install Ruby")
}

// A set saved on one computer and opened on a clean one makes the plan
// of the same tools; opened where everything is there, it makes none.
func TestInstallSet(t *testing.T) {
	a := installPageApp(t)
	var saved []byte
	a.SaveSet = func(data []byte, done func(string, error)) {
		saved = data
		done("/Users/me/hearth-set.json", nil)
	}
	tt := newTester(a)
	if err := tt.Click("More"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("Save my set…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := []string{"homebrew", "mise", "node", "claude", "cursor"}
	if got, err := catalog.ParseSet(saved); err != nil || !slices.Equal(got, want) {
		t.Fatalf("saved %q, %v; want %q", got, err, want)
	}

	b := cleanApp(t)
	b.Snapshot.Agents = nil
	b.router.Push(setupPage)
	b.OpenSet = func(done func(string, []byte, error)) { done("/Volumes/USB/hearth-set.json", saved, nil) }
	tb := newTester(b)
	if err := tb.Click("Open a set…"); err != nil {
		t.Fatalf("%v: %q", err, tb.Texts())
	}
	tb.Advance(time.Second)
	// Opening a set previews the chosen tools on Step 1 before building the plan.
	if b.setup.step != 0 {
		t.Fatalf("step %d, want 0 preview: %q", b.setup.step, tb.Texts())
	}
	hasTexts(t, tb, "Loaded from set: 5 tools selected. Review the selection and continue to the plan.", "Cursor")
	screenshot(t, tb, "s13-04-install-set-preview-1280")
	if err := tb.Click("Next: plan"); err != nil {
		t.Fatalf("%v: %q", err, tb.Texts())
	}
	tb.Advance(time.Second)
	if b.setup.step != 1 {
		t.Fatalf("step %d: %q", b.setup.step, tb.Texts())
	}
	var planned []string
	for _, p := range b.setup.parts {
		planned = append(planned, p.Tool)
	}
	if !slices.Equal(planned, want) {
		t.Errorf("plan of %q, want %q", planned, want)
	}

	// The computer that saved it has it all.
	a.OpenSet = func(done func(string, []byte, error)) { done("/Users/me/hearth-set.json", saved, nil) }
	a.openSet()
	if a.setup.step != 0 || a.notice != "Everything in the set is on this computer already." {
		t.Errorf("step %d, notice %q", a.setup.step, a.notice)
	}
}

// Tools of a set that do not install here are left out and said so,
// but for another system's base; a file Hearth did not save is not a
// set.
func TestInstallSetElsewhere(t *testing.T) {
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "windows"
	a := cleanApp(t)
	data, _ := catalog.MarshalSet([]string{"homebrew", "redis", "git"})
	a.OpenSet = func(done func(string, []byte, error)) { done(`C:\set.json`, data, nil) }
	a.openSet()
	if a.notice != "Left out, as they do not install here: Redis" {
		t.Errorf("notice %q", a.notice)
	}
	if a.setup.step != 0 || !a.setupChosen("git") || a.setupChosen("redis") {
		t.Errorf("step=%d git=%v redis=%v", a.setup.step, a.setupChosen("git"), a.setupChosen("redis"))
	}
	a.setupNext()
	if n := len(a.setup.parts); n == 0 || a.setup.parts[n-1].Tool != "git" {
		t.Errorf("parts %+v", a.setup.parts)
	}
	a.OpenSet = func(done func(string, []byte, error)) { done("/tmp/notes.json", []byte(`{"notes": []}`), nil) }
	a.openSet()
	if a.planErr != "notes.json is not a set saved by Hearth." {
		t.Errorf("error %q", a.planErr)
	}
}

func TestInstallSearch(t *testing.T) {
	a := installPageApp(t)
	tt := newTester(a)
	screenshot(t, tt, "s13-04-install-1280")
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "s13-04-install-1000")
	// Terraform is normally collapsed behind "N more", but search reveals it immediately.
	a.setup.query = "terraform"
	tt.Frame()
	hasTexts(t, tt, "Terraform")
	screenshot(t, tt, "s13-04-install-search-1280")
	if tt.HasText("Python") {
		t.Errorf("unrelated tool stayed visible during search: %q", tt.Texts())
	}
	a.setup.query = "no-such-tool-xyz"
	tt.Frame()
	hasTexts(t, tt, "Nothing matches")
}

// In Russian and dark mode, category chips, tool icons, selection queue with
// automatic dependencies, and the Computer set card render cleanly in both
// wide and narrow windows.
func TestInstallPageRussianAndDark(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := installPageApp(t)
	a.Locale = "ru"
	a.SaveSet = func([]byte, func(string, error)) {}
	a.OpenSet = func(func(string, []byte, error)) {}
	// Leave mise uninstalled so selecting Python surfaces automatic dependency.
	a.Snapshot.Tools[0].Managers = nil
	a.setup.choice = map[string]bool{"python": true, "rust": true}
	tt := newTester(a)
	screenshot(t, tt, "install-ru-light-1280")
	hasTexts(t, tt, "Установка", "Языки", "Пакеты и Git", "Контейнеры и облака", "Базы данных", "AI-агенты", "Терминал и CLI",
		"К установке", "ПОСТАВИТСЯ АВТОМАТИЧЕСКИ", "mise", "Набор компьютера", "Уже установлено (1): Node.js LTS 24.11.0")
	tt.SetDark(true)
	tt.Frame()
	screenshot(t, tt, "install-ru-dark-1280")
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "install-ru-light-1000")
}
