// Package ui builds Hearth's interface on top of MyGo's widgets.
package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/resources"
)

// Font families registered by RegisterFonts.
const (
	FontSans = "Inter"
	FontMono = "JetBrains Mono"
)

// Accent is Hearth's brand color, the same in both themes.
var Accent = ui.Hex("#E5582B")

// RegisterFonts adds the embedded Inter and JetBrains Mono. Call it once,
// before the first window opens.
func RegisterFonts() error {
	for file, family := range map[string]string{
		"fonts/Inter.ttf":         FontSans,
		"fonts/JetBrainsMono.ttf": FontMono,
	} {
		data, err := resources.Fonts.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read font %s: %w", file, err)
		}
		if err := ui.RegisterFont(data, family); err != nil {
			return fmt.Errorf("register font %s: %w", family, err)
		}
	}
	return nil
}

// Theme returns Hearth's theme on top of base, the window's default theme,
// so that the system's font size and appearance still apply. Colors come
// from the token table in docs/design.md.
func Theme(base *ui.Theme) *ui.Theme {
	t := *base
	if t.Dark {
		t.Background = ui.Hex("#1C1A18")
		t.Surface = ui.Hex("#2C2926")
		t.SurfaceHover = ui.Hex("#34312C")
		t.SurfacePressed = ui.Hex("#47433D")
		t.Border = ui.Hex("#47433D")
		t.Text = ui.Hex("#F2EFEA")
		t.TextMuted = ui.Hex("#A8A299")
		t.Danger = ui.Hex("#EF5B4F")
		t.Warning = ui.Hex("#E0A23A")
		t.Success = ui.Hex("#3DBA72")
		t.AccentHover = Accent.Mix(ui.Hex("#FFFFFF"), 0.12)
		t.AccentPressed = Accent.Mix(ui.Hex("#000000"), 0.12)
	} else {
		t.Background = ui.Hex("#FFFFFF")
		t.Surface = ui.Hex("#FFFFFF")
		t.SurfaceHover = ui.Hex("#FAF9F7")
		t.SurfacePressed = ui.Hex("#F1EFEB")
		t.Border = ui.Hex("#D9D5CE")
		t.Text = ui.Hex("#1D1B18")
		t.TextMuted = ui.Hex("#69655E")
		t.Danger = ui.Hex("#D2453A")
		t.Warning = ui.Hex("#C98208")
		t.Success = ui.Hex("#2E9A5C")
		t.AccentHover = Accent.Mix(ui.Hex("#000000"), 0.08)
		t.AccentPressed = Accent.Mix(ui.Hex("#000000"), 0.16)
	}
	t.Accent = Accent
	t.AccentText = ui.Hex("#FFFFFF")
	t.Selection = Accent.Alpha(0.25)
	t.Focus = Accent.Alpha(0.55)
	t.Radius = 6
	t.Font = FontSans
	return &t
}
