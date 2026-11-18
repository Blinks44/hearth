package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/disk"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/proposals"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	hearthui "github.com/Blinks44/hearth/internal/ui"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/internal/watch"
	"github.com/Blinks44/hearth/locales"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Exit(serveMCP(os.Args[2:], os.Stderr))
	}
	// sudo runs Hearth as its askpass program, for a plan's step.
	if why := os.Getenv(ops.AskpassEnv); why != "" {
		os.Exit(askpass(why))
	}
	if err := hearthui.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	a := hearthui.NewApp()
	a.Machine, a.OS = machine()
	a.Home, _ = os.UserHomeDir() // without it, paths show in full
	a.Shell = mygo.Shell
	a.HearthExecutable, _ = os.Executable()
	ops.AskpassProgram = a.HearthExecutable
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
	a.StartHidden = list.settings.StartHidden
	a.KubeLinks = list.kubeLinks()
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
		catalogCtx, stopCatalog := context.WithCancel(context.Background())
		a.LoadCatalog = func(repo skillcatalog.Repository, done func([]skillcatalog.Skill, map[string]string, error)) func() {
			ctx, cancel := context.WithTimeout(catalogCtx, 2*time.Minute)
			env := scanner.Terminal()
			go func() {
				defer cancel()
				skills, err := skillcatalog.Fetch(ctx, nil, repo)
				dirs := ai.SkillDirectories(env)
				win.Update(func() { done(skills, dirs, err) })
			}()
			return cancel
		}
		// IPC is a request to select a plan, never an execution endpoint.
		stopProposals, err := proposals.Listen(list.dir, func(ctx context.Context, q proposals.Request) error {
			done := make(chan error, 1)
			win.Update(func() {
				if ctx.Err() != nil {
					done <- ctx.Err()
					return
				}
				err := a.RequestFix(q)
				if err == nil {
					win.Show()
					win.Focus()
				}
				done <- err
			})
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return errors.New("hearth window did not respond; retry in the desktop")
			}
		})
		if err != nil {
			log.Print(err)
		} else {
			win.OnClosed(stopProposals)
		}
		list.win = win
		a.ChooseFolder, a.AddProjects, a.RemoveProject = list.choose, list.add, list.remove
		a.FindProjects = list.find
		a.LinkKube = list.linkKube
		a.LoadSkill = func(dir string, done func([]inventory.Entry, string, error)) {
			go func() {
				entries, text, err := ai.SkillFolder(dir)
				win.Update(func() { done(entries, text, err) })
			}()
		}
		a.CheckMCP = func(s inventory.MCPServer, done func(ai.MCPCheck, error)) {
			env := scanner.Terminal()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), ai.MCPCheckTimeout)
				defer cancel()
				res, err := ai.CheckMCP(ctx, s, env)
				win.Update(func() { done(res, err) })
			}()
		}
		a.CheckKube = func(kubectl string, k inventory.KubeContext, done func(detect.KubeAccess)) {
			env := scanner.Terminal()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), detect.KubeAccessTimeout)
				defer cancel()
				res := detect.CheckKubeAccess(ctx, run.Exec{}, kubectl, k, env)
				win.Update(func() { done(res) })
			}()
		}
		a.SaveSet = func(data []byte, done func(string, error)) { saveSet(win, data, done) }
		a.OpenSet = func(done func(string, []byte, error)) { openSet(win, done) }
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
		list.watch.StartupFiles(append(detect.StartupFiles(sysenv.GUI(), a.Home), detect.KubeConfigFiles(scanner.Terminal(), a.Home)...))
		list.read()
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Print(err)
		}
		var fetchPackages func()
		// Disk space is counted after each scan, apart from it: a walk of
		// big caches takes long. Each result replaces the previous count.
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
			go func() {
				sizes, err := disk.Count(ctx, run.Exec{}, env, s, installs, func(next inventory.Sizes) {
					win.Update(func() {
						if ctx.Err() == nil {
							a.Sizes = &next
						}
					})
				})
				if err != nil {
					win.Update(func() {
						if ctx.Err() == nil {
							a.Counting = false
							log.Print(err)
						}
					})
					return
				}
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
				list.watch.StartupFiles(append(detect.StartupFiles(scanner.Terminal(), a.Home), detect.KubeConfigFiles(scanner.Terminal(), a.Home)...))
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
			a.CancelPreparation()
			stopCatalog()
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
			list.settings.StartHidden = a.StartHidden
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
			if cancelSizes != nil {
				cancelSizes()
			}
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

// setFilter is the filter of a set's file in the open and save dialogs.
func setFilter() []mygo.FileFilter {
	return []mygo.FileFilter{{Name: locales.SetFileKind(), Extensions: []string{"json"}}}
}

// saveSet asks where to save a set and writes data there, giving done the
// file through win.Update; "" when the user canceled.
func saveSet(win *mygo.Window, data []byte, done func(path string, err error)) {
	go func() {
		// In Documents: without a folder macOS offers the disk's root,
		// where the user cannot write.
		dir, _ := mygo.App.Path(mygo.PathDocuments)
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: locales.SetSaveTitle(), DefaultPath: filepath.Join(dir, catalog.SetName), Filters: setFilter(), CreateDirectories: true,
		})
		if err == nil && path != "" {
			err = os.WriteFile(path, data, 0o644)
		}
		win.Update(func() { done(path, err) })
	}()
}

// openSet asks for a set's file and reads it, giving done the file and
// what it holds through win.Update; "" when the user canceled.
func openSet(win *mygo.Window, done func(path string, data []byte, err error)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: locales.SetOpenTitle(), Filters: setFilter()})
		if err != nil {
			log.Print(err)
		}
		if len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		win.Update(func() { done(paths[0], data, err) })
	}()
}

// askpass prints the password the user gives for why, as sudo's askpass
// program; it fails when the user would not give it.
func askpass(why string) int {
	password, err := ops.AskPassword(context.Background(), run.Exec{}, why)
	if err != nil {
		log.Print(err)
		return 1
	}
	fmt.Println(password)
	return 0
}
