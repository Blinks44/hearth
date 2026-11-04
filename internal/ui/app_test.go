package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Every sidebar item opens its page, whose header shows its title; the
// languages not found wait under "N more languages".
func TestSidebarOpensPages(t *testing.T) {
	a := NewApp()
	a.Languages = []string{"node", "bun", "python", "php"}
	// Tall enough for every item without scrolling the sidebar.
	tt := ui.NewTester(a.View, 1280, 1100)
	if tt.HasText("Rust") {
		t.Error("a language not found shows before opening the rest")
	}
	if err := tt.Click("6 more languages"); err != nil {
		t.Fatal(err)
	}
	for _, p := range pages() {
		if err := tt.Click(p.title); err != nil {
			t.Fatalf("%s: %v", p.title, err)
		}
		tt.Advance(time.Second) // the page before fades out
		if a.router.Path() != p.path || a.router.Title() != p.title {
			t.Errorf("clicked %q: page %q titled %q", p.title, a.router.Path(), a.router.Title())
		}
		if !tt.HasText(p.subtitle) || !tt.HasText("Coming soon") {
			t.Errorf("%s: header missing, texts %q", p.title, tt.Texts())
		}
	}
}

// A language page not found on this computer still shows in the sidebar
// while it is open.
func TestSidebarKeepsOpenLanguage(t *testing.T) {
	a := NewApp()
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("10 more languages") {
		t.Fatalf("texts %q", tt.Texts())
	}
	a.router.Push("/tools/rust")
	tt.Advance(time.Second)
	if !tt.HasText("Rust") || !tt.HasText("9 more languages") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// Without vibrancy the sidebar paints its own background; with it, it
// paints none so the material shows.
func TestSidebarVibrancy(t *testing.T) {
	a := NewApp()
	tt := ui.NewTester(a.View, 1280, 820)
	alpha := func() uint8 {
		img := tt.Image()
		return img.Pix[img.PixOffset(100, 800)+3]
	}
	if alpha() != 255 {
		t.Errorf("without vibrancy the sidebar's alpha is %d", alpha())
	}
	tt.SetVibrancy(true)
	if alpha() != 0 {
		t.Errorf("over the material the sidebar's alpha is %d", alpha())
	}
}
