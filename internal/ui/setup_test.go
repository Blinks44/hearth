package ui

import (
	"runtime"
	"testing"
	"time"

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
	hasTexts(t, tt, "New computer", "Step 1 of 4 · choice", "Let's set up this computer",
		"Languages and runtimes", "installed: 1", "Node.js LTS", "already there", "Python", "via mise", "via rustup, as Rust recommends",
		"Tools", "pnpm", "Docker engine", "Colima is open source", "Git", "via Xcode Command Line Tools",
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
	hasTexts(t, tt, "Step 2 of 4 · plan", "Installation order")
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.setup.step != 0 {
		t.Error("Back stays on the plan")
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
		{Tool: "git", From: 5, To: 5},
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
		"3  Git", "comes with Homebrew's installer",
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
