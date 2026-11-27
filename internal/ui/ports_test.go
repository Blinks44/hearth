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
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
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
	hasTexts(t, tt,
		locales.ServicesPortsTitle(),
		locales.PortsColPort(),
		locales.PortsColService(),
		locales.PortsColMetricsActions(),
		locales.PortsQuickTitle(),
		":3000", "Node", "shop", "127.0.0.1", "*, [::]", "0.0.0.0",
		"service postgresql@17", "another user's or the system's process", "network",
	)
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "ports-1000")
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	// Narrow to Dev servers tab so port 3000 is the sole row.
	if err := tt.Click(locales.PortsTabDevServers()); err != nil {
		t.Fatalf("click Dev servers tab: %v", err)
	}
	tt.Advance(time.Second)
	if tt.HasText("5432") || !tt.HasText(":3000") {
		t.Fatalf("Dev servers tab did not filter rows: %q", tt.Texts())
	}
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Equal(menu, []string{"Copy localhost:3000", "Open in browser", "Open project", "Stop…"}) {
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
	nativeListener := inventory.DatabaseListener{Port: "8080", Address: "127.0.0.1", Process: "postgres", PID: "250"}
	if ct, ok := a.portContainerFor(portRow{DatabaseListener: nativeListener, Addresses: []string{"127.0.0.1"}}); ok {
		t.Fatalf("native postgres on 127.0.0.1 should not match Docker container, got %+v", ct)
	}
	colimaSSHListener := inventory.DatabaseListener{Port: "8080", Address: "127.0.0.1", Process: "ssh", PID: "260", Path: "/usr/local/bin/ssh", Own: true}
	colimaSSH := portRow{DatabaseListener: colimaSSHListener, Addresses: []string{"127.0.0.1"}}
	if ct, ok := a.portContainerFor(colimaSSH); !ok || ct.Name != "api" {
		t.Fatalf("Colima ssh on container port should match api, got %+v, %v", ct, ok)
	}
	userSSHListener := inventory.DatabaseListener{Port: "9090", Address: "127.0.0.1", Process: "ssh", PID: "261", Path: "/usr/local/bin/ssh", Own: true}
	userSSH := portRow{DatabaseListener: userSSHListener, Addresses: []string{"127.0.0.1"}}
	if a.isPortDocker(userSSH) {
		t.Fatalf("standalone user ssh tunnel should not be classified as Docker")
	}
	if _, _, stoppable := a.portAbout(userSSH); !stoppable {
		t.Fatalf("standalone user ssh tunnel should remain stoppable")
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
	dst, _ := a.Snapshot.Tool("docker")
	if len(dst.Docker.PortContainers) != 0 || !dst.Docker.PortContainersFresh {
		t.Fatalf("stale Docker port containers were not cleared when HasDocker=false: %+v", dst.Docker)
	}
}

func TestPortsDockerStopAndQuickFilters(t *testing.T) {
	webID := strings.Repeat("c", 64)
	a := NewApp()
	a.Home = home
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + t.TempDir()}, nil))
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "databases", Databases: &inventory.DatabaseState{
			Listeners: []inventory.DatabaseListener{
				{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "100", Own: true},
				{Address: "0.0.0.0", Port: "8080", Process: "com.docker.backend", PID: "700", Own: true},
			},
		}},
		{Tool: "docker",
			Installations: []inventory.Installation{{Tool: "docker", Path: "/usr/local/bin/docker", OnPath: 1}},
			Docker: &inventory.DockerState{
				Selected:       "default",
				Contexts:       []inventory.DockerContext{{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true, Status: "available"}},
				PortContainers: []inventory.DockerPortContainer{{ID: webID, Name: "frontend-nginx", Image: "nginx:1.27", Status: "running", Ports: []string{"8080"}}},
			}},
	}}
	a.router.Push(portsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	// Switch to Docker quick filter tab.
	devTab, okDev := tt.Find(locales.PortsTabDevServers())
	if !okDev {
		t.Fatalf("missing Dev servers tab")
	}
	tt.ClickAt(devTab.X+devTab.W+48, devTab.Y+devTab.H/2)
	tt.Advance(time.Second)
	if tt.HasText("5432") || !tt.HasText(":8080") {
		t.Fatalf("Docker tab did not filter rows: %q", tt.Texts())
	}
	// Stop container directly from Ports row.
	if err := tt.Click(locales.DatabaseStopContainer()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.DockerAction != ops.DockerStop {
		t.Fatalf("expected DockerStop plan from Ports, got %+v", a.op)
	}
}
