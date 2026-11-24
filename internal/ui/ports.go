package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// portsPage is "Ports": who listens on the computer's TCP ports.
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
// Colima's, Podman's and WSL's.
var dockerProcesses = []string{
	"com.docker.backend", "com.docker.vpnkit", "vpnkit-bridge", "docker-proxy",
	"OrbStack Helper", "OrbStack", "limactl", "gvproxy", "wslrelay",
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

// portRows are the listeners of the last scan, one row for each port and
// process, by port.
func (a *App) portRows() []portRow {
	st, _ := a.Snapshot.Tool("databases")
	if st.Databases == nil {
		return nil
	}
	var rows []portRow
	for _, l := range st.Databases.Listeners {
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
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.portsRefreshButton(c) }})
		t := c.Theme()
		pal := Colors(t)
		rows := a.portRows()
		projects, network, dockerCount := 0, 0, 0
		for _, r := range rows {
			if r.Project != "" {
				projects++
			}
			if r.Network {
				network++
			}
			if a.isPortDocker(r) {
				dockerCount++
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("ethernet-port")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.PortsLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.PortsListening(), strconv.Itoa(len(rows)), locales.PortsListeningCaption())
					stateCard(c, t.Success, locales.PortsInProjects(), strconv.Itoa(projects), locales.PortsInProjectsCaption())
					stateCard(c, pal.Info, locales.PortsNetwork(), strconv.Itoa(network), locales.PortsNetworkCaption())
				})
				if a.portsErr != "" {
					warningRow(c, a.portsErr)
				}
				a.forwardList(c, "")
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "ports-tabs", &a.portsTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(rows))},
						{locales.ProjectsTitle(), strconv.Itoa(projects)},
						{locales.PortsTabNetwork(), strconv.Itoa(network)},
						{"Docker", strconv.Itoa(dockerCount)},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("ports-filter"), &a.portsFilter).Bind(&a.filterField).Placeholder(locales.PortsFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					if st, _ := a.Snapshot.Tool("databases"); st.Databases != nil && len(st.Databases.Warnings) > 0 && len(rows) == 0 {
						warningRow(c, locales.DatabaseProbeFailed(strings.Join(st.Databases.Warnings, ", ")))
					}
					a.portsTable(c, rows)
				}, func() {
					listCard(c, locales.PortsHow(), "", func() {
						listRow(c, locales.PortsHowRead(), false, locales.PortsHowReadCaption(), func() {})
						listRow(c, locales.PortsHowStop(), false, locales.PortsHowStopCaption(), func() {})
						listRow(c, locales.PortsHowOthers(), false, locales.PortsHowOthersCaption(), func() {})
					})
				})
			})
		})
	})
}

// portsTable lists the rows the filter keeps: the port, who listens and
// where, the bind address, whether the network reaches it, and the actions.
func (a *App) portsTable(c *ui.Context, rows []portRow) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(strings.TrimSpace(a.portsFilter))
	addrWidth := float32(108)
	actionsWidth := float32(154)
	isNarrow := narrow(c)
	if isNarrow {
		addrWidth = 88
		actionsWidth = 104
	}
	target, hasDockerTarget := a.selectedDockerTarget()
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PortsColPort()).Width(56).Shrink(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.PortsColProcess()).Grow(1).MinWidth(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.PortsColAddress()).Width(addrWidth).Shrink(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.PortsColAccess()).Width(86).Shrink(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Box(c).Width(actionsWidth).Shrink(0).Height(1)
		})
		for _, r := range rows {
			ct, hasContainer := a.portContainerFor(r)
			switch a.portsTab {
			case 1:
				if r.Project == "" {
					continue
				}
			case 2:
				if !r.Network {
					continue
				}
			case 3:
				if !hasContainer && !isDockerProcess(r.Process) {
					continue
				}
			}
			about, plan, stoppable := a.portAbout(r)
			canDocker := hasContainer && a.canOpenDockerContainer()
			canStopDocker := hasContainer && hasDockerTarget && docker.ContainerID(ct.ID) && a.BuildPlan != nil && !a.op.running() && !a.portsBusy
			addrsFull := strings.Join(r.Addresses, ", ")
			if filter != "" && !strings.Contains(strings.ToLower(r.Port+" "+r.Process+" "+about+" "+addrsFull+" "+ct.Image), filter) {
				continue
			}
			shown++
			port := r.Port
			hostAddr := "localhost:" + port
			scheme := "http://"
			if port == "443" || port == "8443" {
				scheme = "https://"
			}
			httpURL := scheme + hostAddr
			ui.Row(c.Key("port:"+r.Port+":"+r.PID)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, r.Port).Font(FontMono).FontSize(13).Width(56).Shrink(0).SingleLine()
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					clipped(c, cmp.Or(r.Process, "—")).FontSize(13)
					clipped(c, about).FontSize(10.5).TextColor(pal.TextFaint)
				})
				addrEl := clipped(c, portBindAddress(r.Addresses)).Font(FontMono).FontSize(11).TextColor(pal.MonoText).Width(addrWidth).Shrink(0)
				if addrsFull != "" {
					addrEl.Tooltip(addrsFull)
				}
				ui.Row(c).Width(86).Shrink(0).AlignItems(ui.Center).Children(func() {
					if r.Network {
						chip(c, pal.Info, pal.InfoSoft, pal.InfoText, locales.PortsNetworkChip())
					} else {
						chip(c, pal.TextFaint, pal.Muted, pal.MonoText, "localhost")
					}
				})
				// Contextual actions sit on the left; always-present Copy and
				// More actions stay in fixed right-aligned columns.
				ui.Row(c).Width(actionsWidth).Gap(2).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Children(func() {
					if !isNarrow && !nonHTTPPorts[port] {
						iconButton(c.Key("port-open"), "external-link", locales.PortsOpenBrowser()).OnClick(func() {
							c.OpenURL(httpURL)
						})
					}
					if canDocker {
						iconButton(c.Key("port-docker"), "container", locales.OpenInDocker()).OnClick(func() {
							a.openDockerContainer(ct.ID, ct.Name)
						})
					}
					if !isNarrow && r.Project != "" {
						iconButton(c.Key("port-project"), "folder-open", locales.OpenProject()).OnClick(func() {
							a.router.Push(projectPath(r.Project))
						})
					}
					if canStopDocker {
						ctCopy := ct
						iconButton(c.Key("port-stop-docker"), "square", locales.DatabaseStopContainer()).OnClick(func() {
							a.planDockerPortStop(target, ctCopy)
						})
					} else if stoppable {
						iconButton(c.Key("port-stop"), "square", locales.PortsStop()).OnClick(func() {
							a.Propose(plan)
						})
					}
					iconButton(c.Key("port-copy"), "copy", locales.PortsCopyAddress(port)).OnClick(func() {
						c.WriteClipboard(hostAddr)
						c.Toast(locales.PortsAddressCopied(port))
					})
					iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
						if menu.Item(locales.PortsCopyAddress(port)).Chosen() {
							c.WriteClipboard(hostAddr)
							c.Toast(locales.PortsAddressCopied(port))
						}
						if menu.Item(locales.PortsOpenBrowser()).Chosen() {
							c.OpenURL(httpURL)
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
						if stoppable && menu.Item(locales.PortsStop()).Chosen() {
							a.Propose(plan)
						}
					})
				})
			})
		}
		switch {
		case len(rows) == 0:
			cardNote(c, locales.PortsNone())
		case shown == 0:
			cardNote(c, locales.NothingMatches())
		}
	})
}
