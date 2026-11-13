package detect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDatabases(t *testing.T) {
	dir := t.TempDir()
	r := &run.Fake{}
	for _, command := range []string{"postgres", "psql", "redis-server", "sqlite3"} {
		path := filepath.Join(dir, command+dockerSuffix)
		if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		r.Stdout("version 17.2", path, "--version")
	}
	env := sysenv.Env{"PATH=" + dir, "HOME=" + dir, "USERPROFILE=" + dir, "PATHEXT=.EXE"}
	st, err := detectDatabases(context.Background(), r, env, env, func(sysenv.Env, string) []string { return nil })
	if err != nil || len(st.Databases.Servers) != 2 || len(st.Databases.Clients) != 2 {
		t.Fatalf("%+v %v", st.Databases, err)
	}
	for _, db := range st.Databases.Servers {
		if db.Status != "" || len(db.Ports) != 0 || db.Version != "17.2" {
			t.Fatal(db)
		}
	}
	for _, call := range r.Calls() {
		if len(call.Args) != 2 || call.Args[1] != "--version" {
			t.Fatal(call.Args)
		}
	}
}

func TestDockerDatabases(t *testing.T) {
	r := &run.Fake{}
	d := &inventory.DockerState{}
	dockerDatabases(t.Context(), r, sysenv.Env{"DOCKER_CONTEXT=remote"}, "docker", "unix:///local", d)
	call := r.Calls()[0]
	if !d.DatabaseErr || call.Timeout == 0 || sysenv.Env(call.Env).Get("DOCKER_CONTEXT") != "" {
		t.Fatal(call)
	}
	format := call.Args[len(call.Args)-1]
	if strings.Contains(format, ".Labels") || strings.Contains(format, ".Env") {
		t.Fatal("unrestricted fields")
	}
	r.Stdout(`{"Name":"db","Image":"postgres:16","State":"running","Ports":"127.0.0.1:5433->5432/tcp, [::1]:5433->5432/tcp","Service":"db","Project":"/dev/app"}
{"Name":"metrics","Image":"postgres-exporter:latest","State":"running"}
{"Name":"cache","Image":"redis:7","State":"exited"}
{"Name":"docs","Image":"mongo:8","State":"running","Ports":"0.0.0.0:27017->27017/tcp"}
{"Name":"sql","Image":"mcr.microsoft.com/mssql/server:2022-latest","State":"running","Ports":"127.0.0.1:1433->1433/tcp"}`, call.Args...)
	d = &inventory.DockerState{}
	dockerDatabases(t.Context(), r, nil, "docker", "unix:///local", d)
	if d.DatabaseErr || len(d.Databases) != 4 || d.Databases[2].Kind != "MongoDB" || d.Databases[3].Kind != "SQL Server" || d.Databases[3].Ports[0] != "1433" || len(d.Databases[0].Ports) != 1 || d.Databases[0].Ports[0] != "5433" || d.Databases[1].Status != "stopped" {
		t.Fatalf("%+v", d)
	}
}

func TestDatabaseBrewServices(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "brew"+dockerSuffix)
	if err := os.WriteFile(exe, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"PATH=" + dir, "PATHEXT=.EXE"}
	r := &run.Fake{}
	r.Stdout(`[{"name":"postgresql@17","status":"started","file":"/Users/ann/Library/LaunchAgents/homebrew.mxcl.postgresql@17.plist"},{"name":"redis","status":"none","file":"/opt/homebrew/opt/redis/homebrew.mxcl.redis.plist"},{"name":"postgres-exporter","status":"started"},{"name":"mongodb-community","status":"started"}]`, exe, "services", "list", "--json")
	d := &inventory.DatabaseState{Servers: []inventory.Database{{Kind: "PostgreSQL", Path: "/opt/homebrew/Cellar/postgresql@17/17.2/bin/postgres", Source: "brew"}, {Kind: "MongoDB", Path: "/opt/homebrew/Cellar/mongodb-community/8.0.4/bin/mongod", Source: "brew"}}}
	databaseBrewServices(t.Context(), r, env, d)
	if len(d.Servers) != 3 || d.Servers[0].Status != "running" || d.Servers[0].Autostart != "on" || d.Servers[1].Service != "mongodb-community" || d.Servers[2].Status != "stopped" || d.Servers[2].Autostart != "off" {
		t.Fatal(d)
	}
}

func TestStoppedDatabasePorts(t *testing.T) {
	r := &run.Fake{}
	r.Stdout(`{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5433"},{"HostIp":"::1","HostPort":"5433"}],"6379/udp":[{"HostPort":"6379"}]}`, "docker", "--host", "unix:///local", "inspect", "--format", "{{json .HostConfig.PortBindings}}", "abcdef123456")
	ports, ok := databaseContainerPorts(t.Context(), r, nil, "docker", "unix:///local", "abcdef123456")
	if !ok || len(ports) != 1 || ports[0] != "5433" {
		t.Fatalf("%v %v", ports, ok)
	}
}

func TestMariaDBLinks(t *testing.T) {
	dir := t.TempDir()
	r := &run.Fake{}
	for _, pair := range [][2]string{{"mariadbd", "mysqld"}, {"mariadb", "mysql"}} {
		real := filepath.Join(dir, pair[0]+dockerSuffix)
		if err := os.WriteFile(real, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, pair[1]+dockerSuffix)
		if err := os.Symlink(real, link); err != nil {
			t.Skip("no symlinks:", err)
		}
		for _, path := range []string{real, link} {
			r.Stdout(path+"  Ver 11.4.2-MariaDB for osx10.20", path, "--version")
		}
	}
	env := sysenv.Env{"PATH=" + dir, "HOME=" + dir, "USERPROFILE=" + dir, "PATHEXT=.EXE"}
	st, _ := detectDatabases(t.Context(), r, env, env, func(sysenv.Env, string) []string { return nil })
	if len(st.Databases.Servers) != 1 || st.Databases.Servers[0].Kind != "MariaDB" || len(st.Databases.Clients) != 1 {
		t.Fatalf("%+v", st.Databases)
	}
}

// MariaDB's Windows installer copies mariadbd to mysqld: one server.
func TestMariaDBCopies(t *testing.T) {
	dir := t.TempDir()
	r := &run.Fake{}
	for _, name := range []string{"mariadbd", "mysqld"} {
		path := filepath.Join(dir, name+dockerSuffix)
		if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		r.Stdout(path+"  Ver 13.0.2-MariaDB for Win64", path, "--version")
	}
	env := sysenv.Env{"PATH=" + dir, "HOME=" + dir, "USERPROFILE=" + dir, "PATHEXT=.EXE"}
	st, _ := detectDatabases(t.Context(), r, env, env, func(sysenv.Env, string) []string { return nil })
	if len(st.Databases.Servers) != 1 || st.Databases.Servers[0].Kind != "MariaDB" {
		t.Fatalf("%+v", st.Databases.Servers)
	}
}

func TestDatabaseVersionOf(t *testing.T) {
	for out, want := range map[string]string{
		"/opt/homebrew/Cellar/mysql@8.4/8.4.3/bin/mysqld  Ver 8.4.3 for macos15 on arm64": "8.4.3",
		"postgres (PostgreSQL) 17.2":                                    "17.2",
		"Redis server v=7.4.1 sha=00000000:0 malloc=libc bits=64":       "7.4.1",
		"3.43.2 2023-10-10 13:08:14 0ea7a3d5fe8ebd2c2a0d39f6fd5f0ea7a3": "3.43.2",
	} {
		if got := databaseVersionOf(out); got != want {
			t.Errorf("%q: %q, want %q", out, got, want)
		}
	}
}
