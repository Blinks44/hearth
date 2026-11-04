package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestIconsParse(t *testing.T) {
	m, err := icons()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lucide/search", "lucide/folder-git-2", "brand/nodedotjs", "brand/cursor"} {
		if m[key] == nil {
			t.Errorf("icon %s is missing", key)
		}
	}
	if len(m) < 50 {
		t.Errorf("parsed %d icons, want all of resources/icons", len(m))
	}
}

func TestIconTakesTextColor(t *testing.T) {
	red := ui.Hex("#FF0000")
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Icon(c, Icon("database")).FontSize(48).TextColor(red)
		})
	}
	img := ui.NewTester(view, 80, 80).Image()
	reddish := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0xC000 && g < 0x4000 && bl < 0x4000 {
				reddish++
			}
		}
	}
	if reddish < 20 {
		t.Errorf("icon drew %d red pixels, want it in the text color", reddish)
	}
}

func TestUnknownIconPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Icon of an unknown name did not panic")
		}
	}()
	Icon("no-such-icon")
}
