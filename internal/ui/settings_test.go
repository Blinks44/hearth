package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
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

func TestSettingsFoldersCustomAppsAndHiddenIssues(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.Home = dir
	a.CachePath = filepath.Join(dir, "Library", "Caches", "Hearth", "versions")
	a.DataDir = filepath.Join(dir, "Library", "Application Support", "Hearth")
	a.LogsDir = filepath.Join(dir, "Library", "Logs", "Hearth")
	a.SkillBackups = filepath.Join(dir, "Library", "Application Support", "Hearth", "skill-backups")
	a.Terminals = []string{"Terminal", "Ghostty"}
	a.Snapshot = &inventory.Snapshot{
		TerminalPath: []inventory.PathEntry{
			{Dir: "/opt/homebrew/bin"},
			{Dir: filepath.Join(dir, "gone", "bin"), Missing: true},
		},
		GUIPath: []inventory.PathEntry{{Dir: "/usr/bin"}},
	}
	var opened []string
	var missingErr error
	a.OpenFolder = func(path string, done func(error)) {
		opened = append(opened, path)
		done(missingErr)
	}
	var picked string
	a.PickPath = func(_ string, directory bool, done func(string, error)) {
		if directory {
			t.Error("expected file picker for custom app")
		}
		done(picked, nil)
	}
	saves := 0
	a.OnSettings = func() { saves++ }

	a.router.Push(settings.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Populate Hidden with one active issue from the snapshot and one stale/inactive key.
	// Inspect the actual active issue key generated by a.issues so the test stays realistic.
	activeIssues, _ := a.visibleIssues(a.issues(nil))
	if len(activeIssues) == 0 {
		t.Fatal("expected at least one issue in test snapshot")
	}
	activeKey := activeIssues[0].key
	activeTitle := activeIssues[0].title
	a.Hidden = []string{activeKey, "stale-issue-key"}
	tt.Frame()
	tt.Advance(time.Second)

	// 1. Folders card: open Data, Logs, Backups; check missing folder error feedback.
	for _, tc := range []struct {
		label string
		want  string
	}{
		{locales.SettingsOpen() + " " + locales.SettingsDataDir(), a.DataDir},
		{locales.SettingsOpen() + " " + locales.SettingsLogsDir(), a.LogsDir},
		{locales.SettingsOpen() + " " + locales.SettingsBackupsDir(), a.SkillBackups},
	} {
		if err := tt.Click(tc.label); err != nil {
			t.Fatalf("click %q: %v", tc.label, err)
		}
		tt.Frame()
		if len(opened) == 0 || opened[len(opened)-1] != tc.want {
			t.Fatalf("opened = %v, want last %q", opened, tc.want)
		}
	}
	missingErr = os.ErrNotExist
	if err := tt.Click(locales.SettingsOpen() + " " + locales.SettingsBackupsDir()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.SettingsFolderMissing(tilde(a.SkillBackups, a.Home))) {
		t.Fatalf("missing folder error not shown: %q", a.SettingsError)
	}
	a.SettingsError = ""
	missingErr = nil
	tt.Frame()

	// 2. Custom terminal path via select + input + Save (expands ~).
	if err := tt.Click(locales.SettingsPickTerminal()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.SettingsCustomPath()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.terminalCustomInput = "~/bin/myterm"
	tt.Frame()
	if err := tt.Click(locales.SettingsSave() + " " + locales.SettingsTerminal()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wantTerm := filepath.Join(dir, "bin", "myterm")
	if a.Terminal != wantTerm {
		t.Fatalf("Terminal = %q, want %q", a.Terminal, wantTerm)
	}

	// 3. Custom editor path via select + Browse… picker; chosenEditor() returns it.
	if err := tt.Click(locales.SettingsPickEditor()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.SettingsCustomPath()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	picked = filepath.Join(home, "CustomEdit.app")
	if err := tt.Click(locales.SettingsBrowse() + " " + locales.SettingsEditor()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Editor != picked {
		t.Fatalf("Editor = %q, want %q", a.Editor, picked)
	}
	ed, ok := a.chosenEditor()
	if !ok || ed.Path != picked {
		t.Fatalf("chosenEditor() = (%+v, %v), want path %q", ed, ok, picked)
	}

	// Capture full Settings screenshot with folders, custom paths, and hidden issues,
	// and verify all Select controls in the left column align on the same vertical grid.
	langSel, okL := tt.Find(locales.SettingsChooseLanguage())
	termSel, okT := tt.Find(locales.SettingsPickTerminal())
	edSel, okE := tt.Find(locales.SettingsPickEditor())
	if !okL || !okT || !okE {
		t.Fatalf("missing select controls: lang=%v term=%v ed=%v", okL, okT, okE)
	}
	if langSel.X != termSel.X || termSel.X != edSel.X || langSel.W != termSel.W {
		t.Fatalf("select alignment drift: lang=(%.1f,%.1f) term=(%.1f,%.1f) ed=(%.1f,%.1f)",
			langSel.X, langSel.W, termSel.X, termSel.W, edSel.X, edSel.W)
	}
	screenshot(t, tt, "settings-full")
	narrow := ui.NewTester(a.View, 1000, 680)
	narrow.Advance(time.Second)
	screenshot(t, narrow, "settings-narrow")

	// 4. Hidden issues list: restore one selectively, then restore all.
	if !tt.HasText(activeTitle) || !tt.HasText("stale-issue-key") || !tt.HasText(locales.SettingsHiddenInactive()) {
		t.Fatalf("hidden issues list incomplete: %q", tt.Texts())
	}
	if err := tt.Click(locales.ProblemsRestore() + " " + activeTitle); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(a.Hidden, []string{"stale-issue-key"}) {
		t.Fatalf("after selective restore: Hidden = %v", a.Hidden)
	}
	if err := tt.Click(locales.ProblemsRestoreAll()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.Hidden) != 0 || !tt.HasText(locales.SettingsNoHiddenIssues()) {
		t.Fatalf("after restore all: Hidden = %v", a.Hidden)
	}
	if saves < 4 {
		t.Fatalf("expected settings saves >= 4, got %d", saves)
	}
}

func TestSettingsTrayAndLogin(t *testing.T) {
	a := NewApp()
	a.TrayEnabled = true
	saves := 0
	a.OnSettings = func() { saves++ }
	var loginCalls []bool
	var loginErr error
	a.OnOpenAtLogin = func(enabled bool) error {
		loginCalls = append(loginCalls, enabled)
		return loginErr
	}
	a.router.Push(settings.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	clickSwitch := func(name string) {
		t.Helper()
		info, ok := tt.AccessByRole(ui.RoleSwitch, name)
		if !ok {
			t.Fatalf("missing switch %q", name)
		}
		tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
		tt.Frame()
	}

	if !tt.HasText(locales.SettingsSystemSection()) {
		t.Fatalf("missing system section: %q", tt.Texts())
	}
	clickSwitch(settingsTrayLabel())
	if a.TrayEnabled || saves != 1 {
		t.Fatalf("tray toggle: enabled=%v saves=%d", a.TrayEnabled, saves)
	}
	clickSwitch(locales.SettingsOpenAtLogin())
	if !a.OpenAtLogin || !slices.Equal(loginCalls, []bool{true}) {
		t.Fatalf("open at login: enabled=%v calls=%v", a.OpenAtLogin, loginCalls)
	}
	// Error from OS must roll back the switch and show SettingsError.
	loginErr = os.ErrPermission
	clickSwitch(locales.SettingsOpenAtLogin())
	if !a.OpenAtLogin || a.SettingsError == "" {
		t.Fatalf("expected rollback on error: enabled=%v err=%q", a.OpenAtLogin, a.SettingsError)
	}
}

func TestSettingsAppUpdate(t *testing.T) {
	a := NewApp()
	a.AppVersion = "1.0.0"
	a.AppUpdateEnabled = true
	a.AppUpdateAutoCheck = true
	var checks int
	var autoCalls []bool
	a.CheckAppUpdate = func() { checks++ }
	a.OnAppUpdateAutoCheck = func(on bool) { autoCalls = append(autoCalls, on) }
	a.router.Push(settings.path)

	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	if !tt.HasText("1.0.0") || !tt.HasText(locales.AppUpdateCheckButton()) || !tt.HasText(locales.AppUpdateAutoCheck()) {
		t.Fatalf("missing app update controls: %q", tt.Texts())
	}
	if err := tt.Click(locales.AppUpdateCheckButton()); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("checks = %d, want 1", checks)
	}

	info, ok := tt.AccessByRole(ui.RoleSwitch, locales.AppUpdateAutoCheck())
	if !ok {
		t.Fatalf("missing auto-check switch")
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	tt.Frame()
	if a.AppUpdateAutoCheck || !slices.Equal(autoCalls, []bool{false}) {
		t.Fatalf("auto-check toggle: enabled=%v calls=%v", a.AppUpdateAutoCheck, autoCalls)
	}

	// When a newer version is available, the button switches to Update… and subtitle names the version.
	a.AppUpdateAvailable = "1.1.0"
	tt.Frame()
	if !tt.HasText(locales.AppUpdateAvailableShort("1.1.0")) || !tt.HasText(locales.AppUpdateInstallButton()) {
		t.Fatalf("missing available update state: %q", tt.Texts())
	}
	if err := tt.Click(locales.AppUpdateInstallButton()); err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Fatalf("checks after Update… click = %d, want 2", checks)
	}
}
