package detect

import (
	"os"
	"path/filepath"
	"testing"

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
	r.Stdout("postgresql.service loaded active exited PostgreSQL RDBMS\npostgresql@17-main.service loaded active running PostgreSQL\nmysql.service loaded inactive dead MySQL\npostgres-exporter.service loaded active running Metrics", exe, "list-units", "--all", "--type=service", "--no-legend", "--plain", "--no-pager")
	r.Stdout("LISTEN 0 100 127.0.0.1:5432 0.0.0.0:*\nLISTEN 0 100 [::1]:6379 [::]:*", ss, "-ltnH")
	d := &inventory.DatabaseState{}
	databaseServices(t.Context(), r, env, d)
	databaseListeners(t.Context(), r, env, d)
	if len(d.Servers) != 2 || d.Servers[0].Status != "running" || d.Servers[1].Status != "stopped" || len(d.Listeners) != 2 {
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
