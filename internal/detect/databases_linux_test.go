package detect

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDatabaseSystemd(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "systemctl")
	fakeExe(t, exe)
	ss := filepath.Join(dir, "ss")
	fakeExe(t, ss)
	env := sysenv.Env{"PATH=" + dir}
	r := &run.Fake{}
	r.Stdout("postgresql.service loaded active exited PostgreSQL RDBMS\npostgresql@17-main.service loaded active running PostgreSQL\nmysql.service loaded inactive dead MySQL\nmongod.service loaded failed failed MongoDB\nmssql-server.service loaded inactive dead SQL Server\npostgres-exporter.service loaded active running Metrics", exe, "list-units", "--all", "--type=service", "--no-legend", "--plain", "--no-pager")
	r.Stdout("LISTEN 0 100 127.0.0.1:5432 0.0.0.0:*\nLISTEN 0 100 [::1]:6379 [::]:*", ss, "-ltnH")
	r.Set(run.Result{ExitCode: 1, Stdout: []byte("enabled\ndisabled\nenabled\nstatic\n")}, nil, exe, "is-enabled", "postgresql.service", "mysql.service", "mongod.service", "mssql-server.service")
	d := &inventory.DatabaseState{}
	databaseServices(t.Context(), r, env, d)
	databaseListeners(t.Context(), r, env, d)
	if len(d.Servers) != 4 || d.Servers[0].Status != "running" || d.Servers[1].Status != "stopped" || d.Servers[2].Kind != "MongoDB" || d.Servers[2].Status != "error" || d.Servers[3].Status != "stopped" || len(d.Listeners) != 2 ||
		d.Servers[0].Autostart != "on" || d.Servers[1].Autostart != "off" || d.Servers[3].Autostart != "" {
		t.Fatal(d)
	}
}

// Debian's redis-server is a link to redis-check-rdb: the page names the server.
func TestDatabaseRedisLink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "redis-check-rdb")
	fakeExe(t, real)
	path := filepath.Join(dir, "redis-server")
	if err := os.Symlink(real, path); err != nil {
		t.Fatal(err)
	}
	r := &run.Fake{}
	r.Stdout("Redis server v=7.0.15", path, "--version")
	env := sysenv.Env{"PATH=" + dir, "HOME=" + dir}
	st, err := detectDatabases(t.Context(), r, env, env, func(sysenv.Env, string) []string { return nil })
	if err != nil || len(st.Databases.Servers) != 1 || st.Databases.Servers[0].Path != path {
		t.Fatalf("%+v %v", st.Databases, err)
	}
}

// sqlservr is never run: its version is the mssql-server package's.
func TestDatabaseMongoAndSQLServer(t *testing.T) {
	dir := t.TempDir()
	r := &run.Fake{}
	for _, command := range []string{"mongod", "sqlservr", "mongosh", "sqlcmd"} {
		fakeExe(t, filepath.Join(dir, command))
	}
	r.Stdout("db version v8.0.4\nBuild Info: {}", filepath.Join(dir, "mongod"), "--version")
	r.Stdout("2.3.8", filepath.Join(dir, "mongosh"), "--version")
	r.Stdout("sqlcmd: 1.8.2", filepath.Join(dir, "sqlcmd"), "--version")
	r.Stdout("16.0.4165.4-1", "dpkg-query", "-W", "-f=${Version}", "mssql-server")
	env := sysenv.Env{"PATH=" + dir, "HOME=" + dir}
	st, _ := detectDatabases(t.Context(), r, env, env, func(sysenv.Env, string) []string { return nil })
	d := st.Databases
	if len(d.Servers) != 2 || d.Servers[0].Kind != "MongoDB" || d.Servers[0].Version != "8.0.4" || d.Servers[1].Kind != "SQL Server" || d.Servers[1].Version != "16.0.4165" {
		t.Fatalf("%+v", d.Servers)
	}
	if len(d.Clients) != 2 || d.Clients[0].Version != "2.3.8" || d.Clients[1].Version != "1.8.2" {
		t.Fatalf("%+v", d.Clients)
	}
	for _, call := range r.Calls() {
		if filepath.Base(call.Args[0]) == "sqlservr" || filepath.Base(call.Args[0]) == "mongosh" && call.Timeout < 10*time.Second {
			t.Fatal(call.Args, call.Timeout)
		}
	}
}
