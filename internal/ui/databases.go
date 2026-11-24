package ui

import (
	"cmp"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) databases(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			a.refreshScanButton(c, "databases-refresh")
			a.databaseInstallMenu(c)
		}})
		st, ok := a.Snapshot.Tool("databases")
		if !ok || st.Databases == nil {
			a.toolEmpty(c, "databases", p.title())
			return
		}
		d := st.Databases
		servers := inventory.DatabaseServers(a.Snapshot)
		var conflicts []issues.Issue
		for _, is := range a.issues(c) {
			if is.Kind == issues.DatabasePortConflict {
				conflicts = append(conflicts, is)
			}
		}
		servicesCount, containersCount, projectsCount := 0, 0, 0
		for _, db := range servers {
			if db.Source == "docker" {
				containersCount++
			} else if db.Service != "" {
				servicesCount++
			}
		}
		for _, pr := range a.Projects {
			projectsCount += len(pr.Databases)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pal := Colors(c.Theme())
				running := 0
				for _, db := range servers {
					if db.Status == "running" {
						running++
					}
				}
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("database")).Size(24, 24).TextColor(pal.MonoText) })
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.DatabaseSubtitle()).FontSize(12.5).TextColor(c.Theme().TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.DatabaseServers(), strconv.Itoa(len(servers)), strings.Join(databaseSources(servers), " · "))
					stateCard(c, c.Theme().Success, locales.DatabaseRunningCount(), strconv.Itoa(running), "")
					stateCard(c, c.Theme().Warning, locales.DatabaseConflicts(), strconv.Itoa(len(conflicts)), locales.DatabasePorts())
				})
				for _, is := range conflicts {
					title, explain := issueText(is, a.Home)
					var action func()
					dirKey := ""
					if len(is.Dirs) > 0 {
						dirKey = is.Dirs[0]
					}
					if plan, ok := a.conflictPlan(is, servers); ok {
						action = func() {
							ui.Button(c.Key("stop:"+is.Project+dirKey), locales.ServiceStop()).Shrink(0).OnClick(func() { a.Propose(plan) })
						}
					} else if ct, ok := a.conflictContainer(is, servers); ok {
						target, hasTarget := a.selectedDockerTarget()
						canStop := hasTarget && docker.ContainerID(ct.ID) && a.BuildPlan != nil && !a.op.running() && !a.databaseBusy
						canOpen := a.canOpenDockerContainer()
						if canStop || canOpen {
							action = func() {
								ui.Row(c).Gap(6).Shrink(0).AlignItems(ui.Center).Children(func() {
									if canStop {
										ui.Button(c.Key("stop-container:"+is.Project+dirKey), locales.DatabaseStopContainer()).Shrink(0).OnClick(func() {
											item := docker.Container{ID: ct.ID, Name: ct.Name}
											item.State.Status = "running"
											a.planDockerDatabase(target, item, ops.DockerStop)
										})
									}
									if canOpen {
										ui.Button(c.Key("open-docker:"+is.Project+dirKey), locales.OpenInDocker()).Shrink(0).OnClick(func() {
											a.openDockerContainer(ct.ID, ct.Name)
										})
									}
								})
							}
						}
					}
					warningWith(c, title+". "+explain, action)
				}
				if a.databaseErr != "" {
					warningRow(c, a.databaseErr)
				}
				if len(d.Warnings) > 0 {
					warningRow(c, locales.DatabaseProbeFailed(strings.Join(d.Warnings, ", ")))
				}
				if dockerSt, ok := a.Snapshot.Tool("docker"); ok && dockerSt.Docker != nil && dockerSt.Docker.DatabaseErr {
					warningRow(c, locales.DatabaseContainersFailed())
				}
				toolbar := ui.Row(c).Gap(12).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "database-tabs", &a.databaseTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(servers))},
						{locales.DatabaseServices(), strconv.Itoa(servicesCount)},
						{locales.ProjectsTitle(), strconv.Itoa(projectsCount)},
						{locales.DatabaseContainers(), strconv.Itoa(containersCount)},
					})
					ui.Box(c).Grow(1)
					ui.SearchField(c.Key("database-filter"), &a.databaseFilter).Bind(&a.filterField).Placeholder(locales.DatabaseFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					if a.databaseTab == 2 {
						a.databaseProjects(c, true)
					} else {
						a.databaseTable(c, servers, d.Clients, d.Listeners)
					}
					toolNote(c, locales.DatabaseReadOnly())
					listCard(c, locales.DatabaseListeners(), locales.DatabaseObserved(), func() {
						shown := 0
						for _, l := range d.Listeners {
							if !slices.Contains([]string{"5432", "3306", "6379", "27017", "1433"}, l.Port) && !databaseProjectPort(a, l.Port) {
								continue
							}
							shown++
							listRow(c, l.Address+":"+l.Port, true, l.Process, func() { ui.Text(c, l.PID).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint) })
						}
						if shown == 0 {
							cardNote(c, locales.DatabaseNoListeners())
						}
					})
				}, func() {
					listCard(c, locales.DatabaseClients(), "", func() {
						for _, client := range d.Clients {
							listRow(c, client.Tool+" "+client.Version, false, tilde(client.Path, a.Home), func() {
								if a.OpenDBClient != nil && client.Path != "" {
									iconButton(c.Key("client-open:"+client.Path), "terminal", locales.DatabaseOpenClient(client.Tool)).OnClick(func() {
										a.launchDBClient([]string{client.Path})
									})
								}
							})
						}
						if len(d.Clients) == 0 {
							cardNote(c, locales.DatabaseNoClients())
						}
					})
					listCard(c, locales.DatabaseAutostart(), "", func() {
						shown := 0
						for _, db := range servers {
							if db.Service == "" || db.Source == "docker" {
								continue
							}
							shown++
							text, _ := databaseStatus(c, db.Status)
							listRow(c, db.Service, false, text, func() { a.autostartSwitch(c, db) })
						}
						if shown == 0 {
							cardNote(c, locales.DatabaseNoServices())
						}
					})
					if a.databaseTab != 2 {
						a.databaseProjects(c, false)
					}
				})
			})
		})
	})
}

