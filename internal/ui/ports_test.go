package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
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
	if !slices.Equal(rows[1].Addresses, []string{"*", "[::]"}) {
		t.Fatalf("addresses %+v", rows[1].Addresses)
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
	hasTexts(t, tt, "Ports", "3000", "node", "127.0.0.1", "*, [::]", "0.0.0.0", "service postgresql@17", "another user's or the system's process", "network", "localhost")
	a1, ok1 := tt.Find("0.0.0.0")
	a2, ok2 := tt.Find("127.0.0.1")
	a3, ok3 := tt.Find("*")
	if !ok1 || !ok2 || !ok3 || a1.X != a2.X || a2.X != a3.X {
		t.Fatalf("bind address column misaligned: 0.0.0.0=%+v 127.0.0.1=%+v *=%+v", a1, a2, a3)
	}
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "ports-1000")
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
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

func TestPortsRefreshAndDockerJump(t *testing.T) {
	webID := strings.Repeat("b", 64)
	a := NewApp()
	a.Home = home
	scans := 0
	a.OnScan = func() { scans++ }
	refreshes := 0
	a.RefreshPorts = func(done func(detect.PortsSnapshot)) {
		refreshes++
		done(detect.PortsSnapshot{
			Listeners: []inventory.DatabaseListener{
				{Address: "0.0.0.0", Port: "8080", Process: "com.docker.backend", PID: "700", Own: true},
			},
			Containers: []inventory.DockerPortContainer{
				{ID: webID, Name: "frontend-nginx", Image: "nginx:1.27", Status: "running", Ports: []string{"8080"}},
			},
			HasDocker: true,
		})
	}
	a.WatchDocker = func(docker.Target, bool, func(docker.Engine, error)) func() { return func() {} }
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		ct := docker.Container{ID: webID, Name: "frontend-nginx", Image: "nginx:1.27"}
		ct.State.Status = "running"
		got([]docker.Container{ct}, nil, nil)
		return func() {}
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "databases", Databases: &inventory.DatabaseState{
			Listeners: []inventory.DatabaseListener{
				{Address: "127.0.0.1", Port: "9000", Process: "old-proc", PID: "100", Own: true},
			},
		}},
		{Tool: "docker",
			Installations: []inventory.Installation{{Tool: "docker", Path: "/usr/local/bin/docker", OnPath: 1}},
			Docker: &inventory.DockerState{
				Selected: "default",
				Contexts: []inventory.DockerContext{{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true, Status: "available"}},
			}},
	}}
	a.router.Push(portsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "9000", "old-proc")

	if err := tt.Click(locales.PortsRefresh()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if refreshes != 1 || scans != 0 {
		t.Fatalf("refreshes=%d scans=%d, want 1 and 0", refreshes, scans)
	}
	hasTexts(t, tt, "8080", "0.0.0.0", "Docker container frontend-nginx")
	if tt.HasText("old-proc") {
		t.Fatal("old listener still shown after targeted refresh")
	}

	if err := tt.Click(locales.OpenInDocker()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != dockerEnginePage || a.engine.view != dockerContainers || a.engine.containers.detail.id != webID {
		t.Fatalf("after Open in Docker: path=%q view=%d selected=%q", a.router.Path(), a.engine.view, a.engine.containers.detail.id)
	}
}

func TestPortsBindIPAndWarningPreservation(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{Tool: "databases", Databases: &inventory.DatabaseState{
				Listeners: []inventory.DatabaseListener{
					{Port: "8080", Address: "127.0.0.1", Process: "docker-proxy", PID: "100"},
					{Port: "8080", Address: "127.0.0.2", Process: "node", PID: "200"},
				},
				Warnings: []string{"brew services", "lsof"},
			}},
			{Tool: "docker", Docker: &inventory.DockerState{
				Selected: "default",
				PortContainers: []inventory.DockerPortContainer{{
					ID:       "abcdef123456",
					Name:     "api",
					Status:   "running",
					Ports:    []string{"8080"},
					Bindings: []inventory.DockerPortBinding{{HostIP: "127.0.0.1", HostPort: "8080"}},
				}},
			}},
		},
	}
	rows := a.portRows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 port rows, got %d", len(rows))
	}
	if ct, ok := a.portContainerFor(rows[0]); !ok || ct.Name != "api" {
		t.Fatalf("127.0.0.1 docker-proxy should match api, got %+v, %v", ct, ok)
	}
	if ct, ok := a.portContainerFor(rows[1]); ok {
		t.Fatalf("127.0.0.2 non-docker listener should not match api, got %+v", ct)
	}

	a.applyPortsRefresh(detect.PortsSnapshot{
		Listeners: []inventory.DatabaseListener{{Port: "9000", Address: "127.0.0.1", Process: "vite", PID: "300"}},
	})
	st, _ := a.Snapshot.Tool("databases")
	if !slices.Contains(st.Databases.Warnings, "brew services") {
		t.Fatalf("service warning was wiped on ports refresh: %v", st.Databases.Warnings)
	}
	if slices.Contains(st.Databases.Warnings, "lsof") {
		t.Fatalf("stale listener warning remained after clean refresh: %v", st.Databases.Warnings)
	}
}
