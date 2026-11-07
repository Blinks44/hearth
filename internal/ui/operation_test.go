package ui

import (
	"io"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
)

func dockerPlan() ops.Plan {
	return ops.Plan{
		Title:  "Install Docker Desktop",
		Why:    "Through Homebrew, as your other apps. It asks for the administrator's password.",
		Action: "Accept the terms and install",
		Steps: []ops.Step{
			{Argv: []string{"brew", "install", "--cask", "docker"}},
			{Argv: []string{"open", "-a", "Docker"}, Note: "wait for the engine to start"},
			{Argv: []string{"docker", "context", "use", "desktop-linux"}},
		},
		Notes: []ops.Note{
			{Title: "Administrator rights", Text: "macOS asks for the password: Docker installs a system helper", Tag: "sudo"},
			{Title: "Docker's terms of use", Text: "free for personal use and companies under 250 people", URL: "https://www.docker.com/legal/docker-subscription-service-agreement/"},
		},
		Changes: []string{"+ /Applications/Docker.app", "+ context desktop-linux, now in use"},
	}
}

// The operation dialog shows the plan before it runs, the steps as they
// run, and how it ended; hidden, a toast shows it goes on.
func TestOperationDialog(t *testing.T) {
	a := NewApp()
	a.Snapshot = found("node")
	scans := 0
	a.OnScan = func() { scans++ }
	var update func(ops.State)
	runs, stops := 0, 0
	a.RunPlan = func(p ops.Plan, u func(ops.State)) func() {
		runs++
		update = u
		return func() { stops++ }
	}
	tt := ui.NewTester(a.View, 1280, 820)
	a.Propose(dockerPlan())
	tt.Frame()
	tt.Advance(time.Second)
	for _, text := range []string{"Install Docker Desktop", "brew install --cask docker", "open -a Docker  ·  wait for the engine to start", "WORTH KNOWING", "sudo", "Accept the terms and install"} {
		if !tt.HasText(text) {
			t.Fatalf("plan misses %q: %q", text, tt.Texts())
		}
	}
	screenshot(t, tt, "operation-plan")
	if err := tt.Click("Accept the terms and install"); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("confirming ran %d times", runs)
	}

	update(ops.State{Phase: ops.Running, Steps: []ops.StepStatus{ops.Busy, ops.Waiting, ops.Waiting},
		Log: []string{"==> Downloading https://desktop.docker.com/mac/main/arm64/Docker.dmg", "######## 64.4%"}})
	tt.Frame()
	tt.Advance(time.Second)
	for _, text := range []string{"Step 1 of 3 · can be stopped at any time", "running", "waiting", "LOG · brew install --cask docker", "######## 64.4%"} {
		if !tt.HasText(text) {
			t.Fatalf("running misses %q: %q", text, tt.Texts())
		}
	}
	screenshot(t, tt, "operation-running")
	if err := tt.Click("Stop"); err != nil || stops != 1 {
		t.Fatalf("Stop: %v, stopped %d times", err, stops)
	}

	if err := tt.Click("Run in background"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("LOG · brew install --cask docker") || !tt.HasText("Install Docker Desktop…") {
		t.Fatalf("hidden, the toast does not show instead of the dialog: %q", tt.Texts())
	}
	screenshot(t, tt, "operation-toast")
	if err := tt.Click("Show"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	update(ops.State{Phase: ops.Failed, Steps: []ops.StepStatus{ops.StepFailed, ops.Skipped, ops.Skipped}, ExitCode: 1,
		Log: []string{"curl: (6) Could not resolve host: desktop.docker.com", "Error: Download failed on Cask 'docker'"}})
	tt.Frame()
	tt.Advance(time.Second)
	for _, text := range []string{"Step 1 failed: exit code 1. The steps after it did not run.", "failed", "skipped", "Error: Download failed on Cask 'docker'", "Copy log"} {
		if !tt.HasText(text) {
			t.Fatalf("failed misses %q: %q", text, tt.Texts())
		}
	}
	if scans != 1 {
		t.Errorf("scanned %d times after the end, want 1", scans)
	}
	screenshot(t, tt, "operation-failed")
	if err := tt.Click("Retry"); err != nil || runs != 2 {
		t.Fatalf("Retry: %v, ran %d times", err, runs)
	}

	update(ops.State{Phase: ops.Succeeded, Steps: []ops.StepStatus{ops.Done, ops.Done, ops.Done}, Current: 2, Verified: "docker 27.4.1 answers"})
	tt.Frame()
	tt.Advance(time.Second)
	for _, text := range []string{"Done. Checked: docker 27.4.1 answers", "done", "WHAT CHANGED", "+ /Applications/Docker.app"} {
		if !tt.HasText(text) {
			t.Fatalf("done misses %q: %q", text, tt.Texts())
		}
	}
	screenshot(t, tt, "operation-done")
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("WHAT CHANGED") {
		t.Error("Done does not close the dialog")
	}
}

// A stop says which step and what stays; an operation that ends hidden
// says so in a toast that shows it again.
func TestOperationStoppedHidden(t *testing.T) {
	a := NewApp()
	var update func(ops.State)
	a.RunPlan = func(p ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
	tt := ui.NewTester(a.View, 1280, 820)
	a.Propose(dockerPlan())
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Accept the terms and install"); err != nil {
		t.Fatal(err)
	}
	update(ops.State{Phase: ops.Running, Current: 1, Steps: []ops.StepStatus{ops.Done, ops.Busy, ops.Waiting}})
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Run in background"); err != nil {
		t.Fatal(err)
	}
	update(ops.State{Phase: ops.Stopped, Current: 1, Steps: []ops.StepStatus{ops.Done, ops.StepStopped, ops.Skipped}, ExitCode: -1})
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Install Docker Desktop: stopped") {
		t.Fatalf("no toast of the end: %q", tt.Texts())
	}
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, text := range []string{"You stopped it at step 2. The steps after it did not run.", "WHAT STAYS", "Step 1 finished: what it changed stays", "Start again"} {
		if !tt.HasText(text) {
			t.Fatalf("stopped misses %q: %q", text, tt.Texts())
		}
	}
	screenshot(t, tt, "operation-stopped")
}