func databaseStatus(c *ui.Context, status string) (string, ui.Color) {
	switch status {
	case "running":
		return locales.DatabaseRunning(), c.Theme().Success
	case "stopped":
		return locales.DatabaseStopped(), Colors(c.Theme()).TextFaint
	case "error":
		return locales.DatabaseError(), c.Theme().Warning
	default:
		return locales.DatabaseUnknown(), Colors(c.Theme()).TextFaint
	}
}

func (a *App) launchDBClient(argv []string) {
	if a.OpenDBClient == nil || len(argv) == 0 {
		return
	}
	a.databaseErr = ""
	a.OpenDBClient(argv, func(err error) {
		if err != nil {
			a.databaseErr = err.Error()
		}
	})
}

func (a *App) dockerDatabaseAction(db inventory.Database) (ops.DockerAction, docker.Target, docker.Container, bool) {
	if db.Source != "docker" || !docker.ContainerID(db.ContainerID) || a.BuildPlan == nil || a.op.running() || a.databaseBusy {
		return "", docker.Target{}, docker.Container{}, false
	}
	target, ok := a.selectedDockerTarget()
	if !ok {
		return "", docker.Target{}, docker.Container{}, false
	}
	ct := docker.Container{ID: db.ContainerID, Name: db.Name}
	switch db.Status {
	case "running":
		ct.State.Status = "running"
		return ops.DockerStop, target, ct, true
	case "stopped":
		ct.State.Status = "exited"
		return ops.DockerStart, target, ct, true
	default:
		return "", docker.Target{}, docker.Container{}, false
	}
}

func (a *App) planDockerDatabase(target docker.Target, ct docker.Container, action ops.DockerAction) {
	if a.BuildPlan == nil || a.op.running() || a.databaseBusy {
		return
	}
	a.databaseBusy = true
	a.databaseErr = ""
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerContainers(target, env, []docker.Container{ct}, action)
	}, func(p ops.Plan, err error) {
		a.databaseBusy = false
		if err != nil {
			a.databaseErr = err.Error()
			return
		}
		if a.router.Path() != "/tools/databases" || a.op.running() {
			return
		}
		a.Propose(p)
	})
}

