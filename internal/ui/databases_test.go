package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
)

func TestDatabasesPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
		{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17"},
		{Kind: "Redis", Name: "Redis", Version: "7.4.1", Path: "/opt/homebrew/bin/redis-server", Source: "brew", Status: "running", Service: "redis"},
		{Kind: "MySQL", Name: "MySQL", Version: "8.4.3", Path: "/opt/homebrew/bin/mysqld", Source: "brew", Status: "stopped", Service: "mysql@8.4"},
	}, Clients: []inventory.Installation{{Tool: "sqlite3", Version: "3.43.2", Path: "/usr/bin/sqlite3"}, {Tool: "psql", Version: "17.2", Path: "/opt/homebrew/bin/psql"}}, Listeners: []inventory.DatabaseListener{{Address: "*", Port: "5432", Process: "postgres", PID: "812"}, {Address: "127.0.0.1", Port: "6379", Process: "redis-server", PID: "815"}}}},
		{Tool: "docker", Docker: &inventory.DockerState{Databases: []inventory.Database{{Kind: "PostgreSQL", Name: "shop-db", Version: "postgres:16", Source: "docker", Status: "stopped", Service: "db", Project: home + "/dev/shop-web"}}}}}}
	a.Projects = []projects.Project{{Dir: home + "/dev/shop-web", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}}}
	a.router.Push("/tools/databases")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "databases")
	for _, text := range []string{"PostgreSQL 17.2", "postgres:16", "sqlite3 3.43.2", "Database port listeners", "Port 5432 is already in use"} {
		if !tt.HasText(text) {
			t.Errorf("no %q: %q", text, tt.Texts())
		}
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
