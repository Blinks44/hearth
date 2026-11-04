package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	hearthui "github.com/Blinks44/hearth/internal/ui"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/internal/watch"
)

func main() {
	if err := hearthui.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	a := hearthui.NewApp()
	a.Machine = machine()
	a.Home, _ = os.UserHomeDir() // without it, paths show in full
	a.Shell = mygo.Shell
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors}
	list := &projectList{app: a}
	if dir, err := mygo.App.Path(mygo.PathUserData); err != nil {
		log.Print(err)
	} else {
		list.dir = dir
		// A broken file is kept aside; Hearth starts with the defaults.
		if list.settings, err = store.LoadSettings(dir); err != nil {
			log.Print(err)
		}
	}
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
		list.win = win
		a.ChooseFolder, a.AddProjects, a.RemoveProject = list.choose, list.add, list.remove
		a.LoadSkill = func(dir string, done func([]inventory.Entry, string, error)) {
			go func() {
				entries, text, err := ai.SkillFolder(dir)
				win.Update(func() { done(entries, text, err) })
			}()
		}
		// Project files and the shells' startup files are followed: a
		// change reads that project again, or scans again.
		list.watch = &watch.Service{
			Start: func(ctx context.Context, path string) (watch.Watcher, error) {
				return mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{Debounce: 300 * time.Millisecond})
			},
			Project: func(p projects.Project) { win.Update(func() { list.update(p) }) },
			Shell:   func() { win.Update(a.Scan) },
			Err:     func(err error) { log.Print(err) },
		}
		list.watch.StartupFiles(detect.StartupFiles(sysenv.GUI(), a.Home))
		list.read()
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
		// The version lists, kept a day, come again with every scan.
		lists := &versions.Cache{Dir: filepath.Join(cache, "versions")}
		if cache == "" {
			lists.Dir = filepath.Join(os.TempDir(), "hearth-versions")
		}
		fetchVersions := func() {
			go func() {
				n, err := versions.FetchNode(context.Background(), lists)
				if err != nil {
					log.Print(err)
				}
				win.Update(func() {
					if err != nil {
						// A list shown before stays.
						a.NodeVersionsErr = err.Error()
						return
					}
					a.NodeVersions, a.NodeVersionsErr = &n, ""
				})
			}()
		}
		a.OnScan = func() {
			scanner.Start(scanned)
			fetchVersions()
			list.read()
		}
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
		fetchVersions()
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