func (a *App) databaseTable(c *ui.Context, servers []inventory.Database, clients []inventory.Installation, listeners []inventory.DatabaseListener) {
	pal := Colors(c.Theme())
	user := filepath.Base(a.Home)
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.DatabaseServer()).Width(nameWidth(c)).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.ColumnSource()).Width(76).Shrink(0).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			ui.Text(c, locales.DatabaseAddress()).FontSize(11.5).TextColor(pal.TextFaint)
		})
		shown := 0
		for i, db := range servers {
			if a.databaseTab == 1 && (db.Service == "" || db.Source == "docker") || a.databaseTab == 3 && db.Source != "docker" {
				continue
			}
			if !strings.Contains(strings.ToLower(db.Name+" "+db.Kind+" "+db.Source), strings.ToLower(a.databaseFilter)) {
				continue
			}
			shown++
			port := databasePort(db, listeners)
			url, password := databaseURL(db, user, port)
			client, hasClient := databaseClientFor(db, clients)
			var clientCmd []string
			if hasClient && a.OpenDBClient != nil && db.Status == "running" {
				clientCmd = databaseClientCommand(db, client, port, user)
			}
			ui.Row(c.Key(strconv.Itoa(i))).Gap(12).Padding(12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Width(nameWidth(c)).Gap(3).Children(func() {
					title, subtitle := db.Name+" "+db.Version, db.Service
					if db.Source == "docker" {
						title, subtitle = db.Version, db.Name
					}
					clipped(c, title).Font(FontMono).FontSize(12)
					clipped(c, subtitle).FontSize(10.5).TextColor(pal.TextFaint)
				})
				ui.Text(c, databaseSource(db.Source)).Width(76).Shrink(0).FontSize(10.5).TextColor(pal.MonoText).SingleLine()
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					address := cmp.Or(url, tilde(db.Path, a.Home))
					if db.Source == "docker" && address == "" {
						address = strings.Join(db.Ports, ", ")
					}
					if address == "" {
						address = "—"
					}
					addrEl := clipped(c, address).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
					switch {
					case db.Source == "docker" && len(db.Ports) > 1 && url == "":
						addrEl.Tooltip(strings.Join(db.Ports, ", "))
					case address != "—":
						addrEl.Tooltip(address)
					}
					if password && url != "" {
						ui.Text(c, locales.DatabasePassword()).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					}
					text, dot := databaseStatus(c, db.Status)
					bg, ink := pal.Muted, pal.MonoText
					if db.Status == "running" {
						bg, ink = pal.SuccessSoft, pal.SuccessText
					}
					ui.Row(c).Children(func() { chip(c, dot, bg, ink, text) })
					for _, cl := range clients {
						if db.Path == "" || filepath.Dir(cl.RealPath) != filepath.Dir(db.Path) && filepath.Dir(cl.Path) != filepath.Dir(db.Path) || databaseClientKind(cl.Tool) != db.Kind {
							continue
						}
						ui.Text(c, locales.DatabaseClient(cl.Tool+" "+cl.Version)).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				plan, canService := a.servicePlan(db)
				dockerAction, dockerTarget, dockerCt, canDockerPower := a.dockerDatabaseAction(db)
				canDockerOpen := db.Source == "docker" && a.canOpenDockerContainer() && (db.ContainerID != "" || db.Name != "")
				ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
					if canService {
						icon, label := "play", locales.ServiceStart()
						if db.Status == "running" {
							icon, label = "square", locales.ServiceStop()
						}
						iconButton(c.Key("service-power"), icon, label).OnClick(func() { a.Propose(plan) })
					} else if canDockerPower {
						icon, label := "play", locales.DatabaseStartContainer()
						if dockerAction == ops.DockerStop {
							icon, label = "square", locales.DatabaseStopContainer()
						}
						iconButton(c.Key("docker-power"), icon, label).OnClick(func() {
							a.planDockerDatabase(dockerTarget, dockerCt, dockerAction)
						})
					}
					if len(clientCmd) > 0 {
						iconButton(c.Key("db-client"), "terminal", locales.DatabaseOpenClient(client.Tool)).OnClick(func() {
							a.launchDBClient(clientCmd)
						})
					}
					if canDockerOpen {
						iconButton(c.Key("db-docker"), "container", locales.OpenInDocker()).OnClick(func() {
							a.openDockerContainer(db.ContainerID, db.Name)
						})
					}
					if url != "" {
						iconButton(c, "copy", locales.DatabaseCopyURL()).OnClick(func() { c.WriteClipboard(url); c.Toast(locales.DatabaseURLCopied()) })
					} else if db.Path != "" {
						iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(db.Path); c.Toast(locales.PathCopied()) })
					}
					if db.Path != "" && a.Shell != nil {
						iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(db.Path) })
					}
					hasMenu := canService || canDockerPower || canDockerOpen || len(clientCmd) > 0 || (db.Path != "" && db.Source != "docker")
					if hasMenu {
						iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
							if canService {
								label := locales.ServiceStart()
								if db.Status == "running" {
									label = locales.ServiceStop()
								}
								if menu.Item(label).Chosen() {
									a.Propose(plan)
								}
							}
							if canDockerPower {
								label := locales.DatabaseStartContainer()
								if dockerAction == ops.DockerStop {
									label = locales.DatabaseStopContainer()
								}
								if menu.Item(label).Chosen() {
									a.planDockerDatabase(dockerTarget, dockerCt, dockerAction)
								}
							}
							if len(clientCmd) > 0 && menu.Item(locales.DatabaseOpenClient(client.Tool)).Chosen() {
								a.launchDBClient(clientCmd)
							}
							if canDockerOpen && menu.Item(locales.OpenInDocker()).Chosen() {
								a.openDockerContainer(db.ContainerID, db.Name)
							}
							if db.Path != "" && menu.Item(locales.CopyPath()).Chosen() {
								c.WriteClipboard(db.Path)
								c.Toast(locales.PathCopied())
							}
						})
					}
				})
			})
		}
		if shown == 0 {
			cardNote(c, locales.DatabaseNone())
		}
	})
}

