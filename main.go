package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/disk"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	hearthui "github.com/Blinks44/hearth/internal/ui"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/internal/watch"
	"github.com/Blinks44/hearth/locales"
)

func main() {
	if err := hearthui.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	a := hearthui.NewApp()
	a.Machine, a.OS = machine()
	a.Home, _ = os.UserHomeDir() // without it, paths show in full
	a.Shell = mygo.Shell
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors}
	list := &projectList{app: a, settings: store.DefaultSettings()}
	if dir, err := mygo.App.Path(mygo.PathUserData); err != nil {
		log.Print(err)
	} else {
		list.dir = dir
		a.SkillBackups = filepath.Join(dir, "skill-backups")
		// A broken file is kept aside; Hearth starts with the defaults.
		if list.settings, err = store.LoadSettings(dir); err != nil {
			log.Print(err)
		}
	}
	a.ThemeMode, a.VersionLists, a.Hidden = list.settings.Theme, list.settings.VersionLists, list.settings.Hidden
	// The native parts follow it too: the sidebar's material, the window
	// controls and the menus.
	mygo.Theme.SetSource(mygo.ThemeSource(a.ThemeMode))
	mygo.App.WhenReady(func() {
		a.Locale = mygo.App.Locale()
		a.AppVersion = mygo.App.Version()
		a.OnLocale = mygo.App.SetLocale
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
		a.FindProjects = list.find
		a.LoadSkill = func(dir string, done func([]inventory.Entry, string, error)) {
			go func() {
				entries, text, err := ai.SkillFolder(dir)
				win.Update(func() { done(entries, text, err) })
			}()
		}
		// Operations run one at a time in the background, as the user's
		// own commands would in the terminal, and append all their output
		// to the operations log.
		if logs, err := mygo.App.Path(mygo.PathLogs); err != nil {
			log.Print(err)
		} else {
			a.JournalPath = filepath.Join(logs, "operations.log")
		}
		a.DetectTool = scanner.Detect
		a.CountDocker = func(ctx context.Context, env sysenv.Env, s *inventory.Snapshot) ([]inventory.DockerUsage, error) {
			return disk.Docker(ctx, run.Exec{}, env, s)
		}
		a.DetectAgent = scanner.Agent
		a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
			env := scanner.Terminal()
			go func() {
				terminal := func(ctx context.Context) (sysenv.Env, error) { return sysenv.Terminal(ctx, run.Exec{}) }
				p, err := build(env, terminal)
				win.Update(func() { done(p, err) })
			}()
		}
		a.PlanPath = func(entries []inventory.PathEntry, dirs []string, done func(ops.Plan, error)) {
			env := scanner.Terminal()
			go func() {
				terminal := func(ctx context.Context) (sysenv.Env, error) { return sysenv.Terminal(ctx, run.Exec{}) }
				p, err := ops.PathPlan(entries, dirs, env, terminal)
				win.Update(func() { done(p, err) })
			}()
		}
		a.RunPlan = func(p ops.Plan, update func(ops.State)) func() {
			if p.Env == nil {
				p.Env = slices.Clone(scanner.Terminal())
			}
			ctx, cancel := context.WithCancel(context.Background())
			journalPath := a.JournalPath
			go func() {
				defer cancel()
				journal := io.Discard
				if journalPath != "" {
					f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
					if err != nil {
						log.Print(err)
					} else {
						defer func() { _ = f.Close() }()
						journal = f
					}
				}
				ops.Execute(ctx, run.Exec{}, p, journal, func(s ops.State) { win.Update(func() { update(s) }) })
			}()
			return cancel
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
		// The first start offers the projects in the usual code folders.
		if !list.settings.Suggested && len(list.settings.Projects) == 0 {
			list.find()
		}
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Print(err)
		}
		var fetchPackages func()
		// Disk space is counted after each scan, apart from it: a walk of
		// big caches takes long. The last count shows until it ends.
		var cancelSizes context.CancelFunc
		countSizes := func(s *inventory.Snapshot) {
			if cancelSizes != nil {
				cancelSizes()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelSizes = cancel
			a.Counting = true
			env := slices.Clone(scanner.Terminal())
			var installs []string
			for _, p := range a.Projects {
				for _, name := range p.Installs {
					installs = append(installs, filepath.Join(p.Dir, name))
				}
			}
			home := a.Home
			go func() {
				// Every project's node_modules under home is counted too.
				found, err := projects.NodeModules(ctx, home)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					log.Print(err)
				}
				// The view offers them at once; their sizes come as they
				// are counted.
				win.Update(func() {
					if ctx.Err() != nil {
						return
					}
					if a.Sizes == nil {
						a.Sizes = &inventory.Sizes{}
					}
					a.Sizes.Installs = found
				})
				for _, in := range found {
					if path := filepath.Join(in.Project, in.Name); !slices.Contains(installs, path) {
						installs = append(installs, path)
					}
				}
				sizes, err := disk.Count(ctx, run.Exec{}, env, s, installs, func(path string, u inventory.Usage) {
					win.Update(func() {
						if ctx.Err() != nil {
							return
						}
						if a.Sizes == nil {
							a.Sizes = &inventory.Sizes{}
						}
						if a.Sizes.Paths == nil {
							a.Sizes.Paths = map[string]inventory.Usage{}
						}
						a.Sizes.Paths[path] = u
					})
				})
				if err != nil {
					return
				}
				sizes.Installs = found
				// Saved before the view gets them: the next count adds to
				// the map the view has.
				if cache != "" {
					if err := store.SaveSizes(cache, sizes); err != nil {
						log.Print(err)
					}
				}
				win.Update(func() {
					if ctx.Err() == nil {
						a.Sizes, a.Counting = &sizes, false
					}
				})
			}()
		}
		scanned := func(s inventory.Snapshot) {
			win.Update(func() {
				// What changed since the check before goes in the history.
				if changes := history.Diff(a.Snapshot, &s, a.Sizes); len(changes) > 0 {
					a.History = history.Add(a.History, history.Entry{Taken: s.Taken, Changes: changes})
					if cache != "" {
						entries := slices.Clone(a.History)
						go func() {
							if err := store.SaveHistory(cache, entries); err != nil {
								log.Print(err)
							}
						}()
					}
				}
				a.Snapshot, a.Scanning = &s, false
				if fetchPackages != nil {
					fetchPackages()
				}
				countSizes(&s)
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
		a.RereadPackages = func(ctx context.Context, m inventory.PackageManager) (inventory.PackageManager, error) {
			env := scanner.Terminal()
			if err := pkgmgr.Forget(lists, env, m); err != nil {
				log.Print(err)
			}
			return pkgmgr.Reread(ctx, run.Exec{}, env, m)
		}
		var cancelVersions context.CancelFunc
		var cancelPackages context.CancelFunc
		fetchPackages = func() {
			if cancelPackages != nil {
				cancelPackages()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelPackages = cancel
			// The last check's results stay until the new one's come: the
			// packages and what they hold do not drop out meanwhile.
			if a.PackageUpdates == nil {
				a.PackageUpdates = map[string]inventory.PackageManager{}
			}
			a.PackageChecks = map[string]bool{}
			fetchMise(ctx, win, lists, a, scanner.Terminal())
			st, ok := a.Snapshot.Tool("packages")
			if !ok {
				return
			}
			managers := slices.Clone(st.PackageManagers)
			env := slices.Clone(scanner.Terminal())
			for _, m := range managers {
				if pkgmgr.HasNetworkQuery(m.ID) {
					a.PackageChecks[m.Path] = true
				}
			}
			go func() {
				for _, m := range managers {
					if ctx.Err() != nil {
						return
					}
					if !pkgmgr.HasNetworkQuery(m.ID) {
						continue
					}
					queryCtx, stop := context.WithTimeout(ctx, 2*time.Minute)
					result := pkgmgr.Network(queryCtx, lists, run.Exec{}, env, m)
					stop()
					win.Update(func() {
						if ctx.Err() != nil {
							return
						}
						a.PackageUpdates[m.Path] = result
						delete(a.PackageChecks, m.Path)
					})
				}
			}()
		}
		fetchVersions := func() {
			if cancelVersions != nil {
				cancelVersions()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelVersions = cancel
			lists.SetOffline(!a.VersionLists)
			fetchList(ctx, win, lists, versions.FetchNode, &a.NodeVersions, &a.NodeVersionsErr)
			fetchList(ctx, win, lists, versions.FetchJSManagers, &a.JSVersions, &a.JSVersionsErr)
			fetchList(ctx, win, lists, versions.FetchBun, &a.BunLatest, &a.BunLatestErr)
			fetchList(ctx, win, lists, versions.FetchGo, &a.GoVersions, &a.GoVersionsErr)
			fetchList(ctx, win, lists, versions.FetchPython, &a.PythonVersions, &a.PythonVersionsErr)
			fetchList(ctx, win, lists, versions.FetchPHP, &a.PHPVersions, &a.PHPVersionsErr)
			fetchList(ctx, win, lists, versions.FetchRust, &a.RustVersions, &a.RustVersionsErr)
			fetchList(ctx, win, lists, versions.FetchDotnet, &a.DotnetVersions, &a.DotnetVersionsErr)
			fetchList(ctx, win, lists, versions.FetchRuby, &a.RubyVersions, &a.RubyVersionsErr)
			fetchList(ctx, win, lists, versions.FetchJava, &a.JavaVersions, &a.JavaVersionsErr)
		}
		a.CachePath = lists.Dir
		win.OnClosed(func() {
			if cancelVersions != nil {
				cancelVersions()
			}
			if cancelPackages != nil {
				cancelPackages()
			}
			if cancelSizes != nil {
				cancelSizes()
			}
		})
		a.OnSettings = func() {
			onlineChanged := list.settings.VersionLists != a.VersionLists
			list.settings.Theme, list.settings.VersionLists, list.settings.Hidden = a.ThemeMode, a.VersionLists, a.Hidden
			mygo.Theme.SetSource(mygo.ThemeSource(a.ThemeMode))
			list.save()
			if onlineChanged {
				fetchVersions()
				fetchPackages()
			}
		}
		a.ClearCache = func() {
			if a.CacheBusy {
				return
			}
			a.CacheBusy = true
			if cancelPackages != nil {
				cancelPackages()
			}
			a.PackageUpdates = nil
			a.PackageChecks = nil
			a.SettingsNotice = ""
			if cancelVersions != nil {
				cancelVersions()
			}
			go func() {
				err := lists.Clear()
				win.Update(func() {
					a.CacheBusy = false
					if err != nil {
						a.SettingsError = locales.SettingsCacheError(err.Error())
						return
					}
					a.NodeVersions = nil
					a.JSVersions = nil
					a.BunLatest = nil
					a.GoVersions = nil
					a.PythonVersions = nil
					a.PHPVersions = nil
					a.RustVersions = nil
					a.DotnetVersions = nil
					a.RubyVersions = nil
					a.JavaVersions = nil
					a.NodeVersionsErr = ""
					a.JSVersionsErr = ""
					a.BunLatestErr = ""
					a.GoVersionsErr = ""
					a.PythonVersionsErr = ""
					a.PHPVersionsErr = ""
					a.RustVersionsErr = ""
					a.DotnetVersionsErr = ""
					a.RubyVersionsErr = ""
					a.JavaVersionsErr = ""
					a.SettingsError = ""
					a.SettingsNotice = locales.SettingsCleared()
					fetchVersions()
					fetchPackages()
				})
			}()
		}
		a.OnScan = func() {
			if cancelPackages != nil {
				cancelPackages()
			}
			a.PackageChecks = nil
			scanner.Start(scanned)
			fetchVersions()
			list.read()
		}
		// The last snapshot shows at once, until the scan replaces it.
		if cache != "" {
			go func() {
				// The history comes before the snapshot: a scan compares
				// with the snapshot only once the history is there.
				entries, err := store.LoadHistory(cache)
				switch {
				case err == nil:
					win.Update(func() {
						if a.History == nil {
							a.History = entries
						}
					})
				case !errors.Is(err, os.ErrNotExist):
					log.Print(err)
				}
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
				sizes, err := store.LoadSizes(cache)
				switch {
				case err == nil:
					win.Update(func() {
						if a.Sizes == nil {
							a.Sizes = &sizes
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

// fetchMise asks mise in the background which of the tools it installed
// have newer versions, those with a page of their own as Node too, and
// puts the answer in a.MiseOutdated on the UI thread; a failed ask keeps
// the one before.
func fetchMise(ctx context.Context, win *mygo.Window, lists *versions.Cache, a *hearthui.App, env sysenv.Env) {
	found := sysenv.LookPathAll("mise", env)
	var installed []string
	if a.Snapshot != nil {
		for _, st := range a.Snapshot.Tools {
			for _, in := range st.Installations {
				if in.Source.Manager == "mise" && !in.Source.Shim {
					installed = append(installed, filepath.Base(filepath.Dir(in.Source.Root))+"@"+in.Version)
				}
			}
			for _, mt := range st.MiseTools {
				for _, in := range mt.Installations {
					installed = append(installed, mt.Name+"@"+in.Version)
				}
			}
		}
	}
	if len(installed) == 0 || len(found) == 0 {
		a.MiseOutdated = nil
		return
	}
	env = slices.Clone(env)
	go func() {
		m, err := versions.FetchMise(ctx, lists, run.Exec{}, env, found[0], installed)
		if err != nil && !errors.Is(err, versions.ErrOffline) && ctx.Err() == nil {
			log.Print(err)
		}
		win.Update(func() {
			if ctx.Err() == nil && (err == nil || len(m.Tools) > 0) {
				a.MiseOutdated = &m
			}
		})
	}()
}

// fetchList gets a version list through lists in the background and
// puts it in *dst on the UI thread; when it cannot, the list shown before
// stays and *errText says why.
func fetchList[T any](ctx context.Context, win *mygo.Window, lists *versions.Cache, fetch func(context.Context, *versions.Cache) (T, error), dst **T, errText *string) {
	go func() {
		list, err := fetch(ctx, lists)
		if err != nil && !errors.Is(err, versions.ErrOffline) {
			log.Print(err)
		}
		win.Update(func() {
			if ctx.Err() != nil {
				return
			}
			switch {
			case errors.Is(err, versions.ErrOffline):
				*errText = locales.VersionsOffline()
				return
			case err != nil:
				*errText = err.Error()
				// A request that got no answer says which server, not Go's
				// wording of the network error.
				if uerr, ok := errors.AsType[*url.Error](err); ok {
					if u, perr := url.Parse(uerr.URL); perr == nil {
						*errText = locales.VersionsUnreachable(u.Host)
					}
				}
				return
			}
			*dst, *errText = &list, ""
		})
	}()
}

// machine is the caption under the logo: the computer's name and its OS,
// as "studio · macOS 15.4"; and the OS for the report, as "macOS 15.4
// (arm64)".
func machine() (caption, system string) {
	var parts []string
	if host, err := os.Hostname(); err == nil {
		parts = append(parts, strings.TrimSuffix(host, ".local"))
	}
	o, err := sysenv.ReadOS()
	if err != nil {
		log.Print(err)
	}
	name := strings.TrimSpace(o.Name + " " + o.Version)
	parts = append(parts, name)
	return strings.Join(parts, " · "), name + " (" + o.Arch + ")"
}
