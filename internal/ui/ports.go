package ui

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// portsPage is "Services & Ports": dev servers, containers, local databases,
// k8s port-forwards, and listening TCP ports.
const portsPage = "/system/ports"

// portRow is a row of the page: a port and the process listening at it,
// on one address or several.
type portRow struct {
	inventory.DatabaseListener
	// Addresses are the unique bind addresses observed for this port and PID.
	Addresses []string
	// Network reports that it listens beyond this computer: on all
	// addresses or one of the network's.
	Network bool
	// Project is the folder of the project it runs in; empty for none.
	Project string
}

// dockerProcesses forward ports to containers: Docker Desktop's, OrbStack's,
// Colima's, Kiri's, Podman's and WSL's.
var dockerProcesses = []string{
	"com.docker.backend", "com.docker.vpnkit", "vpnkit-bridge", "docker-proxy",
	"OrbStack Helper", "OrbStack", "limactl", "gvproxy", "wslrelay", "kiri",
	"vfkit", "krunkit", "colima", "rootlesskit", "slirp4netns", "pasta", "podman",
}

func isDockerProcess(name string) bool {
	return slices.Contains(dockerProcesses, name) || strings.HasPrefix(name, "qemu-system")
}

// nonHTTPPorts are common database, broker, and SSH ports that should not
// offer a prominent "Open in browser" quick action button.
var nonHTTPPorts = map[string]bool{
	"22": true, "1433": true, "3306": true, "5432": true, "5672": true,
	"6379": true, "9092": true, "27017": true,
}

// serverProcesses are the programs of the database servers, by kind.
var serverProcesses = map[string]string{"postgres": "PostgreSQL", "mysqld": "MySQL", "mariadbd": "MariaDB", "redis-server": "Redis", "mongod": "MongoDB", "sqlservr": "SQL Server"}

// serverPorts are the ports the servers listen on unless set otherwise.
var serverPorts = map[string]string{"5432": "PostgreSQL", "3306": "MySQL", "6379": "Redis", "27017": "MongoDB", "1433": "SQL Server"}

// isManagedForwardListener reports whether l belongs to an active
// Hearth-managed kubectl port-forward already shown in the table.
func (a *App) isManagedForwardListener(l inventory.DatabaseListener) bool {
	proc := strings.ToLower(strings.TrimSuffix(l.Process, ".exe"))
	for _, f := range a.forwards {
		if f.ended {
			continue
		}
		if f.pod.PID > 0 && l.PID == strconv.Itoa(f.pod.PID) {
			return true
		}
		if proc == "kubectl" && strconv.Itoa(f.target.Local) == l.Port {
			return true
		}
	}
	return false
}

// portRows are the listeners of the last scan, one row for each port and
// process, by port.
func (a *App) portRows() []portRow {
	st, _ := a.Snapshot.Tool("databases")
	if st.Databases == nil {
		return nil
	}
	selfPID := strconv.Itoa(os.Getpid())
	var rows []portRow
	for _, l := range st.Databases.Listeners {
		if (l.PID != "" && l.PID == selfPID) || a.isManagedForwardListener(l) {
			continue
		}
		i := slices.IndexFunc(rows, func(r portRow) bool { return r.Port == l.Port && r.PID == l.PID })
		if i < 0 {
			rows = append(rows, portRow{DatabaseListener: l, Project: a.portProject(l.Dirs)})
			i = len(rows) - 1
		}
		if l.Address != "" && !slices.Contains(rows[i].Addresses, l.Address) {
			rows[i].Addresses = append(rows[i].Addresses, l.Address)
		}
		if !loopback(l.Address) {
			rows[i].Network = true
		}
	}
	slices.SortStableFunc(rows, func(x, y portRow) int {
		px, _ := strconv.Atoi(x.Port)
		py, _ := strconv.Atoi(y.Port)
		return cmp.Compare(px, py)
	})
	return rows
}

func portBindAddress(addrs []string) string {
	switch len(addrs) {
	case 0:
		return "—"
	case 1:
		return addrs[0]
	case 2:
		return addrs[0] + ", " + addrs[1]
	default:
		return addrs[0] + " +" + strconv.Itoa(len(addrs)-1)
	}
}