func (a *App) databaseProjects(c *ui.Context, applyFilter bool) {
	listCard(c, locales.DatabaseProjects(), "compose · .env", func() {
		filter := ""
		if applyFilter {
			filter = strings.ToLower(strings.TrimSpace(a.databaseFilter))
		}
		total, shown := 0, 0
		for _, p := range a.Projects {
			for _, ref := range p.Databases {
				total++
				haystack := strings.ToLower(filepath.Base(p.Dir) + " " + ref.File + " " + ref.Kind + " " + ref.Service + " " + strings.Join(ref.Ports, " "))
				if filter != "" && !strings.Contains(haystack, filter) {
					continue
				}
				shown++
				listRow(c, filepath.Base(p.Dir), false, ref.File+" · "+ref.Kind+" · "+strings.Join(ref.Ports, ", "), func() { ui.Button(c, locales.Details()).OnClick(func() { a.router.Push(projectPath(p.Dir)) }) })
			}
		}
		if shown == 0 {
			if total > 0 && filter != "" {
				cardNote(c, locales.NothingMatches())
			} else {
				cardNote(c, locales.DatabaseNoProjects())
			}
		}
	})
}

func databaseProjectPort(a *App, port string) bool {
	for _, p := range a.Projects {
		for _, ref := range p.Databases {
			if slices.Contains(ref.Ports, port) {
				return true
			}
		}
	}
	return false
}

