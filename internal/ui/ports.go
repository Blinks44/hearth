package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
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
var dockerProcesses = []string{"com.docker.backend", "com.docker.vpnkit", "vpnkit-bridge", "docker-proxy", "OrbStack Helper", "OrbStack", "limactl", "gvproxy", "wslrelay"}

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
	if st, ok := a.Snapshot.Tool("docker"); ok && st.Docker != nil {
		for _, ct := range st.Docker.PortContainers {
			if (ct.Status == "" || ct.Status == "running") && slices.Contains(ct.Ports, port) {
				return ct, true
			}
		}
		for _, db := range st.Docker.Databases {
			if db.Status == "running" && slices.Contains(db.Ports, port) {
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
	return inventory.DockerPortContainer{}, false
}

// portAbout says who listens at r, and the plan that stops it, if Hearth
// can.
func (a *App) portAbout(r portRow) (about string, plan ops.Plan, ok bool) {
	if ct, found := a.portContainer(r.Port); found {
		return locales.PortsContainer(ct.Name), ops.Plan{}, false
	}
	if slices.Contains(dockerProcesses, r.Process) {
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
			db.Warnings = slices.Clone(res.Warnings)
			next.Tools[i].Databases = &db
			updatedDB = true
		case "docker":
			if res.HasDocker && next.Tools[i].Docker != nil {
				d := *next.Tools[i].Docker
				d.PortContainers = slices.Clone(res.Containers)
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

func (a *App) portsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.portsRefreshButton(c) }})
		t := c.Theme()
		pal := Colors(t)
		rows := a.portRows()
		projects, network := 0, 0
		for _, r := range rows {
			if r.Project != "" {
				projects++
			}
			if r.Network {
				network++
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
				a.forwardList(c, "")
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
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
	if narrow(c) {
		addrWidth = 88
	}
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for _, r := range rows {
			about, plan, stoppable := a.portAbout(r)
			ct, hasContainer := a.portContainer(r.Port)
			canDocker := hasContainer && a.canOpenDockerContainer()
			addrsFull := strings.Join(r.Addresses, ", ")
			if filter != "" && !strings.Contains(strings.ToLower(r.Port+" "+r.Process+" "+about+" "+addrsFull+" "+ct.Image), filter) {
				continue
			}
			shown++
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
				ui.Row(c).Width(76).Gap(2).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Children(func() {
					if canDocker {
						iconButton(c.Key("port-docker"), "container", locales.OpenInDocker()).OnClick(func() {
							a.openDockerContainer(ct.ID, ct.Name)
						})
					}
					if r.Project != "" {
						iconButton(c.Key("port-project"), "folder-open", locales.OpenProject()).OnClick(func() {
							a.router.Push(projectPath(r.Project))
						})
					}
					if stoppable {
						iconButton(c.Key("port-stop"), "square", locales.PortsStop()).OnClick(func() {
							a.Propose(plan)
						})
					}
					if !stoppable && r.Project == "" && !canDocker {
						return
					}
					iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
						if canDocker && menu.Item(locales.OpenInDocker()).Chosen() {
							a.openDockerContainer(ct.ID, ct.Name)
						}
						if r.Project != "" && menu.Item(locales.OpenProject()).Chosen() {
							a.router.Push(projectPath(r.Project))
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
			cardNote(c, locales.CleanupNone())
		}
	})
}
