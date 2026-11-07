package detect

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func databaseGlobs(env sysenv.Env, command string) []string {
	var out []string
	for _, root := range []string{env.Get("ProgramFiles"), env.Get("ProgramFiles(x86)")} {
		if root != "" {
			for _, pattern := range []string{"PostgreSQL/*/bin", "MySQL/*/bin", "MariaDB*/bin", "Redis", "MongoDB/Server/*/bin", "mongosh",
				"Microsoft SQL Server/MSSQL*/MSSQL/Binn", "Microsoft SQL Server/Client SDK/ODBC/*/Tools/Binn", "SqlCmd"} {
				out = append(out, filepath.Join(root, pattern, command+".exe"))
			}
		}
	}
	if local := env.Get("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "Programs", "mongosh", command+".exe")) // mongosh's installer for one user
	}
	return out
}

// sqlServerVersion reads the instance's setup version from the registry:
// sqlservr.exe is under Microsoft SQL Server\MSSQL16.SQLEXPRESS\MSSQL\Binn,
// and the instance's key is named as that folder.
func sqlServerVersion(_ context.Context, _ run.Runner, _ sysenv.Env, path string) string {
	instance := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path))))
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Microsoft SQL Server\`+instance+`\Setup`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close() //nolint:errcheck // a key opened for reading only
	v, _, err := k.GetStringValue("Version")
	if err != nil {
		return ""
	}
	return databaseVersionOf(v)
}

// listWindowsServices is replaced in tests, which must not see the machine's services.
var listWindowsServices = windowsServices

// databaseServices reads the services from the Service Control Manager,
// not sc.exe, whose output Windows translates.
func databaseServices(_ context.Context, _ run.Runner, _ sysenv.Env, d *inventory.DatabaseState) {
	services, err := listWindowsServices()
	if err != nil {
		d.Warnings = append(d.Warnings, "Service Control Manager")
		return
	}
	addDatabaseServices(d, services)
}

// winService is a Windows service's name and state.
type winService struct {
	name  string
	state uint32
}

// addDatabaseServices adds the services of databases among services.
func addDatabaseServices(d *inventory.DatabaseState, services []winService) {
	for _, s := range services {
		kind := inventory.DatabaseKind(s.name)
		if kind == "" {
			continue
		}
		status := ""
		switch s.state {
		case windows.SERVICE_RUNNING:
			status = "running"
		case windows.SERVICE_STOPPED:
			status = "stopped"
		}
		databaseService(d, kind, s.name, "system", status)
	}
}

// windowsServices lists the services and their states; enumerating them
// needs no administrator.
func windowsServices() ([]winService, error) {
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, fmt.Errorf("open service manager: %w", err)
	}
	defer windows.CloseServiceHandle(h) //nolint:errcheck // a handle opened for reading only
	var needed, count uint32
	var buf []byte
	for {
		var p *byte
		if len(buf) > 0 {
			p = &buf[0]
		}
		err = windows.EnumServicesStatusEx(h, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL,
			p, uint32(len(buf)), &needed, &count, nil, nil)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_MORE_DATA) || needed <= uint32(len(buf)) {
			return nil, fmt.Errorf("list services: %w", err)
		}
		buf = make([]byte, needed)
	}
	if count == 0 {
		return nil, nil
	}
	entries := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(count))
	services := make([]winService, 0, len(entries))
	for _, e := range entries {
		services = append(services, winService{name: windows.UTF16PtrToString(e.ServiceName), state: e.ServiceStatusProcess.CurrentState})
	}
	return services, nil
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "netstat.exe", "-ano", "-p", "tcp")
	if !ok {
		d.Warnings = append(d.Warnings, "netstat")
		return
	}
	for line := range strings.Lines(string(data)) {
		// The state is translated on localized Windows; a listener is the
		// only TCP row without a foreign port.
		f := strings.Fields(line)
		if len(f) != 5 || !strings.HasSuffix(f[2], ":0") {
			continue
		}
		i := strings.LastIndex(f[1], ":")
		if i < 0 {
			continue
		}
		d.Listeners = append(d.Listeners, inventory.DatabaseListener{Address: f[1][:i], Port: f[1][i+1:], PID: f[4]})
	}
}
