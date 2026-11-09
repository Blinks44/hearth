package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDialogAppearance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reduce bool
	}{
		{name: "normal"},
		{name: "reduced motion", reduce: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := searchApp()
			tt := ui.NewTesterWith(a.View, 1280, 820, ui.TesterOptions{
				Preferences: ui.Preferences{ReduceMotion: tc.reduce},
			})
			pixel := func() uint8 { return tt.Image().RGBAAt(1260, 800).R }
			clear := pixel()
			for range 2 {
				tt.Key(ui.Cmd, ui.KeyK)
				tt.Type("node")
				if a.search.query != "node" {
					t.Fatal("search did not receive input while appearing")
				}
				tt.Advance(80 * time.Millisecond)
				mid := pixel()
				screenshot(t, tt, "dialog-mid")
				tt.Advance(120 * time.Millisecond)
				shown := pixel()
				screenshot(t, tt, "dialog-shown")
				if clear <= mid || mid <= shown {
					t.Fatalf("expected gradual dimming: %d → %d → %d", clear, mid, shown)
				}
				tt.Key(ui.Cmd, ui.KeyK)
				tt.Advance(50 * time.Millisecond)
				mid = pixel()
				tt.Advance(100 * time.Millisecond)
				if shown >= mid || mid >= pixel() || pixel() != clear {
					t.Fatalf("expected gradual dismissal: %d → %d → %d", shown, mid, pixel())
				}
			}
		})
	}
}
