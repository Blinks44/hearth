package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Both workload lists use the same tracks. Only the name is flexible,
// so long names, status badges and indented pods cannot move other cells.
func workloadTracks(c *ui.Context, compact bool) []ui.Track {
	switch {
	case compact:
		return []ui.Track{ui.Fr(1), ui.Fixed(128)}
	case narrow(c):
		return []ui.Track{ui.Fr(1), ui.Fixed(148), ui.Fixed(52), ui.Fixed(88), ui.Fixed(100), ui.Fixed(50)}
	default:
		return []ui.Track{ui.Fr(1), ui.Fixed(170), ui.Fixed(70), ui.Fixed(90), ui.Fixed(130), ui.Fixed(70)}
	}
}

func resourceGrid(row ui.Element, tracks []ui.Track) ui.Element {
	return row.Grid().ColumnTracks(tracks...).Gap(10).AlignItems(ui.Center)
}

func resourceHead(c *ui.Context, tracks []ui.Track, headers ...string) {
	workloadHead(c, tracks, nil, headers...)
}

func workloadHead(c *ui.Context, tracks []ui.Track, check func(), headers ...string) {
	pal := Colors(c.Theme())
	resourceGrid(ui.Box(c), tracks).Padding(8, 16).Background(pal.Subtle).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		if check != nil {
			check()
		}
		for _, header := range headers {
			ui.Text(c, strings.ToUpper(header)).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
		}
	})
}

// The checkbox and row button are siblings so assistive technology can
// reach both actions.
func workloadSelectionRow(c *ui.Context, line ui.Color) ui.Element {
	return ui.Row(c).Gap(10).Padding(0, 16).AlignItems(ui.Stretch).
		BorderWidth(0, 0, 1, 0).BorderColor(line)
}

func workloadStatus(c *ui.Context, label string, bg, fg ui.Color) {
	ui.Column(c).AlignItems(ui.Start).MinWidth(0).Children(func() {
		aiTag(c, label, bg, fg).MaxWidthPercent(100).Tooltip(label)
	})
}
