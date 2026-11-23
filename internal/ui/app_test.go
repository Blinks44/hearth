package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Every sidebar item opens its page, whose header shows its title; the
// languages not found wait under "Not found".
func TestSidebarOpensPages(t *testing.T) {
	a := NewApp()
	a.Snapshot = found("node", "bun", "python", "php")
	// Tall enough for every item without scrolling the sidebar.
	tt := ui.NewTester(a.View, 1280, 1800)
	if tt.HasText("Rust") {
		t.Error("a language not found shows before opening the rest")
	}
	if err := tt.Click("Not found"); err != nil {
		t.Fatal(err)
	}
	for _, p := range pages() {
		if err := tt.Click(p.title()); err != nil {
			t.Fatalf("%s: %v", p.title(), err)
		}
		tt.Advance(time.Second) // the page before fades out
		if a.router.Path() != p.path || a.router.Title() != p.title() {
			t.Errorf("clicked %q: page %q titled %q", p.title(), a.router.Path(), a.router.Title())
		}
		built := slices.Contains([]string{settings.path, setupPage, pathPage, "/system/packages", "/tools/databases", "/tools/docker", "/tools/jspm", "/tools/node", "/tools/bun", "/tools/go", "/tools/python", "/tools/php", "/tools/rust", "/tools/java", "/tools/ruby", "/tools/dotnet", projectsPage.path, cleanupPage, problemsPage, updatesPage, gitPage, cliPage, kubePage, mobilePage, editorsPage, cppPage, modelsPage, overview.path, "/ai/agents", skillsPage, mcpPage, portsPage, usagePage, instructionsPage}, p.path)
		if !tt.HasText(p.subtitle()) || !built && !tt.HasText("Coming soon") {
			t.Errorf("%s: header missing, texts %q", p.title(), tt.Texts())
		}
	}
}

