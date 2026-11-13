package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
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
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	screenshot(t, tt, "settings-licenses")
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "SIL Open Font License") || !tt.HasText("Licenses") {
		t.Fatalf("licenses dialog: %q", tt.Texts())
	}
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.licensesOpen {
		t.Fatal("licenses dialog did not close")
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

// The language menu lists every catalog in its own language, and the
// languages an AI translated say so under the setting.
func TestSettingsLanguages(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := NewApp()
	a.Locale = "zh-Hans"
	var chosen string
	a.OnLocale = func(lang string) { chosen = lang }
	a.router.Push(settings.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "settings-zh")
	for _, text := range []string{"界面语言", "中文（简体）", "由 AI 翻译：可能不准确"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("选择界面语言"); err != nil {
		t.Fatal(err)
	}
	for _, name := range languageNames {
		if !tt.HasText(name) {
			t.Fatalf("menu lacks %q: %q", name, tt.Texts())
		}
	}
	if err := tt.Click("Deutsch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Locale != "de" || chosen != "de" {
		t.Fatal(a.Locale, chosen)
	}
}
