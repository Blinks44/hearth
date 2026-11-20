package ui

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// setupApp is a Mac with Node and Claude Code and nothing else, as the
// wizard's choice shows it.
func setupApp(t *testing.T) *App {
	t.Helper()
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "node", Installations: []inventory.Installation{{Tool: "node", Version: "24.11.0", Path: "/usr/local/bin/node", OnPath: 1}}},
		{Tool: "packages"},
	}, Agents: []inventory.Agent{{ID: "claude", Installations: []inventory.Installation{{Tool: "claude", Version: "2.0.1", Path: home + "/.local/bin/claude"}}}}}
	// As the Overview's offer opens it.
	a.setup.wizard = true
	a.router.Push(setupPage)
	return a
}

// Step 1 shows the catalog by group: the base required, what is there
// already, how each installs; switches and the engine change the choice,
// which the note counts with the base.
func TestSetupChoice(t *testing.T) {
	a := setupApp(t)
	tt := newTester(a)
	screenshot(t, tt, "setup-choice")
	hasTexts(t, tt, "Install", "Step 1 of 4 · choice", "Let's set up this computer",
		"Languages and runtimes", "installed: 1 of 9", "Node.js LTS", "installed · 24.11.0", "/usr/local/bin/node", "Python", "via mise", "via rustup, as Rust recommends",
		"Tools", "pnpm", "Docker engine", "With Colima, Hearth installs the engine", "Git", "via Xcode Command Line Tools",
		"Base · installs first", "Homebrew", "via the official installer", "required", "mise", "via Homebrew",
		"AI agents", "Claude Code", "Codex",
		// Homebrew, mise, Python, pnpm, Git, Colima: Node and Claude Code
		// are there.
		"Chosen 6, with the base. The next step shows the order, the commands and where the administrator's password is needed.")
	if err := tt.Click("Install Python"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Chosen 5, with the base. The next step shows the order, the commands and where the administrator's password is needed.")
	a.setup.engine = "orbstack"
	tt.Frame()
	hasTexts(t, tt, "OrbStack is paid for work at larger companies")
	if in := a.setupInput(); in.Parts()[len(in.Parts())-1].Method.Package != "orbstack" {
		t.Errorf("the engine chosen: %+v", in.Parts())
	}
}

// Next makes the plan of the choice in the background and shows it on
// step 2; the stepper and Back return to the choice.
func TestSetupNext(t *testing.T) {
	a := setupApp(t)
	goos = runtime.GOOS
	built := 0
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		built++
		done(build(sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, nil))
	}
	tt := newTester(a)
	if err := tt.Click("Next: plan"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if built != 1 || a.setup.step != 1 || len(a.setup.parts) == 0 {
		t.Fatalf("built %d, step %d, parts %+v", built, a.setup.step, a.setup.parts)
	}
	if a.setup.back {
		t.Error("the plan comes in as a step back")
	}
	hasTexts(t, tt, "Step 2 of 4 · plan", "Installation order")
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.setup.step != 0 {
		t.Error("Back stays on the plan")
	}
	if !a.setup.back {
		t.Error("the choice comes back in as a step forward")
	}
}

// setupPlan is a plan of the design's kind: Homebrew's package with the
// password, mise hooked into .zshrc, the Command Line Tools with
// Homebrew, Claude Code's installer adding a line to .zshrc too.
func setupPlan() (ops.Plan, []ops.SetupPart) {
	zshrc := home + "/.zshrc"
	first := ops.AddToBlock("", `eval "$(/opt/homebrew/bin/mise activate zsh)"`)
	p := ops.Plan{Title: "Set up this computer", Action: "Install 4", Steps: []ops.Step{
		{Argv: []string{"/bin/sh", "-c", "…"}, Note: "downloads Homebrew's installer"},
		{Argv: []string{"/bin/sh", "-c", "…"}, Note: "checks its signature"},
		{Argv: []string{"/usr/sbin/installer", "-pkg", "/tmp/Homebrew.pkg", "-target", "/"}, Admin: true},
		{Argv: []string{"/opt/homebrew/bin/brew", "install", "mise"}},
		{Edit: &ops.FileEdit{Path: zshrc, After: first, New: true}},
		{Argv: []string{"/bin/sh", "-c", "…"}, Note: "downloads Claude Code's own installer from claude.ai and runs it"},
		{Edit: &ops.FileEdit{Path: zshrc, Before: first, After: ops.AddToBlock(first, `export PATH="$HOME/.local/bin:$PATH"`)}},
	}, Notes: []ops.Note{{Title: "Administrator's password", Text: "once, for Homebrew's installer", Tag: "sudo"}}}
	return p, []ops.SetupPart{
		{Tool: "homebrew", From: 0, To: 3},
		{Tool: "mise", From: 3, To: 5},
		{Tool: "git", From: 5, To: 5, With: "homebrew"},
		{Tool: "claude", From: 5, To: 7},
	}
}

// Step 2 lists the tools in order with what runs and their tags, the file
// both mise and Claude Code change with its diff, and what to know;
// Install runs the plan.
func TestSetupPlanStep(t *testing.T) {
	a := setupApp(t)
	a.setup.plan, a.setup.parts = setupPlan()
	a.setup.step = 1
	var ran *ops.Plan
	a.RunPlan = func(p ops.Plan, update func(ops.State)) func() { ran = &p; return func() {} }
	tt := newTester(a)
	screenshot(t, tt, "setup-plan")
	hasTexts(t, tt, "What installs, and in what order", "Installation order", "4 steps",
		"1  Homebrew", "downloads Homebrew's installer", "password",
		"2  mise", "brew install mise · a line in ~/.zshrc", "changes a file",
		"3  Git", "part of the Command Line Tools, which install with Homebrew",
		"4  Claude Code", "downloads Claude Code's own installer from claude.ai and runs it · a line in ~/.zshrc",
		"Files that will change", "~/.zshrc", "mise, Claude Code · Hearth's block", "show diff",
		"Worth knowing", "Administrator's password", "Install 4")
	if err := tt.Click("show diff"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "setup-plan-diff")
	hasTexts(t, tt, "hide diff", `export PATH="$HOME/.local/bin:$PATH"`, `eval "$(/opt/homebrew/bin/mise activate zsh)"`)
	if err := tt.Click("Install 4"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if ran == nil || len(ran.Steps) != 7 || !a.op.running() {
		t.Errorf("Install ran %+v", ran)
	}
}

// installingApp is the wizard on step 3 with the design's plan, whose
// operation update and stopped drive.
func installingApp(t *testing.T) (a *App, tt *ui.Tester, update *func(ops.State), stopped *bool) {
	t.Helper()
	a = setupApp(t)
	a.setup.plan, a.setup.parts = setupPlan()
	a.setup.step = 1
	update, stopped = new(func(ops.State)), new(bool)
	a.RunPlan = func(p ops.Plan, u func(ops.State)) func() {
		*update = u
		return func() { *stopped = true }
	}
	a.OnScan = func() {}
	tt = newTester(a)
	if err := tt.Click("Install 4"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	return a, tt, update, stopped
}

// Step 3 shows each tool as its steps go, the log of the one running and
// what stays; Run in background leaves it to the operation's toast,
// whose Show comes back to the wizard.
func TestSetupInstallStep(t *testing.T) {
	a, tt, update, stopped := installingApp(t)
	if a.setup.step != 2 || a.op.open {
		t.Fatalf("Install shows step %d, the dialog open %v", a.setup.step, a.op.open)
	}
	(*update)(ops.State{Phase: ops.Running, Current: 3,
		Steps: []ops.StepStatus{ops.Done, ops.Done, ops.Done, ops.Busy, ops.Waiting, ops.Waiting, ops.Waiting},
		Log:   []string{"==> Pouring mise--2025.11.0.arm64_sequoia.bottle.tar.gz"}})
	tt.Frame()
	screenshot(t, tt, "setup-install")
	hasTexts(t, tt, "Step 3 of 4 · install", "Installing", "Step 2 of 4 · mise. You can run it in the background: the installation goes on.",
		"1  Homebrew", "done", "2  mise", "running", "3  Git", "waiting", "Log · step 2", "==> Pouring mise--2025.11.0.arm64_sequoia.bottle.tar.gz",
		"The password is no longer needed", "it was asked at step 1", "You can leave this page",
		"Installed already: Homebrew. They stay even if you stop now. If a step fails, Hearth shows why.")
	if tt.HasText("Show") {
		t.Error("the toast shows over the wizard")
	}
	if err := tt.Click("Stop"); err != nil {
		t.Fatal(err)
	}
	if !*stopped {
		t.Error("Stop did not stop the plan")
	}
	if err := tt.Click("Run in background"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// Away from the header's tooltip, which takes the first click.
	tt.Move(600, 600)
	tt.Advance(time.Second)
	// The toast counts the wizard's tools, as its page.
	hasTexts(t, tt, "Step 2 of 4 · mise")
	if err := tt.Click("Show"); err != nil {
		t.Fatalf("no toast away from the wizard: %v", err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.router.Path() != setupPage || a.op.open {
		t.Errorf("Show went to %s, the dialog open %v", a.router.Path(), a.op.open)
	}
}

// A step that failed says so with its output; the plan made again
// leaves out what installed.
func TestSetupInstallFailed(t *testing.T) {
	a, tt, update, _ := installingApp(t)
	built := 0
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) { built++ }
	(*update)(ops.State{Phase: ops.Failed, Current: 3, ExitCode: 1,
		Steps: []ops.StepStatus{ops.Done, ops.Done, ops.Done, ops.StepFailed, ops.Skipped, ops.Skipped, ops.Skipped},
		Log:   []string{"Error: No such keg: mise"}})
	if !a.Scanning {
		t.Error("no scan after the installation")
	}
	a.Scanning = false
	tt.Frame()
	screenshot(t, tt, "setup-failed")
	hasTexts(t, tt, "mise did not install", "Why: Error: No such keg: mise. The tools after it did not install.", "failed", "skipped", "Error: No such keg: mise", "Details",
		"Installed and staying: Homebrew. The plan made again leaves them out.")
	if err := tt.Click("Make the plan again"); err != nil {
		t.Fatal(err)
	}
	if built != 1 {
		t.Error("the plan was not made again")
	}
}

// Once the plan went through, step 4 waits for the scan, which reads the
// terminal's environment again, then shows each tool's version and path
// in it, and a tool it does not find.
func TestSetupCheckStep(t *testing.T) {
	a, tt, update, _ := installingApp(t)
	done := make([]ops.StepStatus, 7)
	for i := range done {
		done[i] = ops.Done
	}
	(*update)(ops.State{Phase: ops.Succeeded, Current: 6, Steps: done})
	tt.Frame()
	tt.Advance(time.Second) // the step comes in once the installation's has gone
	if !a.Scanning || a.setup.step != 3 {
		t.Fatalf("scanning %v, step %d", a.Scanning, a.setup.step)
	}
	hasTexts(t, tt, "Step 4 of 4 · check", "Checking in a new terminal", "checking")
	a.Scanning = false
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Version: "4.4.22", Path: "/opt/homebrew/bin/brew"}}},
		// The scan tells mise by the tools it keeps, not as a copy of its own.
		{Tool: "node", Managers: []inventory.Manager{{Name: "mise", Version: "2025.11.0", Activated: true}}},
	}, Agents: []inventory.Agent{{ID: "claude", Installations: []inventory.Installation{{Tool: "claude", Version: "2.0.1", Path: home + "/.local/bin/claude", OnPath: 1}}}}}
	tt.Frame()
	screenshot(t, tt, "setup-check")
	hasTexts(t, tt, "Not everything works yet", "Check in a new terminal", "3 of 4 work",
		"1  Homebrew", "4.4.22 · /opt/homebrew/bin/brew", "works", "2025.11.0 · hooked into the shell",
		"3  Git", "not on the terminal's PATH", "not found", "2.0.1 · ~/.local/bin/claude",
		"What's next", "Open a new terminal", "What changed", "~/.zshrc")
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.setup.op != nil || a.router.Path() != overview.path {
		t.Errorf("Done left the wizard at %s", a.router.Path())
	}
}

