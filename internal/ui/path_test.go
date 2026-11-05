package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
		}, GUIPath: []inventory.PathEntry{{Dir: "/usr/bin"}, {Dir: "/bin"}},
		Variables: []inventory.EnvironmentVariable{{Name: "JAVA_HOME", Terminal: "/jdk"}, {Name: "GOPATH", Terminal: home + "/go", GUI: home + "/go"}},
	}
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "path")
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
