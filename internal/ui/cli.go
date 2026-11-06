package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// cliPage is "CLI & clouds": command-line tools without a page of their
// own, one table.
const cliPage = "/tools/cli"

// miseTools are the tools mise installed without a page of their own.
func (a *App) miseTools() []inventory.MiseTool {
	st, _ := a.Snapshot.Tool("mise")
	return st.MiseTools
}

func (a *App) cliView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		tools := a.miseTools()
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("square-terminal")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.CliLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.CleanupFound(), strconv.Itoa(len(tools)), locales.CliFoundCaption())
					stateCard(c, t.Success, locales.CliFromMise(), strconv.Itoa(len(tools)), locales.CliNoPage())
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "cli-tabs", &a.cliTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(tools))},
						{locales.CliFromMise(), strconv.Itoa(len(tools))},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("cli-filter"), &a.cliFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Width(200)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					if len(tools) == 0 {
						toolNote(c, locales.CliNone())
						return
					}
					filter := strings.ToLower(a.cliFilter)
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						var rows []inventory.MiseTool
						for _, mt := range tools {
							if strings.Contains(strings.ToLower(mt.Name), filter) {
								rows = append(rows, mt)
							}
						}
						ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
							ui.Text(c, strings.ToUpper(locales.CliGroupMise())).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
							ui.Text(c, strconv.Itoa(len(rows))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
						})
						for _, mt := range rows {
							a.miseToolRow(c.Key("mise:"+mt.Name), mt)
						}
						if len(rows) == 0 {
							cardNote(c, locales.CleanupNone())
						}
					})
				}, func() {
					listCard(c, locales.CliHow(), "", func() {
						listRow(c, locales.CliHowMise(), false, locales.CliHowMiseCaption(), func() {})
					})
				})
			})
		})
	})
}

// miseToolRow is a tool of mise's: its name, where it stands and its other
// versions, and the version that runs, else the default, else the newest.
func (a *App) miseToolRow(c *ui.Context, mt inventory.MiseTool) {
	pal := Colors(c.Theme())
	shown := 0
	for i, in := range mt.Installations {
		if in.OnPath > 0 {
			shown = i
			break
		}
		if in.Default {
			shown = i
		}
	}
	in := mt.Installations[shown]
	parts := []string{"mise"}
	switch {
	case in.OnPath > 0:
		parts = append(parts, locales.ChipInPath())
	case in.Default:
		parts = append(parts, locales.CliDefault())
	default:
		parts = append(parts, locales.CliNotSet())
	}
	if n := len(mt.Installations) - 1; n > 0 {
		var others []string
		for i, o := range mt.Installations {
			if i != shown {
				others = append(others, o.Version)
			}
		}
		parts = append(parts, locales.CliMoreVersions(n, strings.Join(others, ", ")))
	}
	ui.Row(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, mt.Name).Font(FontMono).FontSize(13)
			clipped(c, strings.Join(parts, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
		})
		ui.Text(c, in.Version).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
	})
}
