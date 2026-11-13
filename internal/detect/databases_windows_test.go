package detect

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func init() {
	listWindowsServices = func() ([]winService, error) { return nil, nil }
}

func TestDatabaseWindowsServices(t *testing.T) {
	d := &inventory.DatabaseState{}
	addDatabaseServices(d, []winService{
		{"postgresql-x64-17", windows.SERVICE_RUNNING, windows.SERVICE_AUTO_START},
		{"EventLog", windows.SERVICE_RUNNING, 0},
		{"MySQL80", windows.SERVICE_STOPPED, windows.SERVICE_DEMAND_START},
		{"MongoDB", windows.SERVICE_RUNNING, 0},
		{"MSSQL$SQLEXPRESS", windows.SERVICE_RUNNING, windows.SERVICE_AUTO_START},
		{"SQLBrowser", windows.SERVICE_STOPPED, 0},
	})
	if len(d.Servers) != 4 || d.Servers[0].Status != "running" || d.Servers[1].Status != "stopped" || d.Servers[2].Kind != "MongoDB" || d.Servers[3].Kind != "SQL Server" ||
		d.Servers[0].Autostart != "on" || d.Servers[1].Autostart != "off" || d.Servers[2].Autostart != "" {
		t.Fatal(d)
	}
}

// The Service Control Manager lists services without an administrator:
// the event log's runs on every Windows.
func TestWindowsServicesReal(t *testing.T) {
	services, err := windowsServices()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range services {
		if s.name == "EventLog" && s.state == windows.SERVICE_RUNNING {
			return
		}
	}
	t.Errorf("EventLog not running among %d services", len(services))
}

func TestDatabaseWindowsListeners(t *testing.T) {
	dir := t.TempDir()
	netstat := filepath.Join(dir, "netstat.exe")
	if err := os.WriteFile(netstat, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"PATH=" + dir, "PATHEXT=.EXE"}
	r := &run.Fake{}
	r.Stdout("  Proto  Local Address          Foreign Address        State           PID\r\n  TCP    127.0.0.1:5432         0.0.0.0:0              ABH\u00d6REN         812\r\n  TCP    127.0.0.1:50000        127.0.0.1:5432         HERGESTELLT     900\r\n", netstat, "-ano", "-p", "tcp")
	old := processInfo
	t.Cleanup(func() { processInfo = old })
	processInfo = func(pid uint32) (process, bool) {
		return process{path: `C:\Program Files\nodejs\node.exe`, args: []string{`C:\Program Files\nodejs\node.exe`, `C:\Users\me\dev\shop\node_modules\vite\bin\vite.js`, "--port", "5432"}, own: true}, pid == 812
	}
	d := &inventory.DatabaseState{}
	databaseListeners(t.Context(), r, env, d)
	l := d.Listeners
	if len(l) != 1 || l[0].Process != "node" || !l[0].Own || len(l[0].Dirs) != 1 || l[0].Dirs[0] != `C:\Users\me\dev\shop\node_modules\vite\bin\vite.js` {
		t.Fatal(l)
	}
}
