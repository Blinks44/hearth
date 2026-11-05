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
		// change reads that project again, or scans again. A change of the
		// Docker engine's containers checks Docker and the databases again.
		list.watch = &watch.Service{
			Start: func(ctx context.Context, path string) (watch.Watcher, error) {
				return mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{Debounce: 300 * time.Millisecond})
			},
			Project: func(p projects.Project) { win.Update(func() { list.update(p) }) },
			Shell:   func() { win.Update(a.Scan) },
			Lines:   run.Lines,
			Containers: func() {
				scanner.Refresh([]string{"docker", "databases"}, func(states []inventory.ToolState) {
					win.Update(func() {
						if a.Snapshot == nil {
							return
						}
						s := scan.Replace(*a.Snapshot, states)
						a.Snapshot = &s
						list.watch.Docker(&s, scanner.Terminal())
					})
				})
			},
			Err: func(err error) { log.Print(err) },
		}
		list.watch.StartupFiles(detect.StartupFiles(sysenv.GUI(), a.Home))
		list.read()
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Print(err)
		}
		scanned := func(s inventory.Snapshot) {
			win.Update(func() {
				a.Snapshot, a.Scanning = &s, false
				list.watch.Docker(&s, scanner.Terminal())
			})
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
			fetchList(win, lists, versions.FetchNode, &a.NodeVersions, &a.NodeVersionsErr)
			fetchList(win, lists, versions.FetchJSManagers, &a.JSVersions, &a.JSVersionsErr)
			fetchList(win, lists, versions.FetchBun, &a.BunLatest, &a.BunLatestErr)
			fetchList(win, lists, versions.FetchGo, &a.GoVersions, &a.GoVersionsErr)
			fetchList(win, lists, versions.FetchPython, &a.PythonVersions, &a.PythonVersionsErr)
			fetchList(win, lists, versions.FetchPHP, &a.PHPVersions, &a.PHPVersionsErr)
			fetchList(win, lists, versions.FetchRust, &a.RustVersions, &a.RustVersionsErr)
			fetchList(win, lists, versions.FetchDotnet, &a.DotnetVersions, &a.DotnetVersionsErr)
			fetchList(win, lists, versions.FetchRuby, &a.RubyVersions, &a.RubyVersionsErr)
			fetchList(win, lists, versions.FetchJava, &a.JavaVersions, &a.JavaVersionsErr)
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

// fetchList gets a version list through lists in the background and
// puts it in *dst on the UI thread; when it cannot, the list shown before
// stays and *errText says why.
func fetchList[T any](win *mygo.Window, lists *versions.Cache, fetch func(context.Context, *versions.Cache) (T, error), dst **T, errText *string) {
	go func() {
		list, err := fetch(context.Background(), lists)
		if err != nil {
			log.Print(err)
		}
		win.Update(func() {
			if err != nil {
				*errText = err.Error()
				return
			}
			*dst, *errText = &list, ""
		})
	}()
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
