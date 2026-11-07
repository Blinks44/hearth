package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// modelsPage is the local AI models' page.
const modelsPage = "/ai/models"

// modelApp is Ollama or LM Studio by name, nil when not found.
type modelApp struct {
	name string
	app  *inventory.ModelApp
}

// modelApps are the apps found, Ollama first.
func (a *App) modelApps() []modelApp {
	st, _ := a.Snapshot.Tool("models")
	if st.Models == nil {
		return nil
	}
	var out []modelApp
	for _, m := range []modelApp{{"Ollama", st.Models.Ollama}, {"LM Studio", st.Models.LMStudio}} {
		if m.app != nil {
			out = append(out, m)
		}
	}
	return out
}

// modelCount is how many models the apps found downloaded.
func (a *App) modelCount() int {
	n := 0
	for _, m := range a.modelApps() {
		n += len(m.app.Models)
	}
	return n
}

// mcpServers are the MCP servers of the user's and the projects' files.
func (a *App) mcpServers() []inventory.MCPServer {
	if a.Snapshot == nil {
		return nil
	}
	servers := slices.Clone(a.Snapshot.MCP)
	for _, p := range a.Projects {
		servers = append(servers, p.MCP...)
	}
	return servers
}

// listening reports that the last scan saw a TCP listener on port.
func (a *App) listening(port string) bool {
	st, _ := a.Snapshot.Tool("databases")
	return st.Databases != nil && slices.ContainsFunc(st.Databases.Listeners, func(l inventory.DatabaseListener) bool { return l.Port == port })
}

func (a *App) modelsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		apps := a.modelApps()
		servers := a.mcpServers()
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.modelsHeader(c, apps)
				segs := []segment{{locales.TabAll(), strconv.Itoa(a.modelCount())}}
				for _, m := range apps {
					segs = append(segs, segment{m.name, strconv.Itoa(len(m.app.Models))})
				}
				a.modelsTab = min(a.modelsTab, len(segs)-1)
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "models-tabs", &a.modelsTab, segs)
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("models-filter"), &a.modelsFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Width(200)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					if len(apps) == 0 {
						toolNote(c, locales.ModelsNone())
						return
					}
					filter := strings.ToLower(a.modelsFilter)
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						for i, m := range apps {
							if a.modelsTab > 0 && a.modelsTab-1 != i {
								continue
							}
							a.modelGroup(c, m, filter, servers)
						}
					})
				}, func() {
					a.modelServers(c, apps)
					a.modelLinks(c, apps, servers)
				})
			})
		})
	})
}

// modelsHeader is the page's tile and lead, and the cards: how many
// models, the space they take and whether Ollama runs.
func (a *App) modelsHeader(c *ui.Context, apps []modelApp) {
	t := c.Theme()
	pal := Colors(t)
	headRow(c, func() {
		ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
			ui.Icon(c, Icon("brain")).Size(24, 24).TextColor(t.TextMuted)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, locales.ModelsTitle()).FontSize(22).FontWeight(600)
			ui.Text(c, locales.ModelsLead()).FontSize(12).TextColor(t.TextMuted)
		})
	}, func() {
		var size int64
		for _, m := range apps {
			for _, md := range m.app.Models {
				size += md.Size
			}
		}
		stateCard(c, pal.TextFaint, locales.ModelsCount(), strconv.Itoa(a.modelCount()), locales.ModelsInApps(len(apps)))
		stateCard(c, t.Warning, locales.ModelsTake(), formatBytes(size), locales.ModelsOnDisk())
		var ollama *inventory.ModelApp
		if len(apps) > 0 && apps[0].name == "Ollama" {
			ollama = apps[0].app
		}
		switch {
		case ollama == nil:
			stateCard(c, pal.TextFaint, "Ollama", locales.ModelsNotFound(), "")
		case a.listening(ollama.Port):
			stateCard(c, t.Success, "Ollama", locales.ModelsRunning(), "localhost:"+ollama.Port)
		default:
			stateCard(c, pal.TextFaint, "Ollama", locales.ModelsStopped(), "localhost:"+ollama.Port)
		}
	})
}

