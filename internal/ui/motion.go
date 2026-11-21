package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// colorFade eases the colors and the shadow an element changes, as a card
// the pointer comes to and goes from, or a step's status tag.
var colorFade = ui.ElementTransition{Colors: true, Shadows: true, Duration: 140 * time.Millisecond}

// stateCardMotion smoothly shifts a header state card between active/counting
// tint and its resting neutral state.
var stateCardMotion = ui.ElementTransition{Colors: true, Duration: 260 * time.Millisecond}

// cardEnter brings a card in, rising a little, where the scan adds it to a
// page already shown, and eases its hover as colorFade does.
var cardEnter = ui.ElementTransition{
	Colors:   true,
	Shadows:  true,
	Duration: 200 * time.Millisecond,
	Enter:    &ui.Motion{Y: 8},
}

// skeletonExit fades a skeleton out as the card it stood for comes in.
var skeletonExit = ui.ElementTransition{Colors: true, Duration: 200 * time.Millisecond, Exit: &ui.Motion{}}

// rowMotion grows rows in, collapses those that go and slides the others
// to their places, as a filter or a removal changes a table.
var rowMotion = ui.ElementTransition{
	Enter: &ui.Motion{Collapse: true},
	Exit:  &ui.Motion{Collapse: true},
}

// stepShift is how far a wizard's step comes in from the side it lies on:
// from the right going forward, from the left going back. The step it
// leaves goes the other way, which its parent's ChildExit says: the step
// was built before the user chose the way.
func stepShift(back bool) float32 {
	if back {
		return -24
	}
	return 24
}

// stepMotion brings a wizard's step in by stepShift once the step it
// replaces is gone: the two crossing would mix their lines.
func stepMotion(back bool) ui.ElementTransition {
	dx := stepShift(back)
	return ui.ElementTransition{
		Colors:       true,
		Duration:     220 * time.Millisecond,
		ExitDuration: 120 * time.Millisecond,
		Enter:        &ui.Motion{X: dx},
		Exit:         &ui.Motion{X: -dx},
		AfterExits:   true,
	}
}

// detailEnter brings a resource's details in beside its list, from a
// little to the right.
var detailEnter = ui.ElementTransition{
	Duration: 180 * time.Millisecond,
	Enter:    &ui.Motion{X: 12},
}

// bulkBarEnter lifts the floating bulk action bar smoothly into view from
// below the list when items are selected.
var bulkBarEnter = ui.ElementTransition{
	Duration: 180 * time.Millisecond,
	Enter:    &ui.Motion{Y: 10},
}

// barMotion moves a progress bar's fill to its new share and color.
var barMotion = ui.ElementTransition{Width: true, Colors: true, Animation: &ui.Smooth}

// sidebarMotion follows the rows moved by the expanding language group.
var sidebarMotion = ui.ElementTransition{Position: true, Colors: true, Shadows: true, Duration: 200 * time.Millisecond}

// spinIcon is an icon that turns while spinning is true, as the refresh
// icon of something being read or checked; still otherwise.
func spinIcon(c *ui.Context, name string, size float32, color ui.Color, spinning bool) {
	box := ui.Box(c).Size(size, size).Shrink(0).Center()
	if spinning {
		const turn = 900 * time.Millisecond
		box.Rotate(float32(c.Now().UnixNano()%int64(turn)) / float32(turn) * 360)
		c.Invalidate()
	}
	box.Children(func() { ui.Icon(c, Icon(name)).Size(size, size).TextColor(color) })
}