// loopback reports an address only this computer reaches.
func loopback(address string) bool {
	address = strings.Trim(address, "[]")
	address, _, _ = strings.Cut(address, "%") // 127.0.0.53%lo
	return strings.HasPrefix(address, "127.") || address == "::1" || address == "localhost"
}

// portProject is the folder of the project dirs are in, the innermost.
func (a *App) portProject(dirs []string) string {
	found := ""
	for _, p := range a.Projects {
		for _, d := range dirs {
			if within(filepath.ToSlash(d), filepath.ToSlash(p.Dir)) && len(p.Dir) > len(found) {
				found = p.Dir
			}
		}
	}
	return found
}

// portService is the running service of a database server that listens at
// r: a server's process, or one this user cannot see, on its kind's port.
func (a *App) portService(r portRow) (inventory.Database, bool) {
	kind := serverPorts[r.Port]
	if r.Process != "" {
		kind = serverProcesses[strings.TrimSuffix(r.Process, ".exe")]
		if kind == "MariaDB" || kind == "MySQL" {
			kind = cmp.Or(serverPorts[r.Port], kind)
		}
	}
	if kind == "" {
		return inventory.Database{}, false
	}
	var found []inventory.Database
	for _, db := range inventory.DatabaseServers(a.Snapshot) {
		same := db.Kind == kind || kind == "MySQL" && db.Kind == "MariaDB"
		if same && db.Source != "docker" && db.Service != "" && db.Status == "running" {
			found = append(found, db)
		}
	}
	if len(found) != 1 {
		return inventory.Database{}, false
	}
	return found[0], true
}

// portContainer is the running Docker container publishing port; false for none.
func (a *App) portContainer(port string) (inventory.DockerPortContainer, bool) {
	var r portRow
	r.Port = port
	return a.portContainerFor(r)
}

func (a *App) portContainerFor(r portRow) (inventory.DockerPortContainer, bool) {
	if st, ok := a.Snapshot.Tool("docker"); ok && st.Docker != nil {
		for _, ct := range st.Docker.PortContainers {
			if (ct.Status == "" || ct.Status == "running") && containerMatchesPort(ct, r) {
				return ct, true
			}
		}
		if !st.Docker.PortContainersFresh {
			for _, db := range st.Docker.Databases {
				if db.Status == "running" && slices.Contains(db.Ports, r.Port) {
					return inventory.DockerPortContainer{
						ID:     db.ContainerID,
						Name:   db.Name,
						Image:  db.Version,
						Status: db.Status,
						Ports:  db.Ports,
					}, true
				}
			}
		}
	}
	return inventory.DockerPortContainer{}, false
}

func containerMatchesPort(ct inventory.DockerPortContainer, r portRow) bool {
	if r.Process != "" && !isDockerProcess(r.Process) && r.Process != "ssh" {
		return false
	}
	if len(ct.Bindings) == 0 {
		return slices.Contains(ct.Ports, r.Port)
	}
	addrs := r.Addresses
	if len(addrs) == 0 && r.Address != "" {
		addrs = []string{r.Address}
	}
	for _, b := range ct.Bindings {
		if b.HostPort != r.Port {
			continue
		}
		if len(addrs) == 0 {
			return true
		}
		if bindingMatchesAddresses(b.HostIP, addrs) {
			return true
		}
	}
	return false
}

func isWildcardIP(ip string) bool {
	return ip == "" || ip == "0.0.0.0" || ip == "::" || ip == "*"
}

func bindingMatchesAddresses(hostIP string, addrs []string) bool {
	if isWildcardIP(hostIP) {
		return true
	}
	for _, addr := range addrs {
		if addr == hostIP || isWildcardIP(addr) {
			return true
		}
	}
	return false
}

