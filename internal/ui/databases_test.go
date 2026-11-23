package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestDatabasesPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
		{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17"},
		{Kind: "Redis", Name: "Redis", Version: "7.4.1", Path: "/opt/homebrew/bin/redis-server", Source: "brew", Status: "running", Service: "redis"},
		{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/bin/mysqld", Source: "brew", Status: "stopped", Service: "mysql@8.4"},
		{Kind: "MongoDB", Name: "MongoDB", Version: "8.0.4", Path: "/opt/homebrew/Cellar/mongodb-community/8.0.4/bin/mongod", Source: "brew", Status: "running", Service: "mongodb-community"},
	}, Clients: []inventory.Installation{{Tool: "sqlite3", Version: "3.43.2", Path: "/usr/bin/sqlite3"}, {Tool: "psql", Version: "17.2", Path: "/opt/homebrew/bin/psql"}, {Tool: "mongosh", Version: "2.3.8", Path: "/opt/homebrew/Cellar/mongodb-community/8.0.4/bin/mongosh"}}, Listeners: []inventory.DatabaseListener{{Address: "*", Port: "5432", Process: "postgres", PID: "812"}, {Address: "127.0.0.1", Port: "6379", Process: "redis-server", PID: "815"}, {Address: "127.0.0.1", Port: "27017", Process: "mongod", PID: "820"}, {Address: "*", Port: "8080", Process: "java", PID: "830"}}}},
		{Tool: "docker", Docker: &inventory.DockerState{Databases: []inventory.Database{{Kind: "PostgreSQL", Name: "shop-db", Version: "postgres:16", Source: "docker", Status: "stopped", Service: "db", Project: home + "/dev/shop-web"},
			{Kind: "SQL Server", Name: "sql", Version: "mcr.microsoft.com/mssql/server:2022-latest", Source: "docker", Status: "running", Ports: []string{"1433"}}}}}}}
	a.Projects = []projects.Project{{Dir: home + "/dev/shop-web", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}}}
	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "databases")
	for _, text := range []string{"PostgreSQL 17.2", "postgres:16", "sqlite3 3.43.2", "MongoDB 8.0.4", "client mongosh 2.3.8", "127.0.0.1:27017", "mcr.microsoft.com/mssql/server:2022-latest", "Database port listeners", "Port 5432 is already in use"} {
		if !tt.HasText(text) {
			t.Errorf("no %q: %q", text, tt.Texts())
		}
	}
	if tt.HasText("*:8080") {
		t.Error("a listener of no database port is shown")
	}
	if issuePage(issues.Issue{Tool: "databases", Project: "/dev/app"}) != "/tools/databases" {
		t.Fatal("wrong conflict destination")
	}
	if err := tt.Click("Containers"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("PostgreSQL 17.2") || !tt.HasText("postgres:16") {
		t.Fatal(tt.Texts())
	}
	a.Snapshot.Tools[1].Docker.Databases = nil
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("No matching database servers found.") {
		t.Fatal(tt.Texts())
	}
}

func TestDatabaseServiceOperations(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
		{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17"},
		{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/bin/mysqld", Source: "brew", Status: "stopped", Service: "mysql@8.4"},
	}, Listeners: []inventory.DatabaseListener{{Address: "*", Port: "5432", Process: "postgres", PID: "812"}}}}}}
	a.Projects = []projects.Project{{Dir: home + "/dev/shop-web", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}}}
	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "databases-actions")
	// The conflict's button stops the service on the port.
	if err := tt.Click("Stop service…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"brew", "services", "stop", "postgresql@17"}) {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	stopped := a.Snapshot.Tools[0].Databases.Servers[1]
	if p, ok := a.servicePlan(stopped); !ok || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "start", "mysql@8.4"}) {
		t.Fatalf("start plan %+v, %v", p, ok)
	}
	// A container on the port holds it, not the service.
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "docker", Docker: &inventory.DockerState{Databases: []inventory.Database{{Kind: "PostgreSQL", Name: "other-db", Source: "docker", Status: "running", Ports: []string{"5432"}}}}})
	if _, ok := a.conflictPlan(issues.Issue{Project: home + "/dev/shop-web", Dirs: []string{"5432"}}, inventory.DatabaseServers(a.Snapshot)); ok {
		t.Error("offers to stop a service for a port a container holds")
	}
}

