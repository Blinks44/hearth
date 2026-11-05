package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// pythonSnapshot is Homebrew's python3 first in PATH, two uv Pythons off
// it, and the system's behind Homebrew's; uv set up with its defaults.
func pythonSnapshot() *inventory.Snapshot {
	uvDir := home + "/.local/share/uv/python/"
	uv := func(v string) inventory.Installation {
		root := uvDir + "cpython-" + v + "-macos-aarch64-none"
		return inventory.Installation{Tool: "python", Version: v, Path: root + "/bin/python3", RealPath: root + "/bin/python3",
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "uv", Root: root, Version: v}}
	}
	tools := []inventory.Package{
		{Name: "ruff", Version: "0.9.2", Commands: []string{"ruff"}},
		{Name: "httpie", Version: "3.2.4", Commands: []string{"http", "https"}},
	}
	ins := []inventory.Installation{
		{Tool: "python", Version: "3.13.1", Path: "/opt/homebrew/bin/python3",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/python@3.13/3.13.1"},
			OnPath: 1, InGUI: 1},
		uv("3.12.8"), uv("3.11.11"),
		{Tool: "python", Version: "3.9.6", Path: "/usr/bin/python3",
			Source: inventory.Source{Kind: inventory.KindSystem, Root: "/usr"}, OnPath: 2, InGUI: 2},
	}
	for i := range ins {
		ins[i].Packages = tools
	}
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "python", Installations: ins,
		Managers: []inventory.Manager{{Name: "uv", Version: "0.5.21",
			Settings: map[string]string{"python-downloads": "automatic", "python-preference": "managed"}}}}}}
}

// pythonLines are Python's lines around now: 3.9 out of support.
func pythonLines(now time.Time) *versions.Python {
	year := func(n int) time.Time { return now.AddDate(n, 0, 0) }
	return &versions.Python{Lines: []versions.PythonLine{
		{Cycle: "3.15", Latest: "3.15.0a1", Release: year(1)},
		{Cycle: "3.14", Latest: "3.14.0", Release: year(-1), End: year(4)},
		{Cycle: "3.13", Latest: "3.13.2", Release: year(-2), End: year(3)},
		{Cycle: "3.12", Latest: "3.12.9", Release: year(-3), End: year(2)},
		{Cycle: "3.9", Latest: "3.9.21", Release: year(-6), End: year(-1)},
	}}
}

func openPython(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	a.Home = home
	a.router.Push("/tools/python")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

// The page shows Homebrew's copy in use, uv's off PATH and the system's
// behind it, needed by the OS; the lines still supported as they stand to
// those; uv's settings; and the tools of uv tool and pipx.
func TestPythonPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = pythonSnapshot()
	tt := openPython(t, a)
	a.PythonVersions = pythonLines(tt.Now())
	tt.Frame()
	screenshot(t, tt, "python")
	y := func(n int) string { return "supported until " + tt.Now().AddDate(n, 0, 0).Format("2006") }
	for _, s := range []string{
		"Versions managed by uv · also from brew, system", "4 copies", "brew · uv · system", pythonCommand + " → brew",
		"3.13.1", "3.12.8", "3.11.11", "3.9.6", y(3), y(2), "support ended", "needed by the system", "in PATH",
		"The system's Python is needed by the OS itself: Hearth does not remove or update it.",
		"How the terminal finds " + pythonCommand, whichAllCommand + " " + pythonCommand,
		"Available to install", "3.14.0", "new version", "3.13.2", "update for 3.13.1", "recommended", "3.12.9", "update for 3.12.8",
		"Version manager · uv", "0.5.21", "Downloads Python itself", "Prefers its own Pythons",
		"Tools", "uv tool · pipx", "ruff", "commands: http, https",
	} {
		if !tt.HasText(s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
	// The system's copy behind Homebrew's is no duplicate: the OS needs it.
	for _, s := range []string{"3.15.0a1", "3.9.21", "Switch on cd", "Default version", "duplicate in PATH", "is not used"} {
		if tt.HasText(s) {
			t.Errorf("shows %q", s)
		}
	}
}

// Microsoft Store's stub is in which -a, tagged, with what it does, but
// not among the copies, nor what the terminal runs when it comes first.
func TestPythonStub(t *testing.T) {
	stub := inventory.Installation{Tool: "python", Path: `C:\Users\Ann\AppData\Local\Microsoft\WindowsApps\python.exe`, OnPath: 2, Stub: true}
	ins := append(pythonSnapshot().Tools[0].Installations[:1:1], stub)
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "python", Installations: ins}}}
	tt := openPython(t, a)
	for _, s := range []string{"1 copy", "stub",
		`C:\Users\Ann\AppData\Local\Microsoft\WindowsApps\python.exe is not Python but a Microsoft Store stub: once the copies before it leave PATH, the command opens the Store instead of an error.`} {
		if !tt.HasText(s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
	if tt.HasText("duplicate in PATH") || tt.HasText("is not used") {
		t.Error("the stub counts as a copy")
	}
	ins[0].OnPath, ins[1].OnPath = 2, 1
	if inventory.TerminalCopy(ins) != -1 {
		t.Error("the terminal runs the stub")
	}
	// First in PATH, as on a fresh Windows: python opens the Store now.
	tt = openPython(t, a)
	if s := `C:\Users\Ann\AppData\Local\Microsoft\WindowsApps\python.exe is not Python but a Microsoft Store stub, and it comes first in PATH: the python command opens the Store.`; !tt.HasText(s) {
		t.Errorf("no %q; texts %q", s, tt.Texts())
	}
}