// modelGroup is an app's models under a row of its name, version and
// source, and the space they take.
func (a *App) modelGroup(c *ui.Context, m modelApp, filter string, servers []inventory.MCPServer) {
	pal := Colors(c.Theme())
	var total int64
	for _, md := range m.app.Models {
		total += md.Size
	}
	head := strings.TrimSpace(m.name + " " + m.app.Version)
	if m.app.Source.Kind != inventory.KindUnknown {
		head += " · " + sourceName(m.app.Source)
	}
	ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(head)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
		ui.Text(c, formatBytes(total)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
	})
	if m.app.Err != "" {
		cardNote(c, locales.ModelsReadFailed(m.app.Err))
	}
	shown := 0
	for _, md := range m.app.Models {
		if !strings.Contains(strings.ToLower(md.Name), filter) {
			continue
		}
		shown++
		var parts []string
		if md.Loaded {
			parts = append(parts, locales.ModelsLoaded())
		} else if !md.Downloaded.IsZero() {
			parts = append(parts, locales.ModelsDownloaded(ago(c, md.Downloaded)))
		}
		switch {
		case md.Quant != "":
			parts = append(parts, md.Quant)
		case md.Format != "":
			parts = append(parts, md.Format)
		}
		for _, s := range inventory.ModelMentions(servers, md) {
			parts = append(parts, locales.ModelsAskedBy(s.Name, agentTitle(s.Agent)))
		}
		ui.Row(c.Key(m.name+":"+md.Name)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				clipped(c, md.Name).FontSize(13)
				clipped(c, strings.Join(parts, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
			})
			ui.Text(c, formatBytes(md.Size)).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
		})
	}
	if shown == 0 && m.app.Err == "" {
		cardNote(c, locales.ModelsNoModels())
	}
}

// modelServers are the apps' servers, whether the last scan saw them
// listen, and the folder Ollama keeps its models in.
func (a *App) modelServers(c *ui.Context, apps []modelApp) {
	listCard(c, locales.ModelsServers(), "", func() {
		for _, m := range apps {
			var caption []string
			if m.app.Source.Kind != inventory.KindUnknown {
				caption = append(caption, sourceName(m.app.Source))
			}
			switch {
			case a.listening(m.app.Port):
				caption = append(caption, locales.ModelsRunning())
			case m.name == "Ollama":
				caption = append(caption, locales.ModelsStopped())
			default:
				caption = append(caption, locales.ModelsServerOff())
			}
			listRow(c, m.name+" · localhost:"+m.app.Port, false, strings.Join(caption, " · "), func() {})
		}
		for _, m := range apps {
			if m.name != "Ollama" {
				continue
			}
			caption := locales.ModelsNotSet(tilde(m.app.Dir, a.Home))
			if m.app.DirVar != "" {
				caption = tilde(m.app.Dir, a.Home)
			}
			listRow(c, "OLLAMA_MODELS", true, caption, func() {})
		}
		if len(apps) == 0 {
			cardNote(c, locales.ModelsNone())
		}
	})
}

// modelLinks are the MCP servers that ask for a model, and the way to
// Cleanup for the rest.
func (a *App) modelLinks(c *ui.Context, apps []modelApp, servers []inventory.MCPServer) {
	t := c.Theme()
	listCard(c, locales.ModelsLinks(), "", func() {
		for _, m := range apps {
			for _, md := range m.app.Models {
				for _, s := range inventory.ModelMentions(servers, md) {
					listRow(c, "MCP "+s.Name+" · "+agentTitle(s.Agent), false, locales.ModelsAsks(md.Name), func() {})
				}
			}
		}
		listRow(c, locales.ModelsRest(), false, locales.ModelsRestCaption(), func() {})
		b := ui.ButtonBase(c.Key("models-cleanup")).Column().AlignItems(ui.Start).Gap(1).Padding(9, 12, 9, 14).BorderWidth(1, 0, 0, 0).BorderColor(Colors(t).Line).
			Label(locales.ModelsOpenCleanup()).OnClick(func() {
			a.cleanupTab = slices.Index(cleanupGroups, cleanup.Models) + 1
			a.router.Push(cleanupPage)
		})
		if b.Hovered() {
			b.Background(Colors(t).Subtle)
		}
		b.Children(func() {
			ui.Text(c, locales.ModelsOpenCleanup()).FontSize(13).TextColor(Accent).SingleLine()
			ui.Text(c, locales.ModelsOpenCleanupCaption()).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
	})
}