// A language only the system brings, as macOS's Ruby 2.6, is not there
// for the wizard, which installs one through mise; a copy of the user's
// own is; on Linux, PHP from the system's manager is.
func TestSetupHaveSystemCopy(t *testing.T) {
	a := setupApp(t)
	system := inventory.Source{Kind: inventory.KindSystem}
	a.Snapshot.Tools = append(a.Snapshot.Tools,
		inventory.ToolState{Tool: "ruby", Installations: []inventory.Installation{{Tool: "ruby", Version: "2.6.10", Path: "/usr/bin/ruby", OnPath: 1, Source: system}}},
		inventory.ToolState{Tool: "php", Installations: []inventory.Installation{{Tool: "php", Version: "8.3.6", Path: "/usr/bin/php", OnPath: 1, Source: system}}},
	)
	if a.setupHave("ruby") {
		t.Error("the system's Ruby counts as installed")
	}
	if !a.setupHave("node") {
		t.Error("Node from an installer does not count")
	}
	goos = "linux"
	if !a.setupHave("php") {
		t.Error("PHP from APT on Linux does not count")
	}
}

// Docker is offered where its first engine is open source; Docker
// Desktop, the one on Windows, waits for the user.
func TestSetupDockerOffered(t *testing.T) {
	a := setupApp(t)
	if !a.setupChosen("docker") {
		t.Error("Colima is not offered on macOS")
	}
	goos = "windows"
	if a.setupChosen("docker") {
		t.Error("Docker Desktop is offered on Windows")
	}
	a.setup.choice = map[string]bool{"docker": true}
	if !a.setupChosen("docker") {
		t.Error("the user's choice does not count")
	}
}

// Docker Engine on Linux refuses the user until they sign in again into
// the docker group: the check says so, and what's next how.
func TestSetupCheckDockerDenied(t *testing.T) {
	a := setupApp(t)
	goos = "linux"
	a.setup.step, a.setup.parts = 3, []ops.SetupPart{{Tool: "docker"}}
	a.setup.op = &operation{started: true, state: ops.State{Phase: ops.Succeeded}}
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "docker",
		Installations: []inventory.Installation{{Tool: "docker", Version: "29.1.3", Path: "/usr/bin/docker", OnPath: 1}},
		Docker:        &inventory.DockerState{Selected: "default", Contexts: []inventory.DockerContext{{Name: "default", Status: "unavailable", Denied: true}}}})
	tt := newTester(a)
	hasTexts(t, tt, "Done, one thing left", "after signing in again", "Sign out and back in", "then docker works without sudo")
}
