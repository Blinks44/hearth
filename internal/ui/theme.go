// Package ui builds Hearth's interface on top of MyGo's widgets.
package ui

import (
	"compress/gzip"
	"fmt"
	"io"

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
		"fonts/Inter.ttf.gz":         FontSans,
		"fonts/JetBrainsMono.ttf.gz": FontMono,
	} {
		data, err := readGzipFont(file)
		if err != nil {
			return fmt.Errorf("read font %s: %w", file, err)
		}
		if err := ui.RegisterFont(data, family); err != nil {
			return fmt.Errorf("register font %s: %w", family, err)
		}
	}
	return nil
}

func readGzipFont(name string) ([]byte, error) {
	f, err := resources.Fonts.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	return io.ReadAll(gz)
}

// Palette holds the design tokens that ui.Theme has no field for.
type Palette struct {
	Sidebar       ui.Color // the sidebar without vibrancy
	SidebarHover  ui.Color // a sidebar item under the pointer
	SidebarChosen ui.Color // the sidebar item of the page shown
	Divider       ui.Color // lines between areas
	TextFaint     ui.Color // captions, counters, placeholders
	Info          ui.Color // a check under way
	Shadow        ui.Color // raised elements: chosen item, buttons

	AccentSoft    ui.Color // behind accent tags
	AccentInk     ui.Color // accent text on AccentSoft
	Subtle        ui.Color // cards of states, table headers
	Muted         ui.Color // segmented tracks, neutral chips
	Line          ui.Color // borders of cards, lines between rows
	ControlBorder ui.Color // fields and source badges
	ActiveRow     ui.Color // the table row of the copy in use
	MonoText      ui.Color // paths, versions, commands
	SuccessSoft   ui.Color // behind a success chip
	SuccessText   ui.Color
	WarningSoft   ui.Color // behind a warning chip
	WarningText   ui.Color
	InfoSoft      ui.Color // behind a step under way
	InfoText      ui.Color
	DangerSoft    ui.Color // behind a failed step
	DangerText    ui.Color

	// The overview's hero: its gradient from the top left, its border and
	// the background of its counts.
	HeroFrom, HeroMid, HeroTo ui.Color
	HeroBorder                ui.Color
	StatBg                    ui.Color
}

// Colors returns the palette that goes with t, light or dark.
func Colors(t *ui.Theme) Palette {
	if t.Dark {
		return Palette{
			Sidebar:       ui.Hex("#252320"),
			SidebarHover:  ui.Hex("#2F2C28"),
			SidebarChosen: ui.Hex("#36332E"),
			Divider:       ui.Hex("#34312C"),
			TextFaint:     ui.Hex("#7D776E"),
			Info:          ui.Hex("#5B8EF0"),
			Shadow:        ui.RGBA(0, 0, 0, 0.24),
			AccentSoft:    ui.Hex("#3A241B"),
			AccentInk:     ui.Hex("#FF8A62"),
			Subtle:        ui.Hex("#211F1C"),
			Muted:         ui.Hex("#2C2926"),
			Line:          ui.Hex("#2E2B27"),
			ControlBorder: ui.Hex("#3A3631"),
			ActiveRow:     ui.Hex("#2B211B"),
			MonoText:      ui.Hex("#CFCAC2"),
			SuccessSoft:   ui.Hex("#1A2E21"),
			SuccessText:   ui.Hex("#5FD08E"),
			WarningSoft:   ui.Hex("#33291A"),
			WarningText:   ui.Hex("#E3AE4A"),
			InfoSoft:      ui.Hex("#1C2639"),
			InfoText:      ui.Hex("#8DB4F5"),
			DangerSoft:    ui.Hex("#3B1F1C"),
			DangerText:    ui.Hex("#FF8A80"),
			HeroFrom:      ui.Hex("#2E231C"),
			HeroMid:       ui.Hex("#26201B"),
			HeroTo:        ui.Hex("#221F1B"),
			HeroBorder:    ui.Hex("#3D2E25"),
			StatBg:        ui.Hex("#2C2926").Alpha(0.8),
		}
	}
	return Palette{
		Sidebar:       ui.Hex("#F4F2EF"),
		SidebarHover:  ui.Hex("#E8E5E0"),
		SidebarChosen: ui.Hex("#FFFFFF"),
		Divider:       ui.Hex("#E8E5E0"),
		TextFaint:     ui.Hex("#9D988F"),
		Info:          ui.Hex("#3A73DA"),
		Shadow:        ui.RGBA(0x2A, 0x1E, 0x10, 0.08),
		AccentSoft:    ui.Hex("#FDEEE7"),
		AccentInk:     ui.Hex("#B9441F"),
		Subtle:        ui.Hex("#FAF9F7"),
		Muted:         ui.Hex("#F1EFEB"),
		Line:          ui.Hex("#EEEBE6"),
		ControlBorder: ui.Hex("#E2DED8"),
		ActiveRow:     ui.Hex("#FFF8F3"),
		MonoText:      ui.Hex("#4A4640"),
		SuccessSoft:   ui.Hex("#E7F5EC"),
		SuccessText:   ui.Hex("#23794A"),
		WarningSoft:   ui.Hex("#FBF2DE"),
		WarningText:   ui.Hex("#9A6300"),
		InfoSoft:      ui.Hex("#E8EFFC"),
		InfoText:      ui.Hex("#244A8F"),
		DangerSoft:    ui.Hex("#FBEAE8"),
		DangerText:    ui.Hex("#8C2A22"),
		HeroFrom:      ui.Hex("#FFF1E6"),
		HeroMid:       ui.Hex("#FFF9F4"),
		HeroTo:        ui.Hex("#FAF8F5"),
		HeroBorder:    ui.Hex("#F0E2D6"),
		StatBg:        ui.Hex("#FFFFFF").Alpha(0.8),
	}
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
