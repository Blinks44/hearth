package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/projects"
)

func TestSettingsPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.CachePath = home + "/Library/Caches/Hearth/versions"
	a.Projects = []projects.Project{{Dir: home + "/dev/hearth"}}
	saved, cleared, removed := 0, 0, ""
	a.OnSettings = func() { saved++ }
	a.ClearCache = func() { cleared++ }
	a.OnLocale = func(lang string) {
		if lang != "ru" {
			t.Fatal(lang)
		}
	}
	a.RemoveProject = func(dir string) { removed = dir; a.Projects = nil }
	a.router.Push(settings.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "settings")
	for _, text := range []string{"Interface language", "Theme", "Version lists and package updates", "hearth", "Version and update cache", "Development build"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Allow version lists and package update checks"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.VersionLists || saved != 1 {
		t.Fatalf("network toggle: %v, saves %d", a.VersionLists, saved)
	}
	if err := tt.Click("Clear cache"); err != nil {
		t.Fatal(err)
	}
	if cleared != 1 {
		t.Fatal("cache action not called")
	}
	if err := tt.Click("Remove from list"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if removed != home+"/dev/hearth" || tt.HasText("hearth") {
		t.Fatal("project removal did not update settings")
	}
	if err := tt.Click("Choose theme"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.ThemeMode != "dark" || saved != 2 {
		t.Fatalf("theme %s, saves %d", a.ThemeMode, saved)
	}
	// mygo applies the saved theme to the whole app; the tester shows it
	// as the system's appearance.
	tt.SetDark(true)
	tt.Frame()
	screenshot(t, tt, "settings-dark")
	tt.SetDark(false)
	a.SettingsNotice = "Cache cleared"
	tt.Frame()
	if err := tt.Click("Choose interface language"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Русский"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Locale != "ru" || a.SettingsNotice != "" {
		t.Fatal(a.Locale, a.SettingsNotice)
	}
	if err := tt.Click("Choose theme"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if a.ThemeMode != "system" {
		t.Fatal(a.ThemeMode)
	}
}