func databaseSources(servers []inventory.Database) []string {
	var out []string
	for _, db := range servers {
		if s := databaseSource(db.Source); db.Source != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// databaseSource names a server's source: its manager, or the kind of
// place for one no manager keeps.
func databaseSource(source string) string {
	switch source {
	case "":
		return "—"
	case "installer":
		return locales.SourceInstaller()
	case "system":
		return locales.SourceSystem()
	}
	return source
}

func databaseClientKind(command string) string {
	switch command {
	case "psql":
		return "PostgreSQL"
	case "mysql":
		return "MySQL"
	case "mariadb":
		return "MariaDB"
	case "redis-cli":
		return "Redis"
	case "mongosh":
		return "MongoDB"
	case "sqlcmd":
		return "SQL Server"
	}
	return ""
}

func databaseDefaultPort(kind string) string {
	switch kind {
	case "PostgreSQL":
		return "5432"
	case "MySQL", "MariaDB":
		return "3306"
	case "Redis":
		return "6379"
	case "MongoDB":
		return "27017"
	case "SQL Server":
		return "1433"
	}
	return ""
}

// databasePort resolves the real observed TCP port for db when available,
// falling back to the kind's default port for local servers. Docker containers
// without a published host port return "".
func databasePort(db inventory.Database, listeners []inventory.DatabaseListener) string {
	if len(db.Ports) > 0 {
		return db.Ports[0]
	}
	if db.Source == "docker" {
		return ""
	}
	if db.Status == "stopped" {
		return databaseDefaultPort(db.Kind)
	}
	if db.Path != "" {
		clean := filepath.Clean(db.Path)
		for _, l := range listeners {
			if l.Path != "" && l.Port != "" && filepath.Clean(l.Path) == clean {
				return l.Port
			}
		}
	}
	var ports []string
	for _, l := range listeners {
		if l.Port == "" || l.Process == "" {
			continue
		}
		procKind := serverProcesses[strings.TrimSuffix(l.Process, ".exe")]
		same := procKind == db.Kind ||
			(db.Kind == "MySQL" && procKind == "MariaDB") ||
			(db.Kind == "MariaDB" && procKind == "MySQL")
		if same && !slices.Contains(ports, l.Port) {
			ports = append(ports, l.Port)
		}
	}
	if len(ports) == 1 {
		return ports[0]
	}
	return databaseDefaultPort(db.Kind)
}

func databaseClientFor(db inventory.Database, clients []inventory.Installation) (inventory.Installation, bool) {
	if db.Path != "" {
		dir := filepath.Dir(db.Path)
		for _, cl := range clients {
			if cl.Path == "" || databaseClientKind(cl.Tool) != db.Kind {
				continue
			}
			if filepath.Dir(cl.RealPath) == dir || filepath.Dir(cl.Path) == dir {
				return cl, true
			}
		}
	}
	for _, cl := range clients {
		if cl.Path != "" && databaseClientKind(cl.Tool) == db.Kind {
			return cl, true
		}
	}
	if db.Kind == "MariaDB" || db.Kind == "MySQL" {
		for _, cl := range clients {
			if cl.Path != "" && (cl.Tool == "mysql" || cl.Tool == "mariadb") {
				return cl, true
			}
		}
	}
	return inventory.Installation{}, false
}

func databaseClientCommand(db inventory.Database, client inventory.Installation, port, user string) []string {
	if client.Path == "" || port == "" {
		return nil
	}
	url, password := databaseURL(db, user, port)
	switch client.Tool {
	case "psql":
		if url == "" {
			return nil
		}
		return []string{client.Path, url}
	case "mysql", "mariadb":
		args := []string{client.Path, "-h", "127.0.0.1", "-P", port, "-u", "root"}
		if password {
			args = append(args, "-p")
		}
		return args
	case "redis-cli":
		return []string{client.Path, "-h", "127.0.0.1", "-p", port}
	case "mongosh":
		if url == "" {
			return nil
		}
		return []string{client.Path, url}
	case "sqlcmd":
		return []string{client.Path, "-S", "localhost," + port, "-U", "sa"}
	}
	return nil
}

// servicePlan is the plan that starts db's service when it is stopped
// and stops it when it runs; ok is false for a status not known, or a
// server without a service Hearth changes.
func (a *App) servicePlan(db inventory.Database) (ops.Plan, bool) {
	switch db.Status {
	case "running":
		return ops.ServicePlan(ops.ServiceStop, db, a.detect("databases"))
	case "stopped", "error":
		return ops.ServicePlan(ops.ServiceStart, db, a.detect("databases"))
	}
	return ops.Plan{}, false
}

// conflictPlan is the plan that stops the service holding is's port: the
// one running service of the kind the project asks for there, when no
// container publishes the port.
func (a *App) conflictPlan(is issues.Issue, servers []inventory.Database) (ops.Plan, bool) {
	if len(is.Dirs) == 0 {
		return ops.Plan{}, false
	}
	port := is.Dirs[0]
	if _, hasContainer := a.portContainer(port); hasContainer {
		return ops.Plan{}, false
	}
	var listeners []inventory.DatabaseListener
	if a.Snapshot != nil {
		if st, ok := a.Snapshot.Tool("databases"); ok && st.Databases != nil {
			listeners = st.Databases.Listeners
		}
	}
	kind := ""
	for _, p := range a.Projects {
		if p.Dir != is.Project {
			continue
		}
		for _, ref := range p.Databases {
			if slices.Contains(ref.Ports, port) {
				kind = ref.Kind
			}
		}
	}
	var holder []inventory.Database
	for _, db := range servers {
		if db.Source == "docker" && db.Status == "running" && slices.Contains(db.Ports, port) {
			return ops.Plan{}, false
		}
		if db.Service != "" && db.Source != "docker" && db.Status == "running" && db.Kind == kind {
			if len(listeners) > 0 && databasePort(db, listeners) != port {
				continue
			}
			holder = append(holder, db)
		}
	}
	if kind == "" || len(holder) != 1 {
		return ops.Plan{}, false
	}
	return ops.ServicePlan(ops.ServiceStop, holder[0], a.detect("databases"))
}

// conflictContainer finds a running Docker container publishing the conflicting
// port, checking database containers and all port-publishing containers.
func (a *App) conflictContainer(is issues.Issue, servers []inventory.Database) (inventory.DockerPortContainer, bool) {
	if len(is.Dirs) == 0 {
		return inventory.DockerPortContainer{}, false
	}
	port := is.Dirs[0]
	for _, db := range servers {
		if db.Source == "docker" && db.Status == "running" && slices.Contains(db.Ports, port) {
			return inventory.DockerPortContainer{
				ID:     db.ContainerID,
				Name:   db.Name,
				Image:  db.Version,
				Status: db.Status,
				Ports:  db.Ports,
			}, true
		}
	}
	return a.portContainer(port)
}

// databaseServer is the first server of kind the system or a package
// manager keeps, not a container's; false when there is none.
func (a *App) databaseServer(kind string) (inventory.Database, bool) {
	for _, db := range inventory.DatabaseServers(a.Snapshot) {
		if db.Kind == kind && db.Source != "docker" {
			return db, true
		}
	}
	return inventory.Database{}, false
}

// databaseInstallMenu is the header's button whose menu installs the
// catalog's database servers that install here: those there show their
// version, one that clashes with a server there says so, and engines
// with multiple catalog versions offer a version submenu.
func (a *App) databaseInstallMenu(c *ui.Context) {
	in := a.setupHere()
	offers := a.setupOffers(in, catalog.Database)
	if a.BuildPlan == nil || len(offers) == 0 {
		return
	}
	ui.PrimaryButton(c.Key("database-install"), locales.DatabaseInstall()).Height(controlHeight).Shrink(0).PopoverMenuButton(func(m *ui.Menu) {
		m.MaxWidth(310)
		for _, e := range offers {
			title := setupTitle(e.ID)
			db, have := a.databaseServer(catalog.DatabaseKinds[e.ID])
			conflict := a.setupConflict(e)
			var method catalog.Method
			var usable bool
			if ms := e.On(goos); len(ms) > 0 {
				if i := slices.IndexFunc(ms, in.Usable); i >= 0 {
					method, usable = ms[i], true
				}
			}
			var versions []catalog.PackageVersion
			if !have && conflict == "" && usable {
				versions = catalog.PackageVersions(e.ID, method.Manager)
			}
			var item *ui.MenuItem
			var subtitle string
			switch {
			case have:
				item = m.Item(title).ID(e.ID).Disabled(true).Trailing(locales.InstalledVersion(db.Version))
			case conflict != "":
				subtitle = locales.SetupConflicts(setupTitle(conflict))
				item = m.Item(title).ID(e.ID).Disabled(true)
			case !usable:
				item = m.Item(title).ID(e.ID).Disabled(true)
			case len(versions) > 1:
				labels := make([]string, len(versions))
				for i, v := range versions {
					labels[i] = v.Version
				}
				subtitle = strings.Join(labels, ", ") + " · " + managerTitle(method)
				item = m.Submenu(title, func(sub *ui.Menu) {
					for i, v := range versions {
						vi := sub.Item(title + " " + v.Version).ID(e.ID + ":" + v.Package)
						if i == 0 {
							vi.Trailing(locales.TagRecommended())
						}
						if vi.Chosen() {
							a.installToolPackage(e.ID, v.Package, v.Version)
						}
					}
				}).ID(e.ID)
			default:
				subtitle = locales.SetupVia(managerTitle(method))
				item = m.Item(title).ID(e.ID)
				if item.Chosen() {
					a.installTool(e.ID)
				}
			}
			iconID := e.ID
			sub := subtitle
			item.Content(func(mc ui.MenuItemContent) {
				t := mc.Context.Theme()
				pal := Colors(t)
				ui.Row(mc.Context).Gap(10).AlignItems(ui.Center).Padding(2, 0).Children(func() {
					tile := ui.Box(mc.Context).Size(26, 26).Radius(7).Center().Shrink(0).
						Background(t.Background).BorderWidth(1).BorderColor(pal.Line)
					if mc.Disabled {
						tile.Opacity(0.5)
					}
					tile.Children(func() {
						iconOrLogo(mc.Context, "brand/"+iconID, 15, t.TextMuted)
					})
					ui.Column(mc.Context).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(mc.Context, title).FontSize(13).FontWeight(500).TextColor(mc.Text).SingleLine()
						if sub != "" {
							ui.Text(mc.Context, sub).FontSize(11).TextColor(mc.Muted).SingleLine()
						}
					})
				})
			})
		}
	})
}