// portAbout says who listens at r, and the plan that stops it, if Hearth
// can.
func (a *App) portAbout(r portRow) (about string, plan ops.Plan, ok bool) {
	if ct, found := a.portContainerFor(r); found {
		return locales.PortsContainer(ct.Name), ops.Plan{}, false
	}
	if isDockerProcess(r.Process) {
		return locales.PortsDocker(), ops.Plan{}, false
	}
	if db, found := a.portService(r); found {
		plan, ok = ops.ServicePlan(ops.ServiceStop, db, a.detect("databases"))
		return locales.PortsService(db.Service), plan, ok
	}
	if r.PID == "" {
		return locales.PortsOther(), ops.Plan{}, false
	}
	if r.Process == "" { // Windows names the PID but not whose it is
		return locales.PortsPID(r.PID) + " · " + locales.PortsOther(), ops.Plan{}, false
	}
	parts := []string{locales.PortsPID(r.PID)}
	switch {
	case r.Process == "ControlCenter" && (r.Port == "5000" || r.Port == "7000"):
		parts = append(parts, locales.PortsAirPlay())
	case r.Path != "" && ops.SystemProgram(r.Path):
		parts = append(parts, locales.PortsSystem())
	case r.Project != "":
		parts = append(parts, filepath.Base(r.Project))
	case !r.Own:
		parts = append(parts, locales.PortsNotYours())
	case len(r.Dirs) > 0:
		parts = append(parts, tilde(r.Dirs[0], a.Home))
	}
	plan, ok = ops.ProcessPlan(r.DatabaseListener, a.detect("databases"))
	return strings.Join(parts, " · "), plan, ok
}

func (a *App) refreshPortsNow() {
	if a.RefreshPorts == nil || a.portsRefreshing || a.Snapshot == nil {
		return
	}
	a.portsRefreshing = true
	a.portsRefreshSeq++
	seq := a.portsRefreshSeq
	a.RefreshPorts(func(res detect.PortsSnapshot) {
		if a.portsRefreshSeq != seq {
			return
		}
		a.portsRefreshing = false
		a.applyPortsRefresh(res)
	})
}

func (a *App) applyPortsRefresh(res detect.PortsSnapshot) {
	if a.Snapshot == nil {
		return
	}
	a.portsLastRefresh = time.Now()
	next := *a.Snapshot
	next.Tools = slices.Clone(a.Snapshot.Tools)
	updatedDB := false
	for i := range next.Tools {
		switch next.Tools[i].Tool {
		case "databases":
			var db inventory.DatabaseState
			if next.Tools[i].Databases != nil {
				db = *next.Tools[i].Databases
			}
			db.Listeners = slices.Clone(res.Listeners)
			db.Warnings = mergeListenerWarnings(db.Warnings, res.Warnings)
			next.Tools[i].Databases = &db
			updatedDB = true
		case "docker":
			if next.Tools[i].Docker != nil && (res.DockerSelected == "" || res.DockerSelected == next.Tools[i].Docker.Selected) {
				d := *next.Tools[i].Docker
				if res.HasDocker {
					d.PortContainers = slices.Clone(res.Containers)
				} else {
					d.PortContainers = nil
				}
				d.PortContainersFresh = true
				next.Tools[i].Docker = &d
			}
		}
	}
	if !updatedDB {
		next.Tools = append(next.Tools, inventory.ToolState{
			Tool: "databases",
			Databases: &inventory.DatabaseState{
				Listeners: slices.Clone(res.Listeners),
				Warnings:  slices.Clone(res.Warnings),
			},
		})
	}
	a.Snapshot = &next
}

