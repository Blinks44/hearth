package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// dialogBase fades the backdrop and its panel together. Only appearance
// animates; content changes and window resizing keep their normal layout.
func dialogBase(c *ui.Context, open *bool, content func(backdrop, panel ui.Element)) ui.Element {
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Transition(ui.ElementTransition{
			Duration:     160 * time.Millisecond,
			ExitDuration: 100 * time.Millisecond,
			Colors:       true,
			Enter:        &ui.Motion{},
			Exit:         &ui.Motion{},
		})
		content(backdrop, panel)
	})
}
