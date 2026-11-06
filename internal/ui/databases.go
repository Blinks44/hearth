package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) databases(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
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
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pal := Colors(c.Theme())
				running := 0
				for _, db := range servers {
					if db.Status == "running" {
						running++
					}
				}
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("database")).Size(24, 24).TextColor(pal.MonoText) })
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.DatabaseSubtitle()).FontSize(12.5).TextColor(c.Theme().TextMuted)
					})
					stateCard(c, pal.TextFaint, locales.DatabaseServers(), strconv.Itoa(len(servers)), strings.Join(databaseSources(servers), " · "))
					stateCard(c, c.Theme().Success, locales.DatabaseRunningCount(), strconv.Itoa(running), "")
					stateCard(c, c.Theme().Warning, locales.DatabaseConflicts(), strconv.Itoa(len(conflicts)), locales.DatabasePorts())
				})
				for _, is := range conflicts {
					title, explain := issueText(is, a.Home)
					warningRow(c, title+". "+explain)
				}
				if len(d.Warnings) > 0 {
					warningRow(c, locales.DatabaseProbeFailed(strings.Join(d.Warnings, ", ")))
				}
				if docker, ok := a.Snapshot.Tool("docker"); ok && docker.Docker != nil && docker.Docker.DatabaseErr {
					warningRow(c, locales.DatabaseContainersFailed())
				}
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					segments(c, "database-tabs", &a.databaseTab, []segment{{locales.TabAll(), strconv.Itoa(len(servers))}, {locales.DatabaseServices(), ""}, {locales.ProjectsTitle(), ""}, {locales.DatabaseContainers(), ""}})
					ui.Box(c).Grow(1)
					ui.SearchField(c.Key("database-filter"), &a.databaseFilter).Placeholder(locales.DatabaseFilter()).Width(200)
				})
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
						if a.databaseTab == 2 {
							a.databaseProjects(c)
						} else {
							a.databaseTable(c, servers, d.Clients)
						}
						toolNote(c, locales.DatabaseReadOnly())
						listCard(c, locales.DatabaseListeners(), locales.DatabaseObserved(), func() {
							shown := 0
							for _, l := range d.Listeners {
								if l.Port != "5432" && l.Port != "3306" && l.Port != "6379" && !databaseProjectPort(a, l.Port) {
									continue
								}
								shown++
								listRow(c, l.Address+":"+l.Port, true, l.Process, func() { ui.Text(c, l.PID).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint) })
							}
							if shown == 0 {
								cardNote(c, locales.DatabaseNoListeners())
							}
						})
					})
					ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
						listCard(c, locales.DatabaseClients(), "", func() {
							for _, client := range d.Clients {
								listRow(c, client.Tool+" "+client.Version, false, tilde(client.Path, a.Home), func() {})
							}
							if len(d.Clients) == 0 {
								cardNote(c, locales.DatabaseNoClients())
							}
						})
						listCard(c, locales.DatabaseServices(), "", func() {
							shown := 0
							for _, db := range servers {
								if db.Service == "" || db.Source == "docker" {
									continue
								}
								shown++
								text, _ := databaseStatus(c, db.Status)
								listRow(c, db.Service, false, text, func() {})
							}
							if shown == 0 {
								cardNote(c, locales.DatabaseNoServices())
							}
						})
						if a.databaseTab != 2 {
							a.databaseProjects(c)
						}
					})
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

func (a *App) databaseTable(c *ui.Context, servers []inventory.Database, clients []inventory.Installation) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.DatabaseServer()).Width(130).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.ColumnSource()).Width(68).FontSize(11.5).TextColor(pal.TextFaint)
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
			ui.Row(c.Key(strconv.Itoa(i))).Gap(12).Padding(12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Width(130).Gap(3).Children(func() {
					title, subtitle := db.Name+" "+db.Version, db.Service
					if db.Source == "docker" {
						title, subtitle = db.Version, db.Name
					}
					ui.Text(c, title).Font(FontMono).FontSize(12).SingleLine()
					ui.Text(c, subtitle).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				})
				ui.Text(c, databaseSource(db.Source)).Width(68).FontSize(10.5).TextColor(pal.MonoText)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					address := tilde(db.Path, a.Home)
					if db.Source == "docker" {
						address = strings.Join(db.Ports, ", ")
					}
					if address == "" {
						address = "—"
					}
					ui.Text(c, address).Font(FontMono).FontSize(10.5).SingleLine().TextColor(pal.MonoText)
					text, dot := databaseStatus(c, db.Status)
					bg, ink := pal.Muted, pal.MonoText
					if db.Status == "running" {
						bg, ink = pal.SuccessSoft, pal.SuccessText
					}
					ui.Row(c).Children(func() { chip(c, dot, bg, ink, text) })
					for _, client := range clients {
						if db.Path == "" || filepath.Dir(client.RealPath) != filepath.Dir(db.Path) && filepath.Dir(client.Path) != filepath.Dir(db.Path) || databaseClientKind(client.Tool) != db.Kind {
							continue
						}
						ui.Text(c, locales.DatabaseClient(client.Tool+" "+client.Version)).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				if db.Path != "" {
					iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(db.Path); c.Toast(locales.PathCopied()) })
					if a.Shell != nil {
						iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(db.Path) })
					}
				}
			})
		}
		if shown == 0 {
			cardNote(c, locales.DatabaseNone())
		}
	})
}

func (a *App) databaseProjects(c *ui.Context) {
	listCard(c, locales.DatabaseProjects(), "compose · .env", func() {
		shown := 0
		for _, p := range a.Projects {
			for _, ref := range p.Databases {
				shown++
				listRow(c, filepath.Base(p.Dir), false, ref.File+" · "+ref.Kind+" · "+strings.Join(ref.Ports, ", "), func() { ui.Button(c, locales.Details()).OnClick(func() { a.router.Push(projectPath(p.Dir)) }) })
			}
		}
		if shown == 0 {
			cardNote(c, locales.DatabaseNoProjects())
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
	}
	return ""
}