func mergeListenerWarnings(existing, fresh []string) []string {
	var out []string
	for _, w := range existing {
		if w != "lsof" && w != "ss" && w != "netstat" && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	for _, w := range fresh {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func (a *App) portsRefreshButton(c *ui.Context) {
	t := c.Theme()
	label := locales.PortsRefresh()
	if a.portsRefreshing {
		label = locales.PortsRefreshing()
	}
	ui.Button(c.Key("ports-refresh"), "").Height(controlHeight).Label(label).
		Disabled(a.RefreshPorts == nil || a.portsRefreshing || a.Snapshot == nil).
		OnClick(a.refreshPortsNow).Children(func() {
		spinIcon(c, "refresh-cw", 14, t.Text, a.portsRefreshing)
		ui.Text(c, label).FontSize(12.5).SingleLine()
	})
}

func (a *App) isPortDocker(r portRow) bool {
	if _, ok := a.portContainerFor(r); ok {
		return true
	}
	return isDockerProcess(r.Process)
}

func (a *App) isPortDatabase(r portRow) bool {
	if _, ok := a.portService(r); ok {
		return true
	}
	return serverProcesses[strings.TrimSuffix(r.Process, ".exe")] != ""
}

func (a *App) activeForwards() []*portForward {
	var out []*portForward
	for _, f := range a.forwards {
		if !f.ended {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) planDockerPortStop(target docker.Target, ct inventory.DockerPortContainer) {
	if a.BuildPlan == nil || a.op.running() || a.portsBusy {
		return
	}
	a.portsBusy = true
	a.portsErr = ""
	item := docker.Container{ID: ct.ID, Name: ct.Name}
	item.State.Status = "running"
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerContainers(target, env, []docker.Container{item}, ops.DockerStop)
	}, func(p ops.Plan, err error) {
		a.portsBusy = false
		if err != nil {
			a.portsErr = err.Error()
			return
		}
		if a.router.Path() != portsPage || a.op.running() {
			return
		}
		a.Propose(p)
	})
}

func (a *App) portsView(c *ui.Context, p page) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if !a.portsLastRefresh.IsZero() && !narrow(c) {
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.PortsUpdatedAgo(ago(c, a.portsLastRefresh)))
				}
				a.portsRefreshButton(c)
			})
		}})

		rows := a.portRows()
		forwards := a.activeForwards()
		devCount, dockerCount, dbCount, networkCount := 0, 0, 0, 0
		var totalCPU float64
		var totalMem int64
		seenPID := make(map[string]struct{}, len(rows))
		for _, r := range rows {
			if r.Network {
				networkCount++
			}
			if r.PID == "" {
				totalCPU += r.CPUPercent
				totalMem += r.MemoryBytes
			} else if _, dup := seenPID[r.PID]; !dup {
				seenPID[r.PID] = struct{}{}
				totalCPU += r.CPUPercent
				totalMem += r.MemoryBytes
			}
			switch {
			case a.isPortDocker(r):
				dockerCount++
			case a.isPortDatabase(r):
				dbCount++
			case inventory.ClassifyDevServer(r.DatabaseListener).Role == inventory.RoleDevServer || a.scriptRunForListener(r.DatabaseListener) != nil:
				devCount++
			}
		}
		totalCount := len(rows) + len(forwards)

		var breakdown []string
		if devCount > 0 {
			breakdown = append(breakdown, locales.PortsStatDev(devCount))
		}
		if dockerCount > 0 {
			breakdown = append(breakdown, strconv.Itoa(dockerCount)+" Docker")
		}
		if dbCount > 0 {
			breakdown = append(breakdown, strconv.Itoa(dbCount)+" "+locales.PortsTabDatabases())
		}
		if len(forwards) > 0 {
			breakdown = append(breakdown, strconv.Itoa(len(forwards))+" "+locales.PortsTabK8s())
		}
		runningCaption := strings.Join(breakdown, " · ")
		if runningCaption == "" {
			runningCaption = locales.PortsListeningCaption()
		}

		loadVal := fmt.Sprintf("%.1f%% CPU", totalCPU)
		loadCaption := locales.PortsStatLoadRAM("0 B")
		if totalMem > 0 {
			loadCaption = locales.PortsStatLoadRAM(formatBytes(totalMem))
		}

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("ethernet-port")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.ServicesPortsLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.PortsStatRunning(), strconv.Itoa(totalCount), runningCaption)
					stateCard(c, t.Success, locales.PortsStatLoad(), loadVal, loadCaption)
					stateCard(c, pal.Info, locales.PortsNetwork(), strconv.Itoa(networkCount), locales.PortsNetworkCaption())
				})

				if a.portsErr != "" {
					warningRow(c, a.portsErr)
				}

				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "ports-tabs", &a.portsTab, []segment{
						{locales.TabAll(), strconv.Itoa(totalCount)},
						{locales.PortsTabDevServers(), strconv.Itoa(devCount)},
						{"Docker", strconv.Itoa(dockerCount)},
						{locales.PortsTabDatabases(), strconv.Itoa(dbCount)},
						{locales.PortsTabK8s(), strconv.Itoa(len(forwards))},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("ports-filter"), &a.portsFilter).Bind(&a.filterField).Placeholder(locales.PortsFilter()).Height(controlHeight).Width(200)
				})

				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					if st, _ := a.Snapshot.Tool("databases"); st.Databases != nil && len(st.Databases.Warnings) > 0 && totalCount == 0 {
						warningRow(c, locales.DatabaseProbeFailed(strings.Join(st.Databases.Warnings, ", ")))
					}
					a.portsTable(c, rows, forwards)
				}, func() {
					listCard(c, locales.PortsQuickTitle(), "", func() {
						quickRow := func(title, body string) {
							ui.Column(c).Gap(2).Padding(10, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
								ui.Text(c, title).FontSize(13).FontWeight(500).SingleLine()
								ui.Text(c, body).FontSize(11.5).TextColor(pal.TextFaint).Wrap()
							})
						}
						quickRow(locales.PortsQuickActions(), locales.PortsQuickActionsBody())
						quickRow(locales.PortsQuickProtect(), locales.PortsQuickProtectBody())
						quickRow(locales.PortsQuickTray(), locales.PortsQuickTrayBody())
					})
				})
			})
		})
	})
}