// A language page not found on this computer still shows in the sidebar
// while it is open.
func TestSidebarKeepsOpenLanguage(t *testing.T) {
	a := NewApp()
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("Not found") || !tt.HasText("10") || tt.HasText("Rust") {
		t.Fatalf("texts %q", tt.Texts())
	}
	a.router.Push("/tools/rust")
	tt.Advance(time.Second)
	if !tt.HasText("Rust") || !tt.HasText("9") {
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

// In Russian the shell says the catalog's words, with Russian plurals.
func TestShellInRussian(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := NewApp()
	a.Snapshot = found("node", "bun", "python", "php")
	tt := ui.NewTester(a.View, 1280, 820)
	a.Snapshot.Taken = tt.Now().Add(-5 * time.Minute)
	tt.Frame()
	for _, s := range []string{"Обзор", "Всё окружение разработчика на этом компьютере", "Языки и рантаймы", "Обслуживание", "Не найдено", "Сканировать", "Проверено 5 мин назад"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if err := tt.Click("Проекты"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Title() != "Проекты" || !tt.HasText("Сканировать") {
		t.Errorf("title %q, texts %q", a.router.Title(), tt.Texts())
	}
}

// found is a snapshot that found the tools by their IDs.
func found(tools ...string) *inventory.Snapshot {
	s := &inventory.Snapshot{}
	for _, id := range tools {
		s.Tools = append(s.Tools, inventory.ToolState{Tool: id, Installations: []inventory.Installation{{Tool: id}}})
	}
	return s
}

// Scan starts one scan at a time, and the footer says it is under way,
// then how long ago it ended, as the clock goes on.
func TestScanStatus(t *testing.T) {
	a := NewApp()
	scans := 0
	a.OnScan = func() { scans++ }
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("Not checked yet") {
		t.Fatalf("texts %q", tt.Texts())
	}
	for range 2 {
		if err := tt.Click("Scan"); err != nil {
			t.Fatal(err)
		}
	}
	if scans != 1 || !a.Scanning || !tt.HasText("Checking…") {
		t.Errorf("%d scans, scanning %v, texts %q", scans, a.Scanning, tt.Texts())
	}

	// The scan's snapshot arrives, as Window.Update gives it.
	a.Snapshot, a.Scanning = found("node"), false
	a.Snapshot.Taken = tt.Now().Add(-90 * time.Second)
	tt.Frame()
	if !tt.HasText("Checked 1 min ago") || !tt.HasText("Node.js") {
		t.Errorf("texts %q", tt.Texts())
	}
	tt.Advance(time.Minute)
	if !tt.HasText("Checked 2 min ago") {
		t.Errorf("a minute on, texts %q", tt.Texts())
	}
	tt.Advance(3 * time.Hour)
	if !tt.HasText("Checked 3 h ago") {
		t.Errorf("hours on, texts %q", tt.Texts())
	}
	if err := tt.Click("Scan"); err != nil || scans != 2 {
		t.Errorf("scan again: %v, %d scans", err, scans)
	}
}

// The sidebar's theme button turns the appearance through system, light
// and dark, and keeps it.
func TestThemeButton(t *testing.T) {
	a := NewApp()
	saved := 0
	a.OnSettings = func() { saved++ }
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"light", "dark", "system"} {
		if err := tt.Click("Theme: " + map[string]string{"light": "System", "dark": "Light", "system": "Dark"}[want]); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.ThemeMode != want {
			t.Errorf("theme %q, want %q", a.ThemeMode, want)
		}
	}
	if saved != 3 {
		t.Errorf("saved %d times, want 3", saved)
	}
}

// Cmd+R scans, Cmd+, opens the settings, and / goes to the page's filter,
// where it types as any other character.
func TestShortcuts(t *testing.T) {
	a := NewApp()
	scans := 0
	a.OnScan = func() { scans++ }
	a.Snapshot = found("node")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyR)
	if scans != 1 {
		t.Errorf("%d scans after Cmd+R", scans)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Advance(time.Second)
	if a.router.Path() != settings.path {
		t.Errorf("Cmd+, went to %q", a.router.Path())
	}
	a.router.Push("/system/path")
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySlash)
	tt.Frame()
	tt.Type("usr/")
	tt.Frame()
	if a.pathFilter != "usr/" {
		t.Errorf("filter %q after / and typing, want %q", a.pathFilter, "usr/")
	}
}

func TestSidebarNotFound(t *testing.T) {
	a := NewApp()
	a.Snapshot = found("node", "git")
	tt := ui.NewTester(a.View, 1280, 1400)
	tt.Frame()
	// Installed languages and all Infrastructure, Tools, AI, and System pages stay in their own sections.
	for _, s := range []string{"Node.js", "Git", "Infrastructure", "Docker", "Kubernetes", "CLI & clouds", "Editors", "AI", "Local models", "PATH & variables"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in the sidebar", s)
		}
	}
	// Missing languages wait under Not found.
	for _, s := range []string{"Rust", "Go", "PHP"} {
		if tt.HasText(s) {
			t.Errorf("%q shows before Not found opens", s)
		}
	}
	for range 3 {
		if err := tt.Click("Not found"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(80 * time.Millisecond)
		tt.Advance(time.Second)
		if !a.notFoundOpen || !tt.HasText("Rust") || !tt.HasText("Go") {
			t.Fatal("not found did not open")
		}
		screenshot(t, tt, "sidebar-not-found-open")
		if err := tt.Click("Not found"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(80 * time.Millisecond)
		tt.Advance(time.Second)
		if a.notFoundOpen || tt.HasText("Rust") {
			t.Fatal("not found did not close")
		}
	}
	// A missing language stays in Languages while its page is open.
	a.router.Push("/tools/rust")
	tt.Advance(time.Second)
	if !tt.HasText("Rust") || a.notFoundOpen {
		t.Errorf("Rust is not in Languages while shown")
	}
	screenshot(t, tt, "sidebar-not-found-closed")
}
