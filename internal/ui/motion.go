package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// colorFade eases the colors and the shadow an element changes, as a card
// the pointer comes to and goes from, or a step's status tag.
var colorFade = ui.ElementTransition{Colors: true, Shadows: true, Duration: 140 * time.Millisecond}

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

// barMotion moves a progress bar's fill to its new share and color.
var barMotion = ui.ElementTransition{Width: true, Colors: true, Animation: &ui.Smooth}

// sidebarMotion follows the rows moved by the expanding language group.
var sidebarMotion = ui.ElementTransition{Position: true, Colors: true, Shadows: true, Duration: 200 * time.Millisecond}
