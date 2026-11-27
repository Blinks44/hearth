package detect

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
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

// winService is a Windows service's name, state and start type; the
// start type is read of databases' services alone, 0 when it is not.
type winService struct {
	name         string
	state, start uint32
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
		autostart := ""
		switch s.start {
		case windows.SERVICE_AUTO_START:
			autostart = "on"
		case windows.SERVICE_DEMAND_START, windows.SERVICE_DISABLED:
			autostart = "off"
		}
		databaseService(d, kind, s.name, "system", status, autostart)
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
		s := winService{name: windows.UTF16PtrToString(e.ServiceName), state: e.ServiceStatusProcess.CurrentState}
		if inventory.DatabaseKind(s.name) != "" {
			s.start = serviceStartType(h, s.name)
		}
		services = append(services, s)
	}
	return services, nil
}

// serviceStartType is how the service name starts, as its configuration
// says; 0 when it cannot be read. Reading it needs no administrator.
func serviceStartType(manager windows.Handle, name string) uint32 {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	h, err := windows.OpenService(manager, p, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return 0
	}
	defer windows.CloseServiceHandle(h) //nolint:errcheck // a handle opened for reading only
	var needed uint32
	if err := windows.QueryServiceConfig(h, nil, 0, &needed); !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return 0
	}
	buf := make([]byte, needed)
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
	if err := windows.QueryServiceConfig(h, config, needed, &needed); err != nil {
		return 0
	}
	return config.StartType
}

func databaseListeners(ctx context.Context, r run.Runner, env sysenv.Env, d *inventory.DatabaseState) {
	data, ok := databaseRun(ctx, r, env, "netstat.exe", "-ano", "-p", "tcp")
	if !ok {
		d.Warnings = append(d.Warnings, "netstat")
		return
	}
	procCache := map[uint32]struct {
		p  process
		ok bool
	}{}
	lookupProc := func(pid uint32) (process, bool) {
		if cached, ok := procCache[pid]; ok {
			return cached.p, cached.ok
		}
		p, ok := processInfo(pid)
		procCache[pid] = struct {
			p  process
			ok bool
		}{p: p, ok: ok}
		return p, ok
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
		l := inventory.DatabaseListener{Address: f[1][:i], Port: f[1][i+1:], PID: f[4]}
		if pid, err := strconv.ParseUint(l.PID, 10, 32); err == nil && pid > 4 { // 0 and 4 are the system's
			if p, ok := lookupProc(uint32(pid)); ok {
				l.Path, l.Own = p.path, p.own
				l.PPID = p.ppid
				l.MemoryBytes = p.mem
				l.Process = strings.TrimSuffix(filepath.Base(p.path), filepath.Ext(p.path))
				l.Args = inventory.RedactArgs(p.args)
				for _, arg := range p.args {
					if filepath.IsAbs(arg) && !strings.EqualFold(arg, p.path) {
						l.Dirs = append(l.Dirs, arg)
					}
				}
				cur := p.ppid
				for range 6 {
					parentPID, err := strconv.ParseUint(cur, 10, 32)
					if err != nil || parentPID <= 4 {
						break
					}
					parent, ok := lookupProc(uint32(parentPID))
					if !ok || parent.ppid == "" || parent.ppid == cur || parent.ppid == l.PID || slices.Contains(l.Ancestors, parent.ppid) {
						break
					}
					l.Ancestors = append(l.Ancestors, parent.ppid)
					cur = parent.ppid
				}
			}
		}
		d.Listeners = append(d.Listeners, l)
	}
}

type process struct {
	path string
	ppid string
	mem  int64
	args []string
	own  bool
}

var (
	kernel32DLL              = windows.NewLazySystemDLL("kernel32.dll")
	procK32GetProcessMemInfo = kernel32DLL.NewProc("K32GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// processInfo reads a process' program, command line and whether it
// runs as this user; ok is false when Windows does not let this user
// see it. Tests set their own.
var processInfo = func(pid uint32) (process, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return process{}, false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return process{}, false
	}
	p := process{path: windows.UTF16ToString(buf[:n])}
	var token windows.Token
	if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token) == nil {
		user, err := token.GetTokenUser()
		me, err2 := windows.GetCurrentProcessToken().GetTokenUser()
		p.own = err == nil && err2 == nil && user.User.Sid.Equals(me.User.Sid)
		_ = token.Close()
	}
	var pbi windows.PROCESS_BASIC_INFORMATION
	var retLen uint32
	if windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), &retLen) == nil && pbi.InheritedFromUniqueProcessId > 0 {
		p.ppid = strconv.FormatUint(uint64(pbi.InheritedFromUniqueProcessId), 10)
	}
	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	if r1, _, _ := procK32GetProcessMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.CB)); r1 != 0 {
		p.mem = int64(pmc.WorkingSetSize)
	}
	// The command line comes as a UNICODE_STRING and the text after it.
	var size uint32
	_ = windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation, nil, 0, &size)
	if size > 0 && size < 1<<20 {
		info := make([]byte, size)
		if windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation, unsafe.Pointer(&info[0]), size, &size) == nil {
			line := (*windows.NTUnicodeString)(unsafe.Pointer(&info[0])).String()
			p.args, _ = windows.DecomposeCommandLine(line)
		}
	}
	return p, true
}