// autostartSwitch is the switch that turns db's service's autostart on
// and off through a plan; a service whose autostart is not known, or
// Hearth cannot change, shows none.
func (a *App) autostartSwitch(c *ui.Context, db inventory.Database) {
	if db.Autostart == "" {
		return
	}
	plan, ok := ops.AutostartPlan(db.Autostart != "on", db, a.detect("databases"))
	if !ok {
		return
	}
	on := db.Autostart == "on"
	label := locales.DatabaseAutostartFor(db.Service)
	if toggle(c, "autostart:"+db.Service, &on).Label(label).Tooltip(label).Disabled(a.op.running()).Changed() {
		a.Propose(plan)
	}
}

// databaseURL is the string a client connects to db with, at port (or its
// kind's default for local servers), as the server sets its users up:
// Homebrew's PostgreSQL lets the user in as themselves, its MySQL root,
// without a password; password reports a server that asks one. "" for a kind
// Hearth has none for, or a Docker container without a published port.
func databaseURL(db inventory.Database, user, port string) (rawURL string, password bool) {
	if port == "" {
		if db.Source == "docker" {
			return "", false
		}
		port = databaseDefaultPort(db.Kind)
	}
	host := net.JoinHostPort("localhost", port)
	switch db.Kind {
	case "PostgreSQL":
		if db.Source == "brew" && validDBUser(user) {
			u := url.URL{Scheme: "postgresql", User: url.User(user), Host: host, Path: "/postgres"}
			return u.String(), false
		}
		return "postgresql://postgres@" + host + "/postgres", true
	case "MySQL":
		return "mysql://root@" + host, db.Source != "brew"
	case "MariaDB":
		return "mysql://root@" + host, true
	case "Redis":
		return "redis://" + host, false
	case "MongoDB":
		return "mongodb://" + host, false
	case "SQL Server":
		return "sqlserver://sa@" + host, true
	}
	return "", false
}

func validDBUser(user string) bool {
	return user != "" && user != "." && user != "/" && user != string(filepath.Separator)
}
