package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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

func TestEveryBrandHasLogo(t *testing.T) {
	m, err := icons()
	if err != nil {
		t.Fatal(err)
	}
	for key := range m {
		if slug, ok := strings.CutPrefix(key, "brand/"); ok && m["logo/"+slug] == nil {
			t.Errorf("brand %s has no logo/%s in colors", slug, slug)
		}
	}
}

func TestLogoKeepsItsColors(t *testing.T) {
	yellow := func(r, g, b uint32) bool { return r > 0xD000 && g > 0xB000 && b < 0x8000 }
	white := func(r, g, b uint32) bool { return r > 0xE000 && g > 0xE000 && b > 0xE000 }
	black := func(r, g, b uint32) bool { return r < 0x2000 && g < 0x2000 && b < 0x2000 }
	tests := []struct {
		name  string
		slug  string
		dark  bool
		color func(r, g, b uint32) bool
	}{
		{"python in its yellow", "python", false, yellow},
		{"python in its yellow on dark", "python", true, yellow},
		{"openai black on light", "openai", false, black},
		{"openai white on dark", "openai", true, white},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := func(c *ui.Context) {
				ui.Column(c).Fill().Center().Background(ui.Hex("#808080")).Children(func() {
					iconOrLogo(c, "brand/"+tt.slug, 48, ui.Hex("#808080"))
				})
			}
			tester := ui.NewTester(view, 80, 80)
			tester.SetDark(tt.dark)
			img := tester.Image()
			n := 0
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					if tt.color(r, g, bl) {
						n++
					}
				}
			}
			if n < 20 {
				t.Errorf("drew %d pixels of the logo's color, want its own colors", n)
			}
		})
	}
}

// The overview's tool cards and agents show logos in their colors, on
// both themes.
func TestOverviewLogos(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := NewApp()
		a.Snapshot = found("node", "bun", "python", "php", "go", "rust", "docker", "pnpm", "git")
		for _, id := range []string{"claude", "codex", "copilot", "gemini", "cursor"} {
			a.Snapshot.Agents = append(a.Snapshot.Agents, inventory.Agent{ID: id, Installations: []inventory.Installation{{Tool: id, Version: "1.0.0"}}})
		}
		tt := openOverview(t, a)
		tt.SetDark(dark)
		tt.Frame()
		hasTexts(t, tt, "Python", "Docker", "Claude Code", "Gemini CLI")
		name := "overview-logos"
		if dark {
			name += "-dark"
		}
		screenshot(t, tt, name)
	}
}