// The header's menu installs a server, each row's string connects to it,
// and a service's switch sets its autostart, each through a plan.
func TestDatabaseInstallAndAutostart(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
			{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17", Autostart: "on"},
			{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/Cellar/mysql@8.4/8.4.3/bin/mysqld", Source: "brew", Status: "stopped", Service: "mysql@8.4", Autostart: "off"},
		}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}}},
	}}
	g := goos
	t.Cleanup(func() { goos = g })
	goos = "darwin"
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, nil))
	}
	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "databases-install")
	hasTexts(t, tt, "postgresql://dev@localhost:5432/postgres", "mysql://root@localhost:3306", "Autostart")

	if err := tt.Click("Install server"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Equal(menu, []string{"PostgreSQL", "Redis", "MySQL", "MariaDB", "MongoDB"}) {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem("MariaDB"); err == nil {
		t.Error("MariaDB offered beside MySQL")
	}
	if err := tt.ChooseMenuItem("Redis"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := [][]string{{"/opt/homebrew/bin/brew", "install", "--formula", "redis"}, {"/opt/homebrew/bin/brew", "services", "start", "redis"}}
	if a.op == nil || len(a.op.plan.Steps) != 2 || !slices.Equal(a.op.plan.Steps[0].Argv, want[0]) || !slices.Equal(a.op.plan.Steps[1].Argv, want[1]) {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	if err := tt.Click("Start mysql@8.4 automatically"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !slices.Equal(a.op.plan.Steps[0].Argv, []string{"brew", "services", "start", "mysql@8.4"}) {
		t.Fatalf("plan %+v", a.op)
	}
}

func TestDatabaseURL(t *testing.T) {
	for _, tt := range []struct {
		db       inventory.Database
		port     string
		url      string
		password bool
	}{
		{inventory.Database{Kind: "PostgreSQL", Source: "brew"}, "", "postgresql://ann@localhost:5432/postgres", false},
		{inventory.Database{Kind: "PostgreSQL", Source: "brew"}, "5433", "postgresql://ann@localhost:5433/postgres", false},
		{inventory.Database{Kind: "PostgreSQL", Source: "system"}, "", "postgresql://postgres@localhost:5432/postgres", true},
		{inventory.Database{Kind: "PostgreSQL", Source: "docker"}, "5434", "postgresql://postgres@localhost:5434/postgres", true},
		{inventory.Database{Kind: "PostgreSQL", Source: "docker"}, "", "", false},
		{inventory.Database{Kind: "MySQL", Source: "brew"}, "", "mysql://root@localhost:3306", false},
		{inventory.Database{Kind: "MySQL", Source: "system"}, "3307", "mysql://root@localhost:3307", true},
		{inventory.Database{Kind: "Redis", Source: "system"}, "", "redis://localhost:6379", false},
		{inventory.Database{Kind: "Cassandra"}, "", "", false},
	} {
		if url, password := databaseURL(tt.db, "ann", tt.port); url != tt.url || password != tt.password {
			t.Errorf("%+v (%q): %q %v, want %q %v", tt.db, tt.port, url, password, tt.url, tt.password)
		}
	}
}

func TestDatabaseObservedPortDockerAndClient(t *testing.T) {
	pgID := strings.Repeat("d", 64)
	a := NewApp()
	a.Home = home
	var launched []string
	a.OpenDBClient = func(argv []string, done func(error)) {
		launched = argv
		done(nil)
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + t.TempDir()}, nil))
	}
	a.WatchDocker = func(docker.Target, bool, func(docker.Engine, error)) func() { return func() {} }
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		ct := docker.Container{ID: pgID, Name: "shop-pg", Image: "postgres:16"}
		ct.State.Status = "running"
		got([]docker.Container{ct}, nil, nil)
		return func() {}
	}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "databases", Databases: &inventory.DatabaseState{
			Servers: []inventory.Database{
				{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/bin/mysqld", Source: "brew", Status: "running", Service: "mysql@8.4"},
			},
			Clients: []inventory.Installation{
				{Tool: "mysql", Version: "8.4.3", Path: "/opt/homebrew/bin/mysql", RealPath: "/opt/homebrew/bin/mysql"},
				{Tool: "psql", Version: "17.2", Path: "/opt/homebrew/bin/psql", RealPath: "/opt/homebrew/bin/psql"},
				{Tool: "sqlite3", Version: "3.43.2", Path: "/usr/bin/sqlite3", RealPath: "/usr/bin/sqlite3"},
			},
			Listeners: []inventory.DatabaseListener{
				{Address: "127.0.0.1", Port: "3307", Process: "mysqld", PID: "901", Path: "/opt/homebrew/bin/mysqld"},
				{Address: "127.0.0.1", Port: "5433", Process: "com.docker.backend", PID: "902"},
			},
		}},
		{Tool: "docker",
			Installations: []inventory.Installation{{Tool: "docker", Path: "/usr/local/bin/docker", OnPath: 1}},
			Docker: &inventory.DockerState{
				Selected: "default",
				Contexts: []inventory.DockerContext{{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true, Status: "available"}},
				Databases: []inventory.Database{
					{Kind: "PostgreSQL", Name: "shop-pg", Version: "postgres:16", Source: "docker", Status: "running", ContainerID: pgID, Ports: []string{"5433"}},
				},
			}},
	}}
	a.Projects = []projects.Project{{Dir: home + "/dev/shop", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5433"}}}}}

	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "mysql://root@localhost:3307", "postgresql://postgres@localhost:5433/postgres")
	screenshot(t, tt, "databases-s13-06-1280")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "databases-s13-06-1000")
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)

	// Launch mysql CLI against observed port 3307 using TCP 127.0.0.1.
	if err := tt.Click(locales.DatabaseOpenClient("mysql")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	wantMySQL := []string{"/opt/homebrew/bin/mysql", "-h", "127.0.0.1", "-P", "3307", "-u", "root"}
	if !slices.Equal(launched, wantMySQL) {
		t.Fatalf("mysql launch %q, want %q", launched, wantMySQL)
	}

	// Launch psql CLI against the Docker PostgreSQL container on port 5433.
	if err := tt.Click(locales.DatabaseOpenClient("psql")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	wantPSQL := []string{"/opt/homebrew/bin/psql", "postgresql://postgres@localhost:5433/postgres"}
	if !slices.Equal(launched, wantPSQL) {
		t.Fatalf("psql launch %q, want %q", launched, wantPSQL)
	}

	// Conflict banner offers stopping the Docker container holding port 5433.
	if err := tt.Click(locales.DatabaseStopContainer()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.DockerAction != ops.DockerStop {
		t.Fatalf("expected DockerStop plan, got %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Open in Docker jumps straight to the database container.
	if err := tt.Click(locales.OpenInDocker()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != dockerEnginePage || a.engine.view != dockerContainers || a.engine.containers.detail.id != pgID {
		t.Fatalf("after Open in Docker: path=%q view=%d selected=%q", a.router.Path(), a.engine.view, a.engine.containers.detail.id)
	}
}

// Windows offers MySQL, but selecting it only proposes a plan.
func TestWindowsDatabaseInstallMenu(t *testing.T) {
	previous := goos
	goos = "windows"
	t.Cleanup(func() { goos = previous })
	a := cleanApp(t)
	tt := openAI(t, a, "/tools/databases")
	if err := tt.Click("Install server"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if !slices.Contains(menu, "MySQL") || slices.Contains(menu, "MariaDB") {
		t.Fatalf("unexpected Windows database menu: %v", menu)
	}
	if err := tt.ChooseMenuItem("MySQL"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || len(a.op.plan.Steps) == 0 {
		t.Fatal("selection did not create a pending plan")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op.started {
		t.Fatal("cancelling installation started the operation")
	}
}

func TestDatabaseStoppedNoCLIProjectsFilterAndRefresh(t *testing.T) {
	a := NewApp()
	a.Home = home
	scans := 0
	a.OnScan = func() { scans++ }
	a.OpenDBClient = func([]string, func(error)) {}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{
		Servers: []inventory.Database{
			{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/bin/mysqld", Source: "brew", Status: "stopped", Service: "mysql@8.4"},
		},
		Clients: []inventory.Installation{
			{Tool: "mysql", Version: "8.4.3", Path: "/opt/homebrew/bin/mysql", RealPath: "/opt/homebrew/bin/mysql"},
		},
	}}}}
	a.Projects = []projects.Project{
		{Dir: home + "/dev/alpha", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}},
		{Dir: home + "/dev/beta", Databases: []projects.DatabaseReference{{Kind: "Redis", Service: "cache", File: "compose.yaml", Ports: []string{"6379"}}}},
	}
	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Refresh button triggers scan.
	if err := tt.Click(locales.PortsRefresh()); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("scans = %d, want 1", scans)
	}
	a.Scanning = false

	// Stopped DB row must NOT offer CLI launch inside its row menu (only the standalone Clients card offers it).
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); slices.Contains(menu, locales.DatabaseOpenClient("mysql")) {
		t.Fatalf("stopped DB menu must not offer CLI launch: %q", menu)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	// Projects tab filters by databaseFilter and shows Nothing matches when no project matches.
	svcTab, okS := tt.Find(locales.DatabaseServices())
	cntTab, okC := tt.Find(locales.DatabaseContainers())
	if !okS || !okC {
		t.Fatalf("missing Services (%v) or Containers (%v) tab", okS, okC)
	}
	tt.ClickAt((svcTab.X+svcTab.W+cntTab.X)/2, svcTab.Y+svcTab.H/2)
	tt.Advance(time.Second)
	hasTexts(t, tt, "alpha", "beta")
	a.databaseFilter = "redis"
	tt.Frame()
	if tt.HasText("alpha") || !tt.HasText("beta") {
		t.Fatalf("Projects tab filter failed: %q", tt.Texts())
	}
	a.databaseFilter = "no-such-db"
	tt.Frame()
	hasTexts(t, tt, locales.NothingMatches())
}
