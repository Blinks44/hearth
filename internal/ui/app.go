package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// App is the state of Hearth's main window.
type App struct {
	// Machine is the caption under the logo, as "studio · macOS 15.4".
	Machine string
	// Home is the user's home directory, which paths show as ~.
	Home string
	// Snapshot is what the last scan found; nil before the first.
	Snapshot *inventory.Snapshot
	// Scanning reports a scan under way.
	Scanning bool
	// NodeVersions is what nodejs.org says of Node's versions; nil until
	// the list comes, or when it could not, NodeVersionsErr says why.
	NodeVersions    *versions.Node
	NodeVersionsErr string
	// BunLatest is Bun's newest stable version, as the npm registry says;
	// nil until it comes, or when it could not, BunLatestErr says why.
	BunLatest    *versions.Latest
	BunLatestErr string
	// GoVersions are Go's supported releases, as go.dev says; nil until
	// they come, or when they could not, GoVersionsErr says why.
	GoVersions    *versions.Go
	GoVersionsErr string
	// PythonVersions are Python's release lines; nil until they come, or
	// when they could not, PythonVersionsErr says why.
	PythonVersions    *versions.Python
	PythonVersionsErr string
	// Shell reveals and opens files; nil hides those actions.
	Shell Shell
	// OnScan starts a scan, whose snapshot comes back through
	// Window.Update; nil disables scanning.
	OnScan func()
	// Projects are the folders the user added, as last read.
	Projects []projects.Project
	// ChooseFolder asks for a folder to add, and shows it with Offer
	// through Window.Update; nil hides adding.
	ChooseFolder func()
	// AddProjects adds the folders to the projects, and RemoveProject
	// takes one off the list, leaving the folder be.
	AddProjects   func(dirs []string)
	RemoveProject func(dir string)
	// LoadSkill reads a skill's folder, its files and SKILL.md, in the
	// background, and gives them to done through Window.Update; nil shows
	// neither.
	LoadSkill func(dir string, done func(entries []inventory.Entry, skillMD string, err error))

	router        *ui.Router
	moreOpen      bool    // the languages not found show in the sidebar
	nodeTab       int     // the tab of the Node.js page shown
	bunTab        int     // the tab of the Bun page shown
	goTab         int     // the tab of the Go page shown
	pythonTab     int     // the tab of the Python page shown
	projectsTab   int     // all projects, or the ones needing attention
	projectFilter string  // the projects page's filter
	adding        *adding // the folder the add dialog shows; nil when closed
	addOpen       bool

	// The skills and MCP pages: the agent filtered by, "" for all; the
	// scope, as scopeSegments; the search; and the key of the one shown.
	skillAgent, mcpAgent   string
	skillScope, mcpScope   int
	skillQuery, mcpQuery   string
	skillChosen, mcpChosen string
	skillFolder            skillFolder // what was read of the skill shown
	markdown               markdown.Options
}

// NewApp returns the app showing the overview.
func NewApp() *App {
	return &App{router: ui.NewRouter(overview.path)}
}

// View builds the window: the sidebar and the page shown beside it.
func (a *App) View(c *ui.Context) {
	c.SetTheme(Theme(c.Theme()))
	t := c.Theme()
	// Vibrancy shows wherever the view draws no background: under the
	// sidebar, which then draws none.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, vibrant)
		ui.Box(c).Width(1).Shrink(0).Background(Colors(t).Divider)
		a.router.View(c, func(r *ui.Route) {
			if r.Match(agentPage) {
				id := r.Query("id")
				r.Title(agentTitle(id))
				a.agentDetail(c, id)
				return
			}
			if r.Match(projectPage) {
				dir := r.Query("dir")
				r.Title(filepath.Base(dir))
				a.project(c, dir)
				return
			}
			for _, p := range pages() {
				if r.Match(p.path) {
					r.Title(p.title())
					switch p.path {
					case "/tools/node":
						a.node(c, p)
					case "/tools/bun":
						a.bun(c, p)
					case "/tools/go":
						a.goPage(c, p)
					case "/tools/python":
						a.python(c, p)
					case projectsPage.path:
						a.projectList(c, p)
					case overview.path:
						a.overviewPage(c, p)
					case "/ai/agents":
						a.agentsPage(c, p)
					case skillsPage:
						a.skillsPage(c, p)
					case mcpPage:
						a.mcpPageView(c, p)
					default:
						a.stub(c, p)
					}
					return
				}
			}
			r.Title(overview.title())
			a.overviewPage(c, overview)
		}).Grow(1).MinWidth(0).Background(t.Background)
	})
}

// Scan starts a scan, as the header and the sidebar do, unless one is
// under way. It runs on the UI thread.
func (a *App) Scan() {
	if a.OnScan != nil && !a.Scanning {
		a.Scanning = true
		a.OnScan()
	}
}