// With the engine behind it, a plan runs its commands and the dialog
// shows the result.
func TestOperationRunsPlan(t *testing.T) {
	a := NewApp()
	f := &run.Fake{}
	f.Stdout("installed\n", "brew", "install", "--cask", "docker")
	f.Stdout("", "open", "-a", "Docker")
	f.Stdout("desktop-linux\n", "docker", "context", "use", "desktop-linux")
	a.RunPlan = func(p ops.Plan, u func(ops.State)) func() {
		ops.Execute(t.Context(), f, p, io.Discard, u)
		return func() {}
	}
	tt := ui.NewTester(a.View, 1280, 820)
	a.Propose(dockerPlan())
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Accept the terms and install"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(f.Calls()) != 3 || !tt.HasText("Done.") {
		t.Errorf("ran %d commands, texts %q", len(f.Calls()), tt.Texts())
	}
}

// A plan that edits a file shows the lines that change, the file copy it
// makes, and once it did not get through, that the file came back.
func TestOperationEditDialog(t *testing.T) {
	a := NewApp()
	a.Home = "/Users/ann"
	a.Shell = &fakeShell{}
	var update func(ops.State)
	a.RunPlan = func(p ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
	before := "export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\"\nalias ll='ls -l'\nexport PATH=\"/opt/homebrew/bin:$PATH\"\n"
	after := ops.CommentOut(before, []int{1}, "no such folder")
	after = ops.CommentOut(after, []int{3}, "already in ~/.zprofile")
	zshrc := "/Users/ann/.zshrc"
	tt := ui.NewTester(a.View, 1280, 820)
	a.Propose(ops.Plan{Title: "Fix PATH in ~/.zshrc", Why: "Two lines become comments, not removed. Hearth keeps a copy of the file first.",
		Action: "Comment out 2 lines", Steps: []ops.Step{{Edit: &ops.FileEdit{Path: zshrc, Before: before, After: after}}}})
	tt.Frame()
	tt.Advance(time.Second)
	for _, text := range []string{"~/.zshrc", "Open file", "WHAT WILL CHANGE · 2 LINES", "File copy", "backup",
		"+ # hearth: no such folder · export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\""} {
		if !tt.HasText(text) {
			t.Fatalf("plan misses %q: %q", text, tt.Texts())
		}
	}
	if tt.HasText("COMMANDS · RUN IN ORDER") {
		t.Error("a plan of edits alone lists commands")
	}
	screenshot(t, tt, "operation-edit")
	if err := tt.Click("Comment out 2 lines"); err != nil {
		t.Fatal(err)
	}
	update(ops.State{Phase: ops.Stopped, Steps: []ops.StepStatus{ops.Done}, Restored: []string{zshrc}})
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("~/.zshrc is back as it was, from its copy") || tt.HasText("Step 1 finished: what it changed stays") {
		t.Errorf("stopped: %q", tt.Texts())
	}
}
