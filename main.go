package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	hearthui "github.com/Blinks44/hearth/internal/ui"
)

func main() {
	if err := hearthui.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	a := hearthui.NewApp()
	a.Machine = machine()
	a.Home, _ = os.UserHomeDir() // without it, paths show in full
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors}
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Hearth",
			Width:     1280,
			Height:    820,
			MinWidth:  1000,
			MinHeight: 680,
			// Opens where the user left it last time.
			StateKey: "main",
			// The view draws the title bar: the window controls sit over
			// the sidebar on macOS and over the page's header elsewhere.
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: titleBarHeight,
			// Centered in the sidebar's top row of 52, as in the design.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			// Under the sidebar: a material on macOS, Mica on Windows 11.
			Vibrancy: mygo.VibrancySidebar,
			Content:  ui.View(a.View),
		})
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Print(err)
		}
		scanned := func(s inventory.Snapshot) {
			win.Update(func() { a.Snapshot, a.Scanning = &s, false })
			if cache != "" {
				go func() {
					if err := store.SaveSnapshot(cache, s); err != nil {
						log.Print(err)
					}
				}()
			}
		}
		a.OnScan = func() { scanner.Start(scanned) }
		// The last snapshot shows at once, until the scan replaces it.
		if cache != "" {
			go func() {
				s, err := store.LoadSnapshot(cache)
				switch {
				case err == nil:
					win.Update(func() {
						if a.Snapshot == nil {
							a.Snapshot = &s
						}
					})
				case !errors.Is(err, os.ErrNotExist):
					log.Print(err)
				}
			}()
		}
		a.Scanning = true
		scanner.Start(scanned)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// machine is the caption under the logo: the computer's name and its OS,
// as "studio · macOS 15.4".
func machine() string {
	var parts []string
	if host, err := os.Hostname(); err == nil {
		parts = append(parts, strings.TrimSuffix(host, ".local"))
	}
	o, err := sysenv.ReadOS()
	if err != nil {
		log.Print(err)
	}
	parts = append(parts, strings.TrimSpace(o.Name+" "+o.Version))
	return strings.Join(parts, " · ")
}
