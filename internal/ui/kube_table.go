package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

// Both workload lists use the same tracks. Only the name is flexible,
// so long names, status badges and indented pods cannot move other cells.
func workloadTracks(c *ui.Context, compact bool) []ui.Track {
	switch {
	case compact:
		return []ui.Track{ui.Fr(1), ui.Fixed(22)}
	case narrow(c):
		return []ui.Track{ui.Fr(1), ui.Fixed(152), ui.Fixed(52), ui.Fixed(86), ui.Fixed(98), ui.Fixed(50)}
	default:
		return []ui.Track{ui.Fr(1), ui.Fixed(176), ui.Fixed(68), ui.Fixed(88), ui.Fixed(126), ui.Fixed(70)}
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

// tableSort is the order a table's rows show in: by one of its columns,
// ascending or not, or by the table's own order while column is "".
type tableSort struct {
	column string
	desc   bool
}

// sortColumn is a column's header: its label, and the key a click sorts
// by; "" for a column that does not sort.
type sortColumn struct {
	label, key string
}

// next is the order after a click on the column: ascending, descending,
// then the table's own order again.
func (s tableSort) next(column string) tableSort {
	switch {
	case s.column != column:
		return tableSort{column: column}
	case !s.desc:
		return tableSort{column: column, desc: true}
	}
	return tableSort{}
}

// sortRows orders items by the sort's column, keeping the order they came
// in for those it finds equal; by holds a comparison for each column key.
func sortRows[T any](s tableSort, items []T, by map[string]func(x, y T) int) {
	compare, ok := by[s.column]
	if !ok {
		return
	}
	slices.SortStableFunc(items, func(x, y T) int {
		if s.desc {
			return compare(y, x)
		}
		return compare(x, y)
	})
}

// sortHead is a table's header whose columns sort the rows as they are
// clicked, the sorted one with an arrow; check is the select-all box,
// nil for none.
func sortHead(c *ui.Context, tracks []ui.Track, check func(), sort *tableSort, columns ...sortColumn) {
	t := c.Theme()
	pal := Colors(t)
	resourceGrid(ui.Box(c), tracks).Padding(8, 16).Background(pal.Subtle).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		if check != nil {
			check()
		}
		for _, col := range columns {
			label := strings.ToUpper(col.label)
			if col.key == "" {
				ui.Text(c, label).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				continue
			}
			on := sort.column == col.key
			b := ui.ButtonBase(c.Key("sort-" + col.key)).MinWidth(0).Gap(3).AlignItems(ui.Center).Justify(ui.Start).
				Label(locales.KubeSortBy(col.label)).OnClick(func() { *sort = sort.next(col.key) })
			color := pal.TextFaint
			if on || b.Hovered() {
				color = t.TextMuted
			}
			b.Children(func() {
				ui.Text(c, label).FontSize(10.5).TextColor(color).SingleLine().MinWidth(0)
				if on {
					icon := "chevron-up"
					if sort.desc {
						icon = "chevron-down"
					}
					ui.Icon(c, Icon(icon)).Size(11, 11).Shrink(0).TextColor(color)
				}
			})
		}
	})
}
