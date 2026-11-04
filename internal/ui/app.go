package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
	// OnScan starts a scan, whose snapshot comes back through
	// Window.Update; nil disables scanning.
	OnScan func()

	router   *ui.Router
	moreOpen bool // the languages not found show in the sidebar
	nodeTab  int  // the tab of the Node.js page shown
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
			for _, p := range pages() {
				if r.Match(p.path) {
					r.Title(p.title())
					if p.path == "/tools/node" {
						a.node(c, p)
					} else {
						a.stub(c, p)
					}
					return
				}
			}
			r.Title(overview.title())
			a.stub(c, overview)
		}).Grow(1).MinWidth(0).Background(t.Background)
	})
}

// scan starts a scan from the header or the sidebar.
func (a *App) scan() {
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
	t := c.Theme()
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(8).Padding(0, right, 0, 28).
		BorderWidth(0, 0, 1, 0).BorderColor(Colors(t).Divider).DragWindow().Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine()
			ui.Text(c, subtitle).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
		ui.Button(c, "").Disabled(a.OnScan == nil || a.Scanning).OnClick(a.scan).Children(func() {
			ui.Icon(c, Icon("refresh-cw")).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, locales.Scan()).SingleLine()
		})
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
				a.item(c, overview)
				a.item(c, projects)
				a.languages(c)
				for _, s := range sections {
					sectionTitle(c, s.title())
					for _, p := range s.pages {
						a.item(c, p)
					}
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).Margin(12, 0, 4, 0).Background(pal.Divider)
		a.item(c, settings)
		a.status(c)
	})
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
			Label(locales.Scan()).Tooltip(locales.Scan()).Disabled(a.OnScan == nil || a.Scanning).OnClick(a.scan)
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
	for _, l := range languages {
		if a.found(l.id) || within(a.router.Path(), l.path) {
			a.item(c, l.page)
		} else {
			rest = append(rest, l.page)
		}
	}
	if len(rest) == 0 {
		return
	}
	pal := Colors(c.Theme())
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
			a.item(c, p)
		}
	}
}

// found reports whether the last scan found the tool on this computer.
func (a *App) found(tool string) bool {
	if a.Snapshot == nil {
		return false
	}
	st, _ := a.Snapshot.Tool(tool)
	return len(st.Installations) > 0
}

// item is a sidebar entry going to page p; it stands out while p is shown.
func (a *App) item(c *ui.Context, p page) {
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
