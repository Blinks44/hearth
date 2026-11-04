package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRegisterFonts(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, f := range ui.FontFamilies() {
		if f.Registered {
			registered[f.Name] = true
		}
	}
	for _, family := range []string{FontSans, FontMono} {
		if !registered[family] {
			t.Errorf("font family %q is not registered", family)
		}
	}
}

func TestTheme(t *testing.T) {
	for _, base := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		got := Theme(base)
		if got.Accent != ui.Hex("#E5582B") {
			t.Errorf("dark=%v: accent = %v, want #E5582B", base.Dark, got.Accent)
		}
		if got.Dark != base.Dark || got.FontSize != base.FontSize {
			t.Errorf("dark=%v: theme lost the base's appearance or font size", base.Dark)
		}
		if got.Font != FontSans {
			t.Errorf("dark=%v: font = %q, want %q", base.Dark, got.Font, FontSans)
		}
		if got.Background == got.Text {
			t.Errorf("dark=%v: background and text are the same color", base.Dark)
		}
	}
	if light, dark := Theme(ui.LightTheme()), Theme(ui.DarkTheme()); light.Background == dark.Background {
		t.Error("light and dark themes share a background")
	}
}
