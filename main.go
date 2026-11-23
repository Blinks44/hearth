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
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/disk"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
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
	// sudo runs Hearth as its askpass program, for a plan's step, with
	// its prompt as the one argument.
	if self, _ := os.Executable(); len(os.Args) == 2 && self != "" && os.Getenv("SUDO_ASKPASS") == self {
		os.Exit(askpass(locales.AskpassPrompt()))
	}
	// Native UI updater window: needs no webview (no WebKitGTK / WebView2).
	mygo.Use(native.Plugin)
	if err := hearthui.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	a := hearthui.NewApp()
	a.Machine, a.OS = machine()
	a.Home, _ = os.UserHomeDir() // without it, paths show in full
	a.Shell = mygo.Shell
	a.Runner = run.Exec{}
	a.HearthExecutable, _ = os.Executable()
	ops.AskpassProgram = a.HearthExecutable
	scanner := &scan.Scanner{Runner: run.Exec{}, Detectors: scan.Detectors}
	list := &projectList{app: a, settings: store.DefaultSettings()}
	if dir, err := mygo.App.Path(mygo.PathUserData); err != nil {
		log.Print(err)
	} else {
		list.dir = dir
		a.DataDir = dir
		a.SkillBackups = filepath.Join(dir, "skill-backups")
		// A broken file is kept aside; Hearth starts with the defaults.
		if list.settings, err = store.LoadSettings(dir); err != nil {
			log.Print(err)
		}
	}
	a.ThemeMode, a.VersionLists, a.Hidden = list.settings.Theme, list.settings.VersionLists, list.settings.Hidden
	a.StartHidden = list.settings.StartHidden
	a.DockerPromoHidden = list.settings.DockerPromoHidden
	a.TrayEnabled = !list.settings.TrayDisabled
	a.ShowInDock = !list.settings.DockHidden || !a.TrayEnabled
	a.KubePinned, a.Terminal, a.Editor = list.settings.KubePinned, list.settings.Terminal, list.settings.Editor
	a.KubeLinks = list.kubeLinks()
	// The native parts follow it too: the sidebar's material, the window
	// controls, the menus and the macOS Dock icon policy.
	mygo.Theme.SetSource(mygo.ThemeSource(a.ThemeMode))
	forwardCtx, stopForwards := context.WithCancel(context.Background())
	dockerCtx, stopDocker := context.WithCancel(context.Background())
	var forwards, dockerWorkers sync.WaitGroup
	mygo.App.WhenReady(func() {
		a.Locale = mygo.App.Locale()
		a.AppVersion = mygo.App.Version()
		a.OpenAtLogin = mygo.App.OpenAtLogin()
		a.OnOpenAtLogin = mygo.App.SetOpenAtLogin
		a.CheckAppUpdate = updater.CheckForUpdates
		a.OnAppUpdateAutoCheck = updater.SetAutomaticChecks
		syncUpdater := func() {
			a.AppUpdateEnabled = mygo.Updater.Enabled()
			a.AppUpdateAvailable = updater.AvailableVersion()
			a.AppUpdateAutoCheck = updater.AutomaticChecks()
		}
		syncUpdater()
		var tray *trayController
		var openSettings func()
		a.OnLocale = func(tag string) {
			mygo.App.SetLocale(tag)
			setAppMenu(openSettings)
			if tray != nil {
				tray.refresh()
			}
		}
		quitting := false
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
			quitting = true
		})
		startHidden := a.TrayEnabled && mygo.App.WasOpenedAtLogin()
		syncDockIcon(a.TrayEnabled, a.ShowInDock, !startHidden)
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Hearth",
			Width:     1280,
			Height:    820,
			MinWidth:  1000,
			MinHeight: 680,
			Hidden:    startHidden,
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
		showMainWindow := func() {
			syncDockIcon(a.TrayEnabled, a.ShowInDock, true)
			win.Show()
			win.Focus()
		}
		openSettings = func() {
			win.Update(func() {
				a.OpenSettings()
				showMainWindow()
			})
		}
		setAppMenu(openSettings)
		offUpdater := updater.OnChange(func() {
			win.Update(func() {
				syncUpdater()
				if tray != nil {
					tray.refresh()
				}
			})
		})
		win.OnClosed(offUpdater)
		mygo.App.OnActivate(func(hasVisibleWindows bool) {
			if !hasVisibleWindows {
				showMainWindow()
			}
		})
		catalogCtx, stopCatalog := context.WithCancel(context.Background())
		skillCache := &skillcatalog.Cache{}
		a.LoadCatalog = func(repo skillcatalog.Repository, force bool, done func([]skillcatalog.Skill, map[string]string, error)) func() {
			ctx, cancel := context.WithTimeout(catalogCtx, 2*time.Minute)
			env := scanner.Terminal()
			go func() {
				defer cancel()
				skills, err := skillCache.Fetch(ctx, nil, repo, force)
				dirs := ai.SkillDirectories(env)
				win.Update(func() { done(skills, dirs, err) })
			}()
			return cancel
		}
		a.SearchSkillsSh = func(query string, done func([]skillcatalog.SearchResult, error)) func() {
			ctx, cancel := context.WithTimeout(catalogCtx, 20*time.Second)
			go func() {
				defer cancel()
				results, err := skillcatalog.SearchSkillsSh(ctx, nil, query, 20)
				if ctx.Err() != nil {
					return
				}
				win.Update(func() { done(results, err) })
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
					showMainWindow()
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
		a.ReadInstruction = func(path string, done func(string, error)) {
			go func() {
				text, err := ai.InstructionText(path)
				win.Update(func() { done(text, err) })
			}()
		}
		a.DiffSkills = func(dirA, dirB string, done func([]ai.SkillFileDiff, error)) {
			go func() {
				diffs, err := ai.SkillDiff(dirA, dirB)
				win.Update(func() { done(diffs, err) })
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
		a.CheckCloudAuth = func(tool, bin string, done func(detect.CloudAuth)) {
			env := scanner.Terminal()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), detect.CloudAuthTimeout)
				defer cancel()
				res := detect.CheckCloudAuth(ctx, run.Exec{}, tool, bin, env)
				win.Update(func() { done(res) })
			}()
		}
		a.ListGcloudProjects = func(bin string, done func([]string, error)) {
			env := scanner.Terminal()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), detect.CloudAuthTimeout)
				defer cancel()
				res, err := detect.ListGcloudProjects(ctx, run.Exec{}, bin, env)
				win.Update(func() { done(res, err) })
			}()
		}
		a.OpenCloudLogin = func(argv []string, done func(error)) {
			env, terminal, hint := scanner.Terminal(), a.Terminal, locales.KubeShellCloseHint()
			go func() {
				err := run.OpenTerminal(context.Background(), run.Exec{}, terminal, argv, env, env.Get("PATH"), "", hint)
				win.Update(func() { done(err) })
			}()
		}
		// The cluster view's reads keep their sign-in and connection.
		api := kube.NewAPI()
		a.WatchCluster = func(t kube.Target, got func(kube.Cluster, error)) func() {
			env := scanner.Terminal()
			t.API = api
			ctx, cancel := context.WithCancel(context.Background())
			go kube.Watch(ctx, run.Exec{}, env, t, func(c kube.Cluster, err error) {
				win.Update(func() { got(c, err) })
			})
			return cancel
		}
		a.WatchDocker = func(t docker.Target, overview bool, got func(docker.Engine, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				docker.Watch(ctx, run.Exec{}, env, t, overview, func(e docker.Engine, err error) {
					win.Update(func() { got(e, err) })
				})
			})
			return cancel
		}
		a.WatchDockerContainers = func(t docker.Target, got func([]docker.Container, error, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				docker.WatchContainers(ctx, run.Exec{}, env, t, func(items []docker.Container, err, statsErr error) {
					win.Update(func() { got(items, err, statsErr) })
				})
			})
			return cancel
		}

		a.WatchDockerResources = func(t docker.Target, kind docker.ResourceKind, got func([]docker.Resource, error, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				docker.WatchResources(ctx, run.Exec{}, env, t, kind, func(items []docker.Resource, err, sizeErr error) {
					win.Update(func() { got(items, err, sizeErr) })
				})
			})
			return cancel
		}
		a.WatchDockerImages = func(t docker.Target, got func([]docker.Image, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				docker.WatchImages(ctx, run.Exec{}, env, t, func(items []docker.Image, err error) {
					win.Update(func() { got(items, err) })
				})
			})
			return cancel
		}

		a.ReadDockerDetail = func(t docker.Target, id string, done func(docker.ContainerDetail, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				d, err := docker.ReadContainerDetail(ctx, run.Exec{}, env, t, id)
				win.Update(func() {
					if ctx.Err() == nil {
						done(d, err)
					}
				})
			})
			return cancel
		}
		a.ReadDockerEnv = func(t docker.Target, id, key string, done func([]byte, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				value, err := docker.ReadContainerEnv(ctx, run.Exec{}, env, t, id, key)
				if ctx.Err() != nil {
					clear(value)
					return
				}
				win.Update(func() {
					if ctx.Err() != nil {
						clear(value)
						return
					}
					done(value, err)
				})
			})
			return cancel
		}
		a.StreamDockerLogs = func(l docker.Logs, got func([]containerlog.LogLine), done func(error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(dockerCtx)
			dockerWorkers.Go(func() {
				err := l.Stream(ctx, run.CombinedLines, env, func(batch []containerlog.LogLine) {
					if ctx.Err() == nil {
						win.Update(func() {
							if ctx.Err() == nil {
								got(batch)
							}
						})
					}
				})
				if ctx.Err() == nil {
					win.Update(func() {
						if ctx.Err() == nil {
							done(err)
						}
					})
				}
			})
			return cancel
		}
		a.OpenDockerShell = func(t docker.Target, id string, done func(error)) {
			env, terminal, hint := scanner.Terminal(), a.Terminal, locales.KubeShellCloseHint()
			go func() {
				ctx := context.Background()
				cmd, err := docker.ShellCommand(ctx, run.Exec{}, env, t, id)
				if err == nil {
					err = run.OpenTerminal(ctx, run.Exec{}, terminal, cmd.Args, cmd.Env, env.Get("PATH"), env.Get("KUBECONFIG"), hint, "DOCKER_CONTEXT=")
				}
				win.Update(func() { done(err) })
			}()
		}

		a.OpenDockerLogin = func(argv []string, done func(error)) {
			env, terminal, hint := scanner.Terminal(), a.Terminal, locales.KubeShellCloseHint()
			go func() {
				err := run.OpenTerminal(context.Background(), run.Exec{}, terminal, argv, env, env.Get("PATH"), "", hint)
				win.Update(func() { done(err) })
			}()
		}

		a.OpenDBClient = func(argv []string, done func(error)) {
			env, terminal, hint := scanner.Terminal(), a.Terminal, locales.KubeShellCloseHint()
			go func() {
				err := run.OpenTerminal(context.Background(), run.Exec{}, terminal, argv, env, env.Get("PATH"), "", hint)
				win.Update(func() { done(err) })
			}()
		}

		a.SecretTimer = func(delay time.Duration, done func()) func() {
			timer := time.AfterFunc(delay, func() { win.Update(done) })
			return func() { timer.Stop() }
		}
		a.ReadSecretValue = func(t kube.Target, secret kube.Secret, key string, done func([]byte, error)) func() {
			env := scanner.Terminal()
			t.API = api
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				value, err := kube.ReadSecretValue(ctx, run.Exec{}, env, t, secret, key)
				if ctx.Err() != nil {
					clear(value)
					return
				}
				win.Update(func() {
					if ctx.Err() != nil {
						clear(value)
						return
					}
					done(value, err)
				})
			}()
			return cancel
		}
		a.StreamLogs = func(l kube.Logs, got func([]kube.LogLine), done func(error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				err := kube.Stream(ctx, run.Lines, env, l, func(lines []kube.LogLine) {
					if ctx.Err() != nil {
						return
					}
					win.Update(func() {
						if ctx.Err() == nil {
							got(lines)
						}
					})
				})
				if ctx.Err() == nil {
					win.Update(func() {
						if ctx.Err() == nil {
							done(err)
						}
					})
				}
			}()
			return cancel
		}
		a.Terminals, a.EditorApps = run.Terminals(), run.EditorApps()
		a.OpenInEditor = func(app, command, path string, done func(error)) {
			env := scanner.Terminal()
			go func() {
				err := run.OpenWith(context.Background(), run.Exec{}, app, command, path, env)
				win.Update(func() { done(err) })
			}()
		}
		a.OpenTerminal = func(done func(error)) {
			env := scanner.Terminal()
			shell := env.Get("SHELL")
			if shell == "" {
				shell = "/bin/sh"
			}
			terminal := a.Terminal
			go func() {
				err := run.OpenTerminal(context.Background(), run.Exec{}, terminal, []string{shell, "-l"}, env, env.Get("PATH"), env.Get("KUBECONFIG"), "")
				win.Update(func() { done(err) })
			}()
		}
		a.OpenKubeShell = func(s kube.Shell, done func(error)) {
			env := scanner.Terminal()
			closeHint := locales.KubeShellCloseHint()
			terminal := a.Terminal
			go func() {
				err := run.OpenTerminal(context.Background(), run.Exec{}, terminal, s.Args(), env, env.Get("PATH"), env.Get("KUBECONFIG"), closeHint)
				win.Update(func() { done(err) })
			}()
		}
		a.ListCustomDefinitions = func(t kube.Target, done func(*kube.CustomDefinitions)) func() {
			env := scanner.Terminal()
			t.API = api
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				defs := kube.ListCustomDefinitions(ctx, run.Exec{}, env, t)
				if ctx.Err() == nil {
					win.Update(func() { done(defs) })
				}
			}()
			return cancel
		}
		a.ReadHelmHistory = func(t kube.HelmTarget, done func([]kube.HelmRevision, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				history, err := kube.HelmHistory(ctx, run.Exec{}, env, t, "")
				if ctx.Err() == nil {
					win.Update(func() { done(history, err) })
				}
			}()
			return cancel
		}
		a.ReadHelmContent = func(t kube.HelmTarget, revision int, manifest bool, done func(string, error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				text, err := kube.HelmContent(ctx, run.Exec{}, env, t, "", revision, manifest)
				if ctx.Err() == nil {
					win.Update(func() { done(text, err) })
				}
			}()
			return cancel
		}
		a.ReadResourceYAML = func(t kube.Target, resource kube.Resource, done func(string, error)) func() {
			env := scanner.Terminal()
			t.API = api
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				yaml, err := kube.ReadResourceYAML(ctx, run.Exec{}, env, t, resource)
				if ctx.Err() == nil {
					win.Update(func() { done(yaml, err) })
				}
			}()
			return cancel
		}
		a.CheckForwardPort = func(port int, done func(int, error)) {
			go func() {
				suggestion, err := ops.CheckForwardPort(port)
				win.Update(func() { done(suggestion, err) })
			}()
		}
		a.RunForward = func(f ops.Forward, ready func(kube.ForwardPod), done func(error)) func() {
			env := scanner.Terminal()
			ctx, cancel := context.WithCancel(forwardCtx)
			forwards.Go(func() {
				defer cancel()
				err := ops.RunForward(ctx, run.Exec{}, run.Lines, env, f, func(pod kube.ForwardPod) {
					win.Update(func() { ready(pod) })
				})
				if forwardCtx.Err() == nil {
					win.Update(func() { done(err) })
				}
			})
			return cancel
		}
		a.ListNamespaces = func(t kube.Target, done func([]string, error)) func() {
			env := scanner.Terminal()
			t.API = api
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				names, err := kube.Namespaces(ctx, run.Exec{}, env, t)
				if ctx.Err() != nil {
					return
				}
				win.Update(func() {
					if ctx.Err() == nil {
						done(names, err)
					}
				})
			}()
			return cancel
		}
		a.SaveSet = func(data []byte, done func(string, error)) { saveSet(win, data, done) }
		a.SaveLogs = func(name, text string, done func(string, error)) { saveLogs(win, name, text, done) }
		a.OpenSet = func(done func(string, []byte, error)) { openSet(win, done) }
		a.PickManifest = func(dir string, folder bool, done func(string, error)) { pickManifest(win, dir, folder, done) }
		a.OpenFolder = func(path string, done func(error)) {
			go func() {
				var err error
				if _, statErr := os.Stat(path); statErr != nil {
					err = statErr
				} else {
					err = mygo.Shell.OpenPath(path)
				}
				win.Update(func() { done(err) })
			}()
		}
		a.PickPath = func(title string, directory bool, done func(string, error)) {
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: directory})
				var chosen string
				if len(paths) > 0 {
					chosen = paths[0]
				}
				win.Update(func() { done(chosen, err) })
			}()
		}
		// Operations run one at a time in the background, as the user's
		// own commands would in the terminal, and append all their output
		// to the operations log.
		if logs, err := mygo.App.Path(mygo.PathLogs); err != nil {
			log.Print(err)
		} else {
			a.LogsDir = logs
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
			Env: scanner.Terminal,
		}
		list.env = scanner.Terminal
		list.watch.StartupFiles(append(detect.StartupFiles(sysenv.GUI(), a.Home), detect.KubeConfigFiles(scanner.Terminal(), a.Home)...))
		list.read()
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Print(err)
		}
		tray = newTrayController(win, a, scanner.Terminal, func() {
			quitting = true
			mygo.App.Quit()
		})
		tray.sync(a.TrayEnabled)
		var fetchPackages func(force bool)
		// Disk space is counted after each scan, apart from it: a walk of
		// big caches takes long. Each result replaces the previous count.
		var cancelSizes context.CancelFunc
		countSizes := func(s *inventory.Snapshot) {
			if cancelSizes != nil {
				cancelSizes()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelSizes = cancel
			var prevDocker []inventory.DockerUsage
			if a.Sizes != nil && len(a.Sizes.Docker) > 0 {
				prevDocker = slices.Clone(a.Sizes.Docker)
			}
			a.Counting = true
			a.DockerCounting = true
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
						if ctx.Err() != nil {
							return
						}
						if len(next.Docker) > 0 || next.DockerErr != "" {
							a.DockerCounting = false
						} else if a.DockerCounting && len(prevDocker) > 0 {
							next.Docker = slices.Clone(prevDocker)
						}
						a.Sizes = &next
					})
				})
				if err != nil {
					win.Update(func() {
						if ctx.Err() == nil {
							a.Counting = false
							a.DockerCounting = false
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
						a.Sizes, a.Counting, a.DockerCounting = &sizes, false, false
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
				a.ClearCloudAuth()
				if tray != nil {
					tray.refresh()
					tray.refreshContainers()
				}
				if fetchPackages != nil {
					fetchPackages(false)
				}
				countSizes(&s)
				list.watch.StartupFiles(append(detect.StartupFiles(scanner.Terminal(), a.Home), detect.KubeConfigFiles(scanner.Terminal(), a.Home)...))
				list.watch.Docker(&s, scanner.Terminal())
				if len(list.dirs()) > 0 {
					list.read()
				}
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
		skillCache.Dir = filepath.Join(cache, "skillcatalog")
		usageCache := filepath.Join(cache, "usage-cache.json")
		if cache == "" {
			lists.Dir = filepath.Join(os.TempDir(), "hearth-versions")
			skillCache.Dir = filepath.Join(os.TempDir(), "hearth-skillcatalog")
			usageCache = filepath.Join(os.TempDir(), "hearth-usage-cache.json")
		}
		scanner.UsageCache = usageCache
		a.RereadPackages = func(ctx context.Context, m inventory.PackageManager) (inventory.PackageManager, error) {
			env := scanner.Terminal()
			if err := pkgmgr.Forget(lists, env, m); err != nil {
				log.Print(err)
			}
			return pkgmgr.Reread(ctx, run.Exec{}, env, m)
		}
		var cancelVersions context.CancelFunc
		var cancelPackages context.CancelFunc
		fetchPackages = func(force bool) {
			if cancelPackages != nil {
				cancelPackages()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelPackages = cancel
			if force {
				ctx = versions.WithForce(ctx)
			}
			lists.SetOffline(!a.VersionLists)
			// The last check's results stay until the new one's come: the
			// packages and what they hold do not drop out meanwhile.
			if a.PackageUpdates == nil {
				a.PackageUpdates = map[string]inventory.PackageManager{}
			}
			a.PackageChecks = map[string]bool{}
			fetchMise(ctx, win, lists, a, scanner.Terminal())
			fetchAgents(ctx, win, lists, a)
			if a.Snapshot == nil {
				return
			}
			st, ok := a.Snapshot.Tool("packages")
			if !ok {
				return
			}
			managers := slices.Clone(st.PackageManagers)
			env := slices.Clone(scanner.Terminal())
			for _, m := range managers {
				if !pkgmgr.HasNetworkQuery(m.ID) {
					continue
				}
				a.PackageChecks[m.Path] = true
				mgr := m
				go func() {
					if ctx.Err() != nil {
						return
					}
					queryCtx, stop := context.WithTimeout(ctx, 2*time.Minute)
					result := pkgmgr.Network(queryCtx, lists, run.Exec{}, env, mgr)
					stop()
					win.Update(func() {
						if ctx.Err() != nil {
							return
						}
						a.PackageUpdates[mgr.Path] = result
						delete(a.PackageChecks, mgr.Path)
					})
				}()
			}
		}
		fetchVersions := func(force bool) {
			if cancelVersions != nil {
				cancelVersions()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancelVersions = cancel
			if force {
				ctx = versions.WithForce(ctx)
			}
			lists.SetOffline(!a.VersionLists)
			a.VersionChecks = map[string]bool{}
			fetchList(ctx, win, lists, a.VersionChecks, "node", versions.FetchNode, &a.NodeVersions, &a.NodeVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "js", versions.FetchJSManagers, &a.JSVersions, &a.JSVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "bun", versions.FetchBun, &a.BunLatest, &a.BunLatestErr)
			fetchList(ctx, win, lists, a.VersionChecks, "go", versions.FetchGo, &a.GoVersions, &a.GoVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "python", versions.FetchPython, &a.PythonVersions, &a.PythonVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "php", versions.FetchPHP, &a.PHPVersions, &a.PHPVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "rust", versions.FetchRust, &a.RustVersions, &a.RustVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "dotnet", versions.FetchDotnet, &a.DotnetVersions, &a.DotnetVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "ruby", versions.FetchRuby, &a.RubyVersions, &a.RubyVersionsErr)
			fetchList(ctx, win, lists, a.VersionChecks, "java", versions.FetchJava, &a.JavaVersions, &a.JavaVersionsErr)
		}
		a.CheckUpdates = func() {
			if !a.VersionLists || a.Snapshot == nil {
				return
			}
			fetchVersions(true)
			fetchPackages(true)
		}
		a.RecountSizes = func() {
			if a.Snapshot != nil && !a.Counting {
				countSizes(a.Snapshot)
			}
		}
		var cancelPorts context.CancelFunc
		a.RefreshPorts = func(done func(detect.PortsSnapshot)) {
			if cancelPorts != nil {
				cancelPorts()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cancelPorts = cancel
			env := scanner.Terminal()
			snap := a.Snapshot
			go func() {
				defer cancel()
				res := detect.RefreshPorts(ctx, run.Exec{}, env, snap)
				if ctx.Err() != nil {
					return
				}
				win.Update(func() { done(res) })
			}()
		}
		a.CachePath = lists.Dir
		win.OnClose(func(e *mygo.CloseEvent) {
			a.ClearSecretClipboard()
			if !quitting && a.TrayEnabled {
				e.PreventDefault()
				win.Hide()
				syncDockIcon(a.TrayEnabled, a.ShowInDock, false)
			}
		})
		win.OnClosed(func() {
			if tray != nil {
				tray.destroy()
			}
			a.CancelPreparation()
			a.StopCluster()
			a.StopDocker()
			stopForwards()
			stopCatalog()
			if cancelPorts != nil {
				cancelPorts()
			}
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
			if !a.TrayEnabled {
				a.ShowInDock = true
			}
			list.settings.Theme, list.settings.VersionLists, list.settings.Hidden = a.ThemeMode, a.VersionLists, a.Hidden
			list.settings.StartHidden = a.StartHidden
			list.settings.DockerPromoHidden = a.DockerPromoHidden
			list.settings.TrayDisabled = !a.TrayEnabled
			list.settings.DockHidden = !a.ShowInDock
			list.settings.KubePinned, list.settings.Terminal, list.settings.Editor = a.KubePinned, a.Terminal, a.Editor
			mygo.Theme.SetSource(mygo.ThemeSource(a.ThemeMode))
			if tray != nil {
				tray.sync(a.TrayEnabled)
			}
			prevPolicy := mygo.App.ActivationPolicy()
			syncDockIcon(a.TrayEnabled, a.ShowInDock, win.IsVisible())
			if mygo.App.ActivationPolicy() != prevPolicy && win.IsVisible() {
				win.Show()
				win.Focus()
			}
			list.save()
			if onlineChanged {
				fetchVersions(false)
				fetchPackages(false)
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
			a.VersionChecks = nil
			a.AgentChecking = false
			a.MiseChecking = false
			a.SettingsNotice = ""
			if cancelVersions != nil {
				cancelVersions()
			}
			stopCatalog()
			go func() {
				err := errors.Join(lists.Clear(), skillCache.Clear(), ai.ClearUsageCache(usageCache))
				win.Update(func() {
					a.CacheBusy = false
					if err != nil {
						a.SettingsError = locales.SettingsCacheError(err.Error())
						return
					}
					a.ClearSkillCatalogCache()
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
					a.AgentVersions = nil
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
					a.AgentVersionsErr = ""
					a.SettingsError = ""
					a.SettingsNotice = locales.SettingsCleared()
					fetchVersions(false)
					fetchPackages(false)
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
			a.AgentChecking = false
			a.MiseChecking = false
			scanner.Start(scanned)
			fetchVersions(false)
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
							if tray != nil {
								tray.refresh()
								tray.refreshContainers()
							}
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
		fetchVersions(false)
	})
	err := mygo.App.Run()
	stopForwards()
	stopDocker()
	forwards.Wait()
	dockerWorkers.Wait()
	if err != nil {
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
		a.MiseOutdatedErr = ""
		a.MiseChecking = false
		return
	}
	a.MiseChecking = true
	a.MiseOutdatedErr = ""
	env = slices.Clone(env)
	go func() {
		m, err := versions.FetchMise(ctx, lists, run.Exec{}, env, found[0], installed)
		if err != nil && !errors.Is(err, versions.ErrOffline) && ctx.Err() == nil {
			log.Print(err)
		}
		win.Update(func() {
			if ctx.Err() != nil {
				return
			}
			a.MiseChecking = false
			switch {
			case errors.Is(err, versions.ErrOffline):
				a.MiseOutdatedErr = locales.VersionsOffline()
				if len(m.Tools) > 0 {
					a.MiseOutdated = &m
				}
			case err != nil:
				a.MiseOutdatedErr = err.Error()
				if len(m.Tools) > 0 {
					a.MiseOutdated = &m
				}
			default:
				a.MiseOutdated = &m
				a.MiseOutdatedErr = ""
			}
		})
	}()
}

// fetchAgents asks the npm registry in the background for the latest
// release of installed npm-packaged AI agents and puts the answer in
// a.AgentVersions on the UI thread; a failed ask keeps the previous one.
func fetchAgents(ctx context.Context, win *mygo.Window, lists *versions.Cache, a *hearthui.App) {
	all := catalog.AgentNpmPackages()
	pkgs := map[string]string{}
	if a.Snapshot != nil {
		for _, ag := range a.Snapshot.Agents {
			if len(ag.Installations) == 0 {
				continue
			}
			if pkg, ok := all[ag.ID]; ok {
				pkgs[ag.ID] = pkg
			}
		}
	}
	if len(pkgs) == 0 {
		a.AgentVersions = nil
		a.AgentVersionsErr = ""
		a.AgentChecking = false
		return
	}
	a.AgentChecking = true
	a.AgentVersionsErr = ""
	go func() {
		agents, err := versions.FetchAgents(ctx, lists, pkgs)
		if err != nil && !errors.Is(err, versions.ErrOffline) && ctx.Err() == nil {
			log.Print(err)
		}
		win.Update(func() {
			if ctx.Err() != nil {
				return
			}
			a.AgentChecking = false
			switch {
			case errors.Is(err, versions.ErrOffline):
				a.AgentVersionsErr = locales.VersionsOffline()
				if len(agents.Latest) > 0 {
					a.AgentVersions = &agents
				}
			case err != nil:
				a.AgentVersionsErr = err.Error()
				if uerr, ok := errors.AsType[*url.Error](err); ok {
					if u, perr := url.Parse(uerr.URL); perr == nil {
						a.AgentVersionsErr = locales.VersionsUnreachable(u.Host)
					}
				}
				if len(agents.Latest) > 0 {
					a.AgentVersions = &agents
				}
			default:
				a.AgentVersions = &agents
				a.AgentVersionsErr = ""
			}
		})
	}()
}

// fetchList gets a version list through lists in the background and
// puts it in *dst on the UI thread; when it cannot, the list shown before
// stays and *errText says why.
func fetchList[T any](ctx context.Context, win *mygo.Window, lists *versions.Cache, checks map[string]bool, key string, fetch func(context.Context, *versions.Cache) (T, error), dst **T, errText *string) {
	checks[key] = true
	go func() {
		list, err := fetch(ctx, lists)
		if err != nil && !errors.Is(err, versions.ErrOffline) {
			log.Print(err)
		}
		win.Update(func() {
			if ctx.Err() != nil {
				return
			}
			delete(checks, key)
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

func saveLogs(win *mygo.Window, name, text string, done func(string, error)) {
	go func() {
		dir, _ := mygo.App.Path(mygo.PathDocuments)
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: locales.KubeLogsSave(), DefaultPath: filepath.Join(dir, name),
			Filters:           []mygo.FileFilter{{Name: locales.KubeLogsFileKind(), Extensions: []string{"log"}}},
			CreateDirectories: true,
		})
		if err == nil && path != "" {
			if writeErr := store.WriteFile(filepath.Dir(path), filepath.Base(path), []byte(text)); writeErr != nil {
				err = fmt.Errorf("write log: %w", writeErr)
			}
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

func pickManifest(win *mygo.Window, dir string, folder bool, done func(string, error)) {
	go func() {
		opts := mygo.OpenDialogOptions{Parent: win, DefaultPath: dir, Directory: folder, Title: locales.KubeApplyChooseFolder()}
		if !folder {
			opts.Title = locales.KubeApplyChooseFile()
			opts.Filters = []mygo.FileFilter{{Name: locales.KubeApplySource(), Extensions: []string{"yaml", "yml", "json"}}}
		}
		paths, err := mygo.Dialog.Open(opts)
		var path string
		if len(paths) > 0 {
			path = paths[0]
		}
		win.Update(func() { done(path, err) })
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
