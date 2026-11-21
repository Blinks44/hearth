package ui

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
	for _, text := range []string{"Step 1 failed: Error: Download failed on Cask 'docker'. The steps after it did not run.", "failed", "skipped", "Error: Download failed on Cask 'docker'", "Copy log"} {
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
	// A check that does not pass puts the file back too, and says so.
	if err := tt.Click("Start again"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	update(ops.State{Phase: ops.NotVerified, Steps: []ops.StepStatus{ops.Done}, Err: "the order changed", Restored: []string{zshrc}})
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("~/.zshrc is back as it was, from its copy") {
		t.Errorf("not verified: %q", tt.Texts())
	}
	screenshot(t, tt, "operation-edit-restored")
}

func TestOperationFileChanged(t *testing.T) {
	for _, tc := range []struct {
		lang, caption, close, confirm, retry, copyLog string
		width, height                                 int
	}{
		{
			lang: "en", width: 1000, height: 680,
			caption: "The file changed after the preview. Close this window and start the action again to review a new plan.",
			close:   "Close", confirm: "Connect", retry: "Retry", copyLog: "Copy log",
		},
		{
			lang: "ru", width: 1280, height: 820,
			caption: "Файл изменился после предпросмотра. Закройте окно и начните действие заново, чтобы увидеть новый план.",
			close:   "Закрыть", confirm: "Подключить", retry: "Повторить", copyLog: "Скопировать лог",
		},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return tc.lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			a := NewApp()
			a.Home = "/Users/ann"
			var update func(ops.State)
			a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() {
				update = u
				return func() {}
			}
			tt := ui.NewTester(a.View, tc.width, tc.height)
			const path = "/.gemini/config/mcp_config.json"
			a.Propose(ops.Plan{Title: "Sketch MCP · Antigravity CLI", Action: tc.confirm,
				Steps: []ops.Step{{Edit: &ops.FileEdit{Path: a.Home + path, After: "{}\n"}}}})
			tt.Frame()
			tt.Advance(time.Second)
			if err := tt.Click(tc.confirm); err != nil {
				t.Fatal(err)
			}
			errText := path + ": " + ops.ErrChanged.Error()
			update(ops.State{Phase: ops.Failed, Steps: []ops.StepStatus{ops.StepFailed}, FileChanged: true, Err: a.Home + errText})
			tt.Frame()
			tt.Advance(time.Second)
			hasTexts(t, tt, tc.caption, tc.copyLog, "~/.gemini/config/mcp_config.json")
			if tt.HasText(tc.retry) {
				t.Fatalf("stale plan offers retry: %q", tt.Texts())
			}
			screenshot(t, tt, "operation-file-changed-"+tc.lang)
			if err := tt.Click(tc.copyLog); err != nil {
				t.Fatal(err)
			}
			if got := tt.Clipboard(); got != "~"+errText {
				t.Errorf("copied log = %q, want %q", got, "~"+errText)
			}
			if err := tt.Click(tc.close); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if tt.HasText(tc.caption) {
				t.Error("Close does not dismiss the stale plan")
			}
		})
	}
}

func TestFailReason(t *testing.T) {
	tests := []struct {
		name string
		st   ops.State
		want string
	}{
		{"error", ops.State{Err: "refused", Log: []string{"$ brew install mise", "oops"}}, "refused"},
		{"last line of the step", ops.State{ExitCode: 1, Log: []string{"$ a", "earlier", "$ brew install mise", "Error: No such keg: mise.", ""}}, "Error: No such keg: mise"},
		{"no network", ops.State{ExitCode: 28, Log: []string{"$ /bin/sh -c …", "curl: (28) Failed to connect to api.github.com port 443 after 75073 ms: Couldn't connect to server"}}, "no connection to api.github.com, check the internet"},
		{"no host", ops.State{ExitCode: 6, Log: []string{"$ /bin/sh -c …", "curl: (6) Could not resolve host: mise.run"}}, "no connection to mise.run, check the internet"},
		{"usage hint", ops.State{ExitCode: 1, Log: []string{"$ kind create cluster", "Command Output: docker: Error response from daemon: Conflict.", "", "Run 'docker run --help' for more information"}}, "Command Output: docker: Error response from daemon: Conflict"},
		{"no output", ops.State{ExitCode: 2, Log: []string{"$ a", "earlier", "$ b"}}, "exit code 2"},
		{"HRESULT", ops.State{ExitCode: 0x8a150006, Log: []string{"$ winget install"}}, "exit code 0x8a150006"},
		{"WinGet cancelled", ops.State{ExitCode: 0x8a15010c, Log: []string{"$ winget install"}},
			"the installation was canceled, as when Windows' permission request (UAC) is declined or left unanswered (0x8a15010c)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := failReason(tc.st); got != tc.want {
				t.Errorf("failReason = %q, want %q", got, tc.want)
			}
		})
	}
}

// A variable set in the Windows registry shows as a setting edited, its
// value hidden.
func TestStepEditUserVar(t *testing.T) {
	e, setting, ok := stepEdit(ops.Step{UserVar: &ops.UserVarEdit{Name: "LINEAR_TOKEN", Value: "lin_s3cr3t"}})
	if !ok || !setting || e.Path != `HKCU\Environment\LINEAR_TOKEN` {
		t.Fatalf("stepEdit: %+v %v %v", e, setting, ok)
	}
	if shown := e.Masked(e.After); strings.Contains(shown, "s3cr3t") || !strings.Contains(shown, "LINEAR_TOKEN=") {
		t.Errorf("shown: %q", shown)
	}
}

func TestSkillStoragePreview(t *testing.T) {
	a := NewApp()
	a.Snapshot = found("node")
	plan := ops.Plan{Title: "Merge copies of 1 skill", Why: "Other copies move to backups; their paths become links.", Steps: []ops.Step{{Argv: []string{"mv", "/home/dev/.claude/skills/rex", "/home/dev/backups/rex"}}, {Argv: []string{"ln", "-s", "/home/dev/.agents/skills/rex", "/home/dev/.claude/skills/rex"}}}, Notes: []ops.Note{{Title: "Shared skill folder · rex", Text: "/home/dev/.agents/skills/rex"}, {Title: "Links to the shared folder", Text: "/home/dev/.claude/skills/rex → /home/dev/.agents/skills/rex"}}}
	a.Propose(plan)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Shared skill folder · rex", "/home/dev/.agents/skills/rex", "Links to the shared folder")
	screenshot(t, tt, "skill-storage-preview")
}

// All terminal outcomes refresh inventory, and discard sizes from before the
// operation. Repeated final notifications cannot restart the refresh.
func TestOperationInvalidatesSizes(t *testing.T) {
	for _, phase := range []ops.Phase{ops.Succeeded, ops.Failed, ops.Stopped, ops.NotVerified} {
		t.Run(fmt.Sprint(phase), func(t *testing.T) {
			a := NewApp()
			a.Sizes = &inventory.Sizes{Paths: map[string]inventory.Usage{"old": {Bytes: 11e9}}}
			scans := 0
			a.OnScan = func() { scans++ }
			var update func(ops.State)
			a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() { update = u; return func() {} }
			a.Propose(dockerPlan())
			a.startOperation()
			update(ops.State{Phase: phase})
			update(ops.State{Phase: phase})
			if scans != 1 || !a.Scanning || !a.Counting || a.Sizes != nil {
				t.Fatalf("refresh: scans=%d scanning=%v counting=%v sizes=%v", scans, a.Scanning, a.Counting, a.Sizes)
			}
		})
	}
}
