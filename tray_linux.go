//go:build !darwin && !windows

package main

import (
	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/resources"
)

func trayIcon() (png []byte, template bool) {
	return resources.TrayIconColor, false
}

func (t *trayController) buildMenu() *mygo.Menu {
	items := buildTrayMenuItems(t.app.Snapshot, t.containers, t.switching, t.app.AppUpdateAvailable, trayActions{
		openPopover: t.showPopover,
		openHearth: func() {
			t.showWindow()
		},
		openContainers: func() {
			t.app.OpenDockerContainers()
			t.showWindow()
		},
		openDocker: func() {
			t.app.OpenDocker()
			t.showWindow()
		},
		startColima: func() {
			t.app.TriggerColima(true)
			t.showWindow()
		},
		stopColima: func() {
			t.app.TriggerColima(false)
			t.showWindow()
		},
		switchDockerCtx: t.switchDockerContext,
		switchKubeCtx:   t.switchKubeContext,
		checkAppUpdate: func() {
			if t.app.CheckAppUpdate != nil {
				t.app.CheckAppUpdate()
			}
		},
		openSettings: func() {
			t.app.OpenSettings()
			t.showWindow()
		},
		quit: func() {
			if t.onQuit != nil {
				t.onQuit()
			}
		},
	})
	return mygo.NewMenu(items)
}

func (t *trayController) initialTrayMenu() *mygo.Menu {
	return t.buildMenu()
}

func (t *trayController) bindTrayClicks() {
	t.tray.OnClick(func() {
		t.togglePopover()
	})
}

func (t *trayController) syncTrayMenu() {
	t.tray.SetMenu(t.buildMenu())
}
