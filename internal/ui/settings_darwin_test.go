//go:build darwin

package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

func TestSettingsDockIconToggleAndInvariant(t *testing.T) {
	a := NewApp()
	a.TrayEnabled = true
	a.ShowInDock = true
	saves := 0
	a.OnSettings = func() { saves++ }

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

	// Turning Dock icon off works while menu bar icon is enabled.
	clickSwitch(locales.SettingsDockLabel())
	if a.ShowInDock || saves != 1 {
		t.Fatalf("dock toggle off: showInDock=%v saves=%d", a.ShowInDock, saves)
	}

	// Turning menu bar icon off must restore ShowInDock and disable the Dock switch.
	clickSwitch(settingsTrayLabel())
	if a.TrayEnabled || !a.ShowInDock || saves != 2 {
		t.Fatalf("tray toggle off must restore dock: tray=%v dock=%v saves=%d", a.TrayEnabled, a.ShowInDock, saves)
	}
	if !tt.HasText(locales.SettingsDockRequiresTray()) {
		t.Fatalf("missing requires-tray hint: %q", tt.Texts())
	}

	// Clicking the disabled Dock switch must not turn it off again.
	clickSwitch(locales.SettingsDockLabel())
	if !a.ShowInDock || saves != 2 {
		t.Fatalf("disabled dock switch should be a no-op: dock=%v saves=%d", a.ShowInDock, saves)
	}
}
