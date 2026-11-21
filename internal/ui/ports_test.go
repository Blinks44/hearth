package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// The page names who listens at each port and the project it runs in;
// the menu stops one's own program, or a database's service through its
// manager, and offers nothing for Docker's, the system's or others'.
func TestPorts(t *testing.T) {
	a := NewApp()
	a.Home = home
	shop := home + "/dev/shop"
	a.Projects = []projects.Project{{Dir: shop}}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
		Servers: []inventory.Database{{Kind: "PostgreSQL", Name: "PostgreSQL", Source: "brew", Status: "running", Service: "postgresql@17"}},
		Listeners: []inventory.DatabaseListener{
			{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "812", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Own: true},
			{Address: "*", Port: "3000", Process: "node", PID: "4312", Path: "/opt/homebrew/bin/node", Dirs: []string{shop + "/web"}, Own: true},
			{Address: "[::]", Port: "3000", Process: "node", PID: "4312", Path: "/opt/homebrew/bin/node", Dirs: []string{shop + "/web"}, Own: true},
			{Address: "*", Port: "8080", Process: "com.docker.backend", PID: "700", Path: "/Applications/Docker.app/Contents/MacOS/com.docker.backend", Own: true},
			{Address: "0.0.0.0", Port: "631"},
		}}}}}
	rows := a.portRows()
	if len(rows) != 4 || rows[0].Port != "631" || rows[1].Port != "3000" || rows[1].Project != shop || !rows[1].Network || rows[2].Network {
		t.Fatalf("rows %+v", rows)
	}
	for _, r := range []portRow{rows[0], rows[3]} {
		if _, _, ok := a.portAbout(r); ok {
			t.Errorf("offers to stop %+v", r)
		}
	}
	if _, plan, ok := a.portAbout(rows[2]); !ok || !slices.Equal(plan.Steps[0].Argv, []string{"brew", "services", "stop", "postgresql@17"}) {
		t.Errorf("postgres: %+v, %v", plan, ok)
	}

	a.router.Push(portsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "ports")
	hasTexts(t, tt, "Ports", "3000", "node", "service postgresql@17", "another user's or the system's process", "network", "localhost")
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Equal(menu, []string{"Open project", "Stop…"}) {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem("Stop…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Contains(a.op.plan.Steps[0].Argv, "4312") {
		t.Fatalf("plan %+v", a.op)
	}
}

func TestPortsFilterByProcess(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
		Listeners: []inventory.DatabaseListener{
			{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "812", Path: "/opt/homebrew/bin/pg", Own: true},
			{Address: "*", Port: "3000", Process: "myworker", PID: "4312", Path: "/usr/local/bin/app", Own: true},
		}}}}}
	a.router.Push(portsPage)
	a.portsFilter = "myworker"
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "3000", "myworker")
	if tt.HasText("5432") || tt.HasText("postgres") {
		t.Errorf("filter by process name did not narrow rows: %q", tt.Texts())
	}
}
