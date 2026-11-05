package detect

import (
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
	r.Stdout("postgresql@17-main.service loaded active running PostgreSQL\nmysql.service loaded inactive dead MySQL\npostgres-exporter.service loaded active running Metrics", exe, "list-units", "--all", "--type=service", "--no-legend", "--plain", "--no-pager")
	r.Stdout("LISTEN 0 100 127.0.0.1:5432 0.0.0.0:*\nLISTEN 0 100 [::1]:6379 [::]:*", ss, "-ltnH")
	d := &inventory.DatabaseState{}
	databaseServices(t.Context(), r, env, d)
	databaseListeners(t.Context(), r, env, d)
	if len(d.Servers) != 2 || d.Servers[0].Status != "running" || d.Servers[1].Status != "stopped" || len(d.Listeners) != 2 {
		t.Fatal(d)
	}
}
