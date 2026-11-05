package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDatabaseWindowsServices(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sc.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	netstat := filepath.Join(dir, "netstat.exe")
	if err := os.WriteFile(netstat, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"PATH=" + dir, "PATHEXT=.EXE"}
	r := &run.Fake{}
	r.Stdout("SERVICE_NAME: postgresql-x64-17\r\n        TYPE               : 10  WIN32_OWN_PROCESS\r\n        STATE              : 4  RUNNING\r\nSERVICE_NAME: MySQL80\r\n        STATE              : 1  STOPPED\r\n", exe, "queryex", "type=", "service", "state=", "all")
	r.Stdout("  Proto  Local Address          Foreign Address        State           PID\r\n  TCP    127.0.0.1:5432         0.0.0.0:0              ABH\u00d6REN         812\r\n  TCP    127.0.0.1:50000        127.0.0.1:5432         HERGESTELLT     900\r\n", netstat, "-ano", "-p", "tcp")
	d := &inventory.DatabaseState{}
	databaseServices(t.Context(), r, env, d)
	databaseListeners(t.Context(), r, env, d)
	if len(d.Servers) != 2 || d.Servers[0].Status != "running" || d.Servers[1].Status != "stopped" || len(d.Listeners) != 1 {
		t.Fatal(d)
	}
}