// portsTable renders the unified Services & Ports table matching Sketch frame 56:
// Port | Category tag + Service/Project + Subtitle | Metrics / Access + Actions.
func (a *App) portsTable(c *ui.Context, rows []portRow, forwards []*portForward) {
	t := c.Theme()
	pal := Colors(t)
	filter := strings.ToLower(strings.TrimSpace(a.portsFilter))
	isNarrow := narrow(c)
	target, hasDockerTarget := a.selectedDockerTarget()
	shown := 0

	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PortsColPort()).Width(60).Shrink(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.PortsColService()).Grow(1).MinWidth(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.PortsColMetricsActions()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine().Shrink(0)
		})

		for _, r := range rows {
			ct, hasContainer := a.portContainerFor(r)
			dbSvc, hasDB := a.portService(r)
			info := inventory.ClassifyDevServer(r.DatabaseListener)
			managedRun := a.scriptRunForListener(r.DatabaseListener)
			isDocker := hasContainer || isDockerProcess(r.Process)
			isDB := !isDocker && (hasDB || serverProcesses[strings.TrimSuffix(r.Process, ".exe")] != "")
			isDev := !isDocker && !isDB && (info.Role == inventory.RoleDevServer || managedRun != nil)

			switch a.portsTab {
			case 1:
				if !isDev {
					continue
				}
			case 2:
				if !isDocker {
					continue
				}
			case 3:
				if !isDB {
					continue
				}
			case 4:
				continue // k8s tab shows port-forwards below
			}

			about, plan, stoppable := a.portAbout(r)
			canDocker := hasContainer && a.canOpenDockerContainer()
			canStopDocker := hasContainer && hasDockerTarget && docker.ContainerID(ct.ID) && a.BuildPlan != nil && !a.op.running() && !a.portsBusy
			addrsFull := strings.Join(r.Addresses, ", ")
			searchBlob := strings.ToLower(r.Port + " " + r.Process + " " + info.Kind + " " + about + " " + addrsFull + " " + ct.Name + " " + ct.Image + " " + dbSvc.Kind + " " + dbSvc.Service)
			if managedRun != nil {
				searchBlob += " " + strings.ToLower(managedRun.script.Name+" "+filepath.Base(managedRun.dir))
			}
			if filter != "" && !strings.Contains(searchBlob, filter) {
				continue
			}

			shown++
			port := r.Port
			hostAddr := "localhost:" + port
			httpURL := listenerURL(port)
			httpOpenable := (info.HTTPOpenable || managedRun != nil) && !nonHTTPPorts[port]

			// Determine colored category tag & primary display title matching Sketch 56.
			var tagText string
			var tagBg, tagFg ui.Color
			title := cmp.Or(r.Process, "—")
			switch {
			case isDocker:
				tagText, tagBg, tagFg = "Docker", pal.InfoSoft, pal.InfoText
				if hasContainer && ct.Name != "" {
					title = strings.TrimPrefix(ct.Name, "/")
				}
			case isDB:
				tagText, tagBg, tagFg = locales.PortsDbTag(), pal.SuccessSoft, pal.SuccessText
				if hasDB && dbSvc.Kind != "" {
					title = dbSvc.Kind
					if dbSvc.Version != "" {
						major, _, _ := strings.Cut(dbSvc.Version, ".")
						if major != "" {
							title += " " + major
						}
					}
				} else if kind := serverProcesses[strings.TrimSuffix(r.Process, ".exe")]; kind != "" {
					title = kind
				}
			case isDev:
				tagText, tagBg, tagFg = cmp.Or(info.Kind, "Dev"), pal.AccentSoft, pal.AccentInk
				if managedRun != nil {
					title = filepath.Base(managedRun.dir)
					if managedRun.script.Name != "" && !strings.EqualFold(managedRun.script.Name, "dev") {
						title += " · " + managedRun.script.Name
					}
				} else if r.Project != "" {
					title = filepath.Base(r.Project)
				}
			}

			// Subtitle: keep informative context (bind addr, project/command/container/service).
			bindStr := portBindAddress(r.Addresses)
			sub := about
			if bindStr != "" && bindStr != "—" && !strings.Contains(sub, bindStr) {
				sub = sub + " · " + bindStr
			}

			metrics := formatCPUAndMem(r.CPUPercent, r.MemoryBytes)
			portText := ":" + port

			ui.Row(c.Key("port:"+r.Port+":"+r.PID)).Gap(12).Padding(8, 16).MinHeight(50).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, portText).Font(FontMono).FontSize(13).Width(60).Shrink(0).SingleLine()

				ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
					if tagText != "" {
						ui.Box(c).Width(62).Shrink(0).Padding(3, 6).Radius(6).Center().Background(tagBg).Children(func() {
							clipped(c, tagText).FontSize(11).FontWeight(500).TextColor(tagFg)
						})
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							clipped(c, title).MinWidth(0).FontSize(13).FontWeight(500)
							if managedRun != nil {
								aiTag(c, locales.PortsManagedByHearth(), pal.SuccessSoft, pal.SuccessText).Shrink(0)
							}
						})
						subTip := sub
						if len(r.Addresses) > 2 && addrsFull != "" {
							subTip = about + " · " + addrsFull
						}
						ui.Text(c, sub).SingleLine().FontSize(11).TextColor(pal.TextFaint).Tooltip(subTip)
					})
				})

				ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
					if !isNarrow && metrics != "" {
						ui.Text(c, metrics).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine().Shrink(0)
					}
					if r.Network {
						if !stoppable && !canStopDocker && managedRun == nil && metrics == "" && bindStr != "" && bindStr != "—" {
							ui.Text(c, bindStr).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine().Shrink(0)
						}
						if !isNarrow || (!stoppable && !canStopDocker && managedRun == nil) {
							chip(c, pal.Info, pal.InfoSoft, pal.InfoText, locales.PortsNetworkChip())
						}
					}

					ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
						if !isNarrow && managedRun != nil {
							runCopy := managedRun
							subtleSquareIconButton(c, "port-logs", "scroll-text", locales.PortsOpenLogs(), 26, func() {
								a.OpenProjectScriptLogs(runCopy.dir, runCopy.script.Source, runCopy.script.Name)
							})
							subtleSquareIconButton(c, "port-restart", "refresh-cw", locales.ProjectScriptsRestart(), 26, func() {
								a.restartScript(runCopy)
							})
						}
						if !isNarrow && httpOpenable && (r.Own || isDocker || managedRun != nil) {
							subtleSquareIconButton(c, "port-open", "external-link", locales.PortsOpenBrowser(), 26, func() {
								c.OpenURL(httpURL)
							})
						}
						if canDocker {
							subtleSquareIconButton(c, "port-docker", "container", locales.OpenInDocker(), 26, func() {
								a.openDockerContainer(ct.ID, ct.Name)
							})
						}
						if !isNarrow && managedRun == nil && r.Project != "" {
							subtleSquareIconButton(c, "port-project", "folder-open", locales.OpenProject(), 26, func() {
								a.router.Push(projectPath(r.Project))
							})
						}
						switch {
						case managedRun != nil:
							runCopy := managedRun
							dangerIconButton(c, "port-stop-managed", "square", locales.ProjectScriptsStop(), 26, func() {
								a.stopScript(runCopy)
							})
						case canStopDocker:
							ctCopy := ct
							dangerIconButton(c, "port-stop-docker", "square", locales.DatabaseStopContainer(), 26, func() {
								a.planDockerPortStop(target, ctCopy)
							})
						case stoppable:
							dangerIconButton(c, "port-stop", "square", locales.PortsStop(), 26, func() {
								a.Propose(plan)
							})
						}
						iconButton(c, "ellipsis", locales.MoreActions()).Size(26, 26).Menu(func(menu *ui.Menu) {
							if menu.Item(locales.PortsCopyAddress(port)).Chosen() {
								c.WriteClipboard(hostAddr)
								c.Toast(locales.PortsAddressCopied(port))
							}
							if menu.Item(locales.PortsOpenBrowser()).Chosen() {
								c.OpenURL(httpURL)
							}
							if managedRun != nil {
								if menu.Item(locales.PortsOpenLogs()).Chosen() {
									a.OpenProjectScriptLogs(managedRun.dir, managedRun.script.Source, managedRun.script.Name)
								}
								if menu.Item(locales.ProjectScriptsRestart()).Chosen() {
									a.restartScript(managedRun)
								}
								if menu.Item(locales.ProjectScriptsStop()).Chosen() {
									a.stopScript(managedRun)
								}
							}
							if canDocker && menu.Item(locales.OpenInDocker()).Chosen() {
								a.openDockerContainer(ct.ID, ct.Name)
							}
							if r.Project != "" && menu.Item(locales.OpenProject()).Chosen() {
								a.router.Push(projectPath(r.Project))
							}
							if canStopDocker && menu.Item(locales.DatabaseStopContainer()).Chosen() {
								a.planDockerPortStop(target, ct)
							}
							if managedRun == nil && stoppable && menu.Item(locales.PortsStop()).Chosen() {
								a.Propose(plan)
							}
						})
					})
				})
			})
		}

		// Active Kubernetes port-forwards (inline in table under All and k8s tabs).
		if a.portsTab == 0 || a.portsTab == 4 {
			for _, fwd := range forwards {
				f := fwd
				portStr := strconv.Itoa(f.target.Local)
				title := f.target.Name
				if f.target.Context != "" {
					title = f.target.Context + " · " + f.target.Name
				}
				sub := "kubectl port-forward → :" + strconv.Itoa(f.target.Remote)
				if filter != "" && !strings.Contains(strings.ToLower(portStr+" k8s "+title+" "+sub), filter) {
					continue
				}
				shown++
				httpURL := listenerURL(portStr)
				ui.Row(c.Key("port-fwd:"+portStr)).Gap(12).Padding(8, 16).MinHeight(50).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Text(c, ":"+portStr).Font(FontMono).FontSize(13).Width(60).Shrink(0).SingleLine()
					ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Width(62).Shrink(0).Padding(3, 6).Radius(6).Center().Background(pal.WarningSoft).Children(func() {
							ui.Text(c, "k8s").FontSize(11).FontWeight(500).TextColor(pal.WarningText).SingleLine()
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							clipped(c, title).FontSize(13).FontWeight(500)
							clipped(c, sub).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
						})
					})
					ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
						if f.stopping {
							chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.KubeForwardStopping())
						} else {
							if !nonHTTPPorts[portStr] {
								subtleSquareIconButton(c, "fwd-open:"+portStr, "external-link", locales.PortsOpenBrowser(), 26, func() {
									c.OpenURL(httpURL)
								})
							}
							if f.stop != nil {
								dangerIconButton(c, "fwd-stop:"+portStr, "square", locales.ProjectScriptsStop(), 26, func() {
									f.stopping = true
									f.stop()
								})
							}
						}
					})
				})
			}
		}

		switch {
		case len(rows) == 0 && len(forwards) == 0:
			cardNote(c, locales.PortsNone())
		case shown == 0:
			cardNote(c, locales.NothingMatches())
		}
	})
}