// stub is a page still to be built: its header and "Coming soon".
func (a *App) stub(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Icon(c, mustIcon(p.icon)).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
			ui.Text(c, locales.ComingSoon()).TextColor(c.Theme().TextMuted)
		})
	})
}

// header is the top of a page: its title and subtitle, and on the right
// its actions. It drags the window, and keeps clear of the window
// controls on Windows and Linux.
func (a *App) header(c *ui.Context, title, subtitle string) {
	a.headerWith(c, title, subtitle, headerExtras{})
}

// headerExtras are what a page adds to its header: a button going back
// before the title, and actions before and after Scan.
type headerExtras struct {
	back, before, after func()
}

// headerWith is header with the page's extras.
func (a *App) headerWith(c *ui.Context, title, subtitle string, x headerExtras) {
	t := c.Theme()
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(8).Padding(0, right, 0, 28).
		BorderWidth(0, 0, 1, 0).BorderColor(Colors(t).Divider).DragWindow().Children(func() {
		if x.back != nil {
			ui.Button(c.Key("back"), "").Label(locales.BackToProjects()).Tooltip(locales.BackToProjects()).
				Size(28, 26).Padding(0).Margin(0, 4, 0, 0).OnClick(x.back).Children(func() {
				ui.Icon(c, Icon("chevron-left")).Size(14, 14).TextColor(t.TextMuted)
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine()
			ui.Text(c, subtitle).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
		if x.before != nil {
			x.before()
		}
		ui.Button(c, "").Disabled(a.OnScan == nil || a.Scanning).OnClick(a.Scan).Children(func() {
			ui.Icon(c, Icon("refresh-cw")).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, locales.Scan()).SingleLine()
		})
		if x.after != nil {
			x.after()
		}
	})
}

// sidebar is the navigation: the logo, the pages by section, and settings
// with the scan status at the bottom.
func (a *App) sidebar(c *ui.Context, vibrant bool) {
	t := c.Theme()
	pal := Colors(t)
	side := ui.Column(c).Width(244).Shrink(0).Padding(0, 10, 12, 10)
	if !vibrant {
		side.Background(pal.Sidebar)
	}
	side.Children(func() {
		// macOS puts the traffic lights in a row of their own above the
		// logo; Windows and Linux put their buttons over the header.
		if bar := c.TitleBar(); bar.Left > 0 {
			ui.Box(c).Height(52).Shrink(0).DragWindow()
		} else {
			ui.Box(c).Height(14).Shrink(0).DragWindow()
		}
		brand(c, a.Machine)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				a.item(c, overview, nil)
				a.item(c, projectsPage, func() {
					if len(a.Projects) > 0 {
						ui.Text(c, strconv.Itoa(len(a.Projects))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					}
				})
				a.languages(c)
				for _, s := range sections {
					sectionTitle(c, s.title())
					for _, p := range s.pages {
						a.item(c, p, a.pageEnd(c, p))
					}
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).Margin(12, 0, 4, 0).Background(pal.Divider)
		a.item(c, settings, nil)
		a.status(c)
	})
}

// pageEnd is what an item shows on its right: how many agents, skills or
// MCP servers were found on the AI pages', and the PATH's issues on its.
func (a *App) pageEnd(c *ui.Context, p page) func() {
	n := 0
	switch p.path {
	case "/ai/agents":
		n = len(a.foundAgents())
	case skillsPage:
		n = len(a.allSkills())
	case mcpPage:
		n = len(a.mcpItems())
	case pathPage:
		return a.issueCount(c)
	}
	if n == 0 {
		return nil
	}
	return func() {
		ui.Text(c, strconv.Itoa(n)).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	}
}

// issueCount shows how many issues the PATH has; nil for none.
func (a *App) issueCount(c *ui.Context) func() {
	n := 0
	for _, is := range a.issues(c) {
		if issuePage(is) == pathPage {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return func() {
		pal := Colors(c.Theme())
		ui.Text(c, strconv.Itoa(n)).FontSize(10.5).FontWeight(500).TextColor(pal.WarningText).
			Padding(1, 6).Radius(8).Background(pal.WarningSoft).SingleLine()
	}
}

// status tells when the computer was last checked, with a button that
// checks it again.
func (a *App) status(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	dot, text := pal.TextFaint, locales.NotChecked()
	switch {
	case a.Scanning:
		dot, text = pal.Info, locales.Checking()
	case a.Snapshot != nil:
		dot, text = t.Success, checked(c, a.Snapshot.Taken)
	}
	ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(2, 2, 0, 9).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, text).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).SingleLine()
		again := ui.ButtonBase(c.Key("scan-again")).Size(24, 24).Radius(6).Center().
			Label(locales.Scan()).Tooltip(locales.Scan()).Disabled(a.OnScan == nil || a.Scanning).OnClick(a.Scan)
		if again.Hovered() {
			again.Background(pal.SidebarHover)
		}
		again.Children(func() {
			ui.Icon(c, Icon("refresh-cw")).Size(12, 12).TextColor(pal.TextFaint)
		})
	})
}

// checked says how long ago taken was, and asks for a frame when the text
// would change.
func checked(c *ui.Context, taken time.Time) string {
	ago := c.Now().Sub(taken)
	switch {
	case ago < time.Minute:
		c.After(time.Minute - ago)
		return locales.CheckedJustNow()
	case ago < time.Hour:
		c.After(time.Minute - ago%time.Minute)
		return locales.CheckedMinutesAgo(int(ago / time.Minute))
	case ago < 24*time.Hour:
		c.After(time.Hour - ago%time.Hour)
		return locales.CheckedHoursAgo(int(ago / time.Hour))
	}
	return locales.CheckedOn(taken)
}

// languages is the "Languages & runtimes" section: the languages found,
// the page shown if it is another one, and the rest under "N more
// languages".
func (a *App) languages(c *ui.Context) {
	sectionTitle(c, locales.SectionLanguages())
	var rest []page
	pal := Colors(c.Theme())
	plus := func() {
		ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(pal.TextFaint).Tooltip(locales.NotInstalledHint())
	}
	for _, l := range languages {
		switch v := a.version(l.id); {
		case v != "":
			a.item(c, l.page, func() {
				ui.Text(c, v).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			})
		case within(a.router.Path(), l.path):
			a.item(c, l.page, plus)
		default:
			rest = append(rest, l.page)
		}
	}
	if len(rest) == 0 {
		return
	}
	more := ui.ButtonBase(c.Key("more-languages")).Height(30).Shrink(0).Gap(9).Padding(0, 8).Radius(7).AlignItems(ui.Center)
	if more.Hovered() {
		more.Background(pal.SidebarHover)
	}
	more.Children(func() {
		ui.Icon(c, Icon("layers")).Size(16, 16).TextColor(pal.TextFaint)
		ui.Text(c, locales.MoreLanguages(len(rest))).FontSize(13).TextColor(pal.TextFaint).Grow(1).SingleLine()
		ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(pal.TextFaint)
	})
	if more.Clicked() {
		a.moreOpen = !a.moreOpen
	}
	if a.moreOpen {
		for _, p := range rest {
			a.item(c, p, plus)
		}
	}
}

// version is the sidebar's version of a tool the last scan found, as
// 22.12: the one the terminal runs, else the first known; "…" when its
// version is unknown, and "" when the scan did not find it.
func (a *App) version(tool string) string {
	if a.Snapshot == nil {
		return ""
	}
	st, _ := a.Snapshot.Tool(tool)
	if len(st.Installations) == 0 {
		return ""
	}
	v := ""
	if i := inventory.TerminalCopy(st.Installations); i >= 0 {
		v = st.Installations[i].Version
	}
	for _, in := range st.Installations {
		if v == "" {
			v = in.Version
		}
	}
	if v == "" {
		return "…"
	}
	if major, rest, ok := strings.Cut(v, "."); ok {
		minor, _, _ := strings.Cut(rest, ".")
		return major + "." + minor
	}
	return v
}

// item is a sidebar entry going to page p; it stands out while p is
// shown. end, if not nil, builds what shows on its right.
func (a *App) item(c *ui.Context, p page, end func()) {
	t := c.Theme()
	pal := Colors(t)
	chosen := within(a.router.Path(), p.path)
	e := ui.ButtonBase(c.Key(p.path)).Height(30).Shrink(0).Gap(9).Padding(0, 8).Radius(7).AlignItems(ui.Center)
	iconColor := t.TextMuted
	switch {
	case chosen:
		e.Background(pal.SidebarChosen).InsetShadow(0, 0, 0, 1, ui.RGBA(0, 0, 0, 0.05)).Shadow(0, 1, 2, 0, pal.Shadow)
		iconColor = t.Accent
	case e.Hovered():
		e.Background(pal.SidebarHover)
	}
	e.Children(func() {
		ui.Icon(c, mustIcon(p.icon)).Size(16, 16).TextColor(iconColor)
		ui.Text(c, p.title()).FontSize(13).Grow(1).SingleLine()
		if end != nil {
			end()
		}
	})
	if e.Clicked() && !chosen {
		a.router.Push(p.path)
	}
}

func sectionTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(11).FontWeight(500).TextColor(Colors(c.Theme()).TextFaint).
		Padding(12, 8, 5, 8).SingleLine()
}

// brand is the logo, the app's name and the machine it looks at.
func brand(c *ui.Context, machine string) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(2, 8, 14, 8).Shrink(0).Children(func() {
		ui.Box(c).Size(28, 28).Radius(8).Center().Background(Accent).
			Shadow(0, 2, 6, 0, ui.Hex("#C2410C").Alpha(0.25)).Children(func() {
			ui.Icon(c, Icon("flame")).Size(16, 16).TextColor(ui.Hex("#FFFFFF"))
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, "Hearth").FontSize(14).FontWeight(600).SingleLine()
			if machine != "" {
				ui.Text(c, machine).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
	})
}
