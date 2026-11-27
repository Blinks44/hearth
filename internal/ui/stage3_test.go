package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestProjectScriptsRunnerAndLogs(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, "dev", "web-app")

	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Listeners: []inventory.DatabaseListener{
						{
							Port:        "5173",
							Address:     "127.0.0.1",
							PID:         "4208",
							PPID:        "4205",
							PGID:        "4201",
							Process:     "node",
							Args:        []string{"node", "node_modules/.bin/vite"},
							CPUPercent:  2.3,
							MemoryBytes: 128 * 1024 * 1024,
							Own:         true,
							Dirs:        []string{projDir},
						},
					},
				},
			},
		},
	}
	a.Projects = []projects.Project{
		{
			Dir: projDir,
			Scripts: []projects.Script{
				{Name: "dev", Source: "package.json", Command: "vite", Argv: []string{"npm", "run", "dev"}},
				{Name: "test", Source: "Makefile", Command: "go test ./...", Argv: []string{"make", "test"}},
			},
		},
	}

	var emitStart func(int)
	var emitLines func([]containerlog.LogLine)
	var emitDone func(error)
	stopped := false
	a.RunScript = func(spec ops.ScriptSpec, onStart func(int), onLines func([]containerlog.LogLine), done func(error)) func() {
		emitStart = onStart
		emitLines = onLines
		emitDone = done
		return func() {
			stopped = true
			if emitDone != nil {
				emitDone(nil)
			}
		}
	}

	a.OpenProjectServices(projDir)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()

	hasTexts(t, tt, locales.ProjectScriptsTitle(), "dev", "package.json", "test", "Makefile")

	// Start the first script ("dev").
	if err := tt.Click(locales.ProjectScriptsRun()); err != nil {
		t.Fatalf("click Run: %v", err)
	}
	tt.Frame()
	if emitStart == nil || emitLines == nil {
		t.Fatal("RunScript was not invoked")
	}

	emitStart(4201)
	emitLines([]containerlog.LogLine{{Text: "VITE v6.0 ready in 120 ms"}})
	tt.Frame()

	// Grandchild listener on :5173 with matching PGID 4201 merges into managed run without duplicating.
	hasTexts(t, tt,
		locales.ProjectRunningTitle(),
		locales.ProjectScriptsRunning(),
		locales.ProjectStatusRunning("5173"),
		":5173",
	)
	if len(a.scriptListeners(a.scriptRuns[0], a.portRows())) != 1 {
		t.Fatalf("expected PGID grandchild listener to match managed run")
	}

	// Narrow window (1000x680) must render cleanly without panicking or dropping key labels.
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Frame()
	hasTexts(t, ttNarrow, locales.ProjectRunningTitle(), locales.ProjectScriptsTitle(), "dev")

	// Clicking Logs opens the dedicated full-screen Script Logs sub-view.
	if err := tt.Click(locales.ProjectScriptsLogs()); err != nil {
		t.Fatalf("click Logs: %v", err)
	}
	tt.Frame()
	hasTexts(t, tt, "VITE v6.0 ready in 120 ms", locales.ProjectScriptsRunning(), locales.ProjectScriptsStop())

	// Stop the managed script directly from the full-screen logs view.
	if err := tt.Click(locales.ProjectScriptsStop()); err != nil {
		t.Fatalf("click Stop: %v", err)
	}
	tt.Frame()
	if !stopped {
		t.Fatal("expected managed script stop function to be called")
	}
}

func TestPortsDevServerAndForwardDedup(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, "dev", "shop")

	a := NewApp()
	a.Home = home
	a.Projects = []projects.Project{{Dir: projDir}}
	a.forwards = []*portForward{
		{
			target: ops.Forward{Context: "dev", Namespace: "default", Kind: "service", Name: "api", Local: 9090, Remote: 80},
			pod:    kube.ForwardPod{Name: "api-0", Port: 80, PID: 7777},
			stop:   func() {},
		},
	}
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Listeners: []inventory.DatabaseListener{
						{
							Port:        "5173",
							Address:     "127.0.0.1",
							PID:         "1111",
							Process:     "node",
							Args:        []string{"node", "node_modules/.bin/vite"},
							CPUPercent:  1.8,
							MemoryBytes: 150 * 1024 * 1024,
							Own:         true,
							Dirs:        []string{projDir},
						},
						{
							Port:    "5432",
							Address: "127.0.0.1",
							PID:     "2222",
							Process: "postgres",
							Own:     true,
						},
						{
							Port:    "9090",
							Address: "127.0.0.1",
							PID:     "7777",
							Process: "kubectl",
							Args:    []string{"kubectl", "port-forward", "svc/api", "9090:80"},
							Own:     true,
						},
					},
				},
			},
		},
	}

	a.OpenPorts()
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()

	// Managed kubectl forward PID 7777 must be deduplicated from the table rows.
	rows := a.portRows()
	for _, r := range rows {
		if r.PID == "7777" {
			t.Fatalf("expected managed kubectl forward PID 7777 to be deduplicated, still in rows: %+v", rows)
		}
	}

	hasTexts(t, tt, "Vite", "5173", "5432", locales.PortsTabDevServers(), "1.8%")

	// Switch to Dev servers tab: Vite stays, postgres disappears.
	if err := tt.Click(locales.PortsTabDevServers()); err != nil {
		t.Fatalf("click Dev servers tab: %v", err)
	}
	tt.Frame()
	hasTexts(t, tt, "Vite", "5173")
	if tt.HasText("5432") {
		t.Fatal("postgres port 5432 should be hidden under Dev servers tab")
	}

	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Frame()
	hasTexts(t, ttNarrow, "Vite", "5173")
}

func TestAgentRadarAndHooks(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, "dev", "hearth")

	a := NewApp()
	a.Home = home
	a.HearthExecutable = "/Applications/Hearth.app/Contents/MacOS/Hearth"
	a.Projects = []projects.Project{{Dir: projDir}}
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Dir: filepath.Join(home, ".claude"), Installations: []inventory.Installation{{Tool: "claude", Path: "/opt/homebrew/bin/claude", OnPath: 1}}},
			{ID: "codex", Dir: filepath.Join(home, ".codex"), Installations: []inventory.Installation{{Tool: "codex", Path: "/opt/homebrew/bin/codex", OnPath: 1}}},
		},
	}
	a.RadarHooksInstalled = map[string]bool{"codex": true}
	a.RadarSessions = []inventory.ActiveAgentSession{
		{
			Agent:     "claude",
			Status:    inventory.AgentStatusWaiting,
			Exact:     true,
			Cwd:       projDir,
			Model:     "claude-sonnet-4",
			Tokens:    12500,
			CostUSD:   0.42,
			PID:       "5150",
			UpdatedAt: time.Now(),
		},
		{
			Agent:     "codex",
			Status:    inventory.AgentStatusWorking,
			Exact:     false,
			Cwd:       home,
			PID:       "5151",
			UpdatedAt: time.Now(),
		},
	}

	var builtPlan ops.Plan
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(nil, nil)
		builtPlan = p
		done(p, err)
	}

	a.OpenAgents()
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()

	hasTexts(t, tt,
		locales.RadarCardTitle(),
		locales.RadarStatusWaiting(),
		locales.RadarStatusWorking(),
		locales.RadarExactHint(),
		locales.RadarPassiveHint(),
		locales.RadarHooksPrivacyLine(),
		locales.RadarHooksInstalled(),
		locales.RadarHooksConnect(),
		"hearth",
		"$0.42",
	)

	// Narrow window check.
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Frame()
	hasTexts(t, ttNarrow, locales.RadarCardTitle(), locales.RadarStatusWaiting(), "hearth")

	// Clicking Connect hooks… opens the agent popover menu and builds a FileEdit plan for Claude.
	if err := tt.Click(locales.RadarHooksConnect()); err != nil {
		t.Fatalf("click Connect hooks: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Claude Code · " + locales.RadarHooksInstall()); err != nil {
		t.Fatalf("choose Claude Code hooks install: %v", err)
	}
	tt.Frame()
	if len(builtPlan.Steps) == 0 || builtPlan.Steps[0].Edit == nil || !strings.HasSuffix(filepath.ToSlash(builtPlan.Steps[0].Edit.Path), "/.claude/settings.json") {
		t.Fatalf("expected Claude settings.json hook plan, got %+v", builtPlan)
	}
}

func TestTrayPopoverView(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, "dev", "portal")

	// Empty state (no Docker, no agents, no servers).
	emptyApp := NewApp()
	emptyApp.Home = home
	ttEmpty := ui.NewTester(func(c *ui.Context) {
		emptyApp.TrayPopoverView(c, TrayPopoverActions{
			OnOpenHearth: func() {},
			OnOpenPorts:  func() {},
		})
	}, trayPopoverWidth, 580)
	ttEmpty.Frame()
	hasTexts(t, ttEmpty,
		locales.TrayPopoverEmpty(),
		locales.TrayPopoverEmptyBody(),
		locales.TrayPopoverAllServices(),
		locales.TrayPopoverRightClickHint(),
	)

	// Idle state with Colima installed shows Colima toggle in the footer even when nothing is running.
	colimaIdleApp := NewApp()
	colimaIdleApp.Home = home
	ttColimaIdle := ui.NewTester(func(c *ui.Context) {
		colimaIdleApp.TrayPopoverView(c, TrayPopoverActions{
			HasColima:     true,
			ColimaRunning: false,
			OnOpenHearth:  func() {},
			OnOpenPorts:   func() {},
		})
	}, trayPopoverWidth, 580)
	ttColimaIdle.Frame()
	hasTexts(t, ttColimaIdle, locales.TrayPopoverEmpty(), locales.TrayPopoverColimaStoppedShort())

	// Busy state with all sections, unified running services, upward full-width context dropdowns, Colima toggle, and bounds in en & ru.
	a := NewApp()
	a.Home = home
	a.Projects = []projects.Project{{Dir: projDir}}
	a.RadarSessions = []inventory.ActiveAgentSession{
		{Agent: "claude", Status: inventory.AgentStatusWaiting, Exact: true, Cwd: projDir, CostUSD: 0.42, UpdatedAt: time.Now()},
	}
	a.scriptRuns = []*scriptRun{
		{
			key:     scriptRunKey(projDir, "package.json", "dev"),
			dir:     projDir,
			script:  projects.Script{Name: "dev", Source: "package.json"},
			pid:     9001,
			running: true,
			stop:    func() {},
		},
	}
	a.forwards = []*portForward{
		{
			target: ops.Forward{Context: "dev", Name: "web", Local: 8081, Remote: 80},
			stop:   func() {},
		},
	}
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Servers: []inventory.Database{
						{Kind: "PostgreSQL", Service: "postgresql@17", Status: "running", Source: "brew", Ports: []string{"5432"}},
					},
					Listeners: []inventory.DatabaseListener{
						{Port: "3000", Address: "127.0.0.1", PID: "9002", Process: "node", Args: []string{"node", "next-server"}, Own: true, Dirs: []string{projDir}},
					},
				},
			},
		},
	}

	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	for _, loc := range []string{"en", "ru"} {
		locale := loc
		i18n.SetLocaleFunc(func() string { return locale })

		var switchedDocker, switchedKube string
		var colimaToggledTo *bool
		var checkedUpdates bool
		var openedPorts bool
		runningCt := docker.Container{ID: "c1", Name: "/api-1", Image: "nginx", Ports: "0.0.0.0:8080->80/tcp"}
		runningCt.State.Status = "running"
		tt := ui.NewTester(func(c *ui.Context) {
			a.TrayPopoverView(c, TrayPopoverActions{
				Containers:            []docker.Container{runningCt},
				HasColima:             true,
				ColimaRunning:         true,
				DockerContext:         "colima",
				DockerContexts:        []string{"colima", "desktop-linux"},
				KubeContext:           "kind-dev",
				KubeContexts:          []string{"kind-dev", "prod"},
				AppUpdateAvailable:    "1.2.0",
				OnRefresh:             func() {},
				OnOpenSettings:        func() {},
				OnOpenHearth:          func() {},
				OnOpenPorts:           func() { openedPorts = true },
				OnCheckAppUpdate:      func() { checkedUpdates = true },
				OnOpenContainers:      func() {},
				OnToggleColima:        func(start bool) { v := start; colimaToggledTo = &v },
				OnSwitchDockerContext: func(name string) { switchedDocker = name },
				OnSwitchKubeContext:   func(name string) { switchedKube = name },
			})
		}, trayPopoverWidth, 620)
		tt.Frame()
		hasTexts(t, tt,
			"Hearth",
			"Next.js",
			"api-1",
			"PostgreSQL",
			"localhost:8081 → :80",
			"colima",
			"kind-dev",
			locales.TrayPopoverColimaRunningShort(),
			locales.TrayPopoverAllServices(),
			locales.AppUpdateAvailable("1.2.0"),
		)
		// AI agent radar sessions belong on the main AI -> Agents page, not in the tray popover.
		if tt.HasText("Claude Code · portal") || tt.HasText(locales.TrayPopoverWaitingChip(1)) {
			t.Fatalf("[%s] tray popover should not render agent radar rows: %q", locale, tt.Texts())
		}

		// Footer links and context selects must never overflow the 444px flyout width.
		for _, label := range []string{locales.TrayPopoverAllServices(), locales.AppUpdateAvailable("1.2.0"), "Docker: colima", "K8s: kind-dev"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("[%s] missing footer control %q", locale, label)
			}
			if r.X < 0 || r.X+r.W > trayPopoverWidth {
				t.Fatalf("[%s] footer control %q clipped: rect=%+v (max width %d)", locale, label, r, trayPopoverWidth)
			}
		}

		// Native downward menu for Docker context switching.
		if err := tt.Click("Docker: colima"); err != nil {
			t.Fatalf("[%s] click Docker context select: %v", locale, err)
		}
		tt.Frame()
		if err := tt.ChooseMenuItem("desktop-linux"); err != nil {
			t.Fatalf("[%s] choose Docker context desktop-linux: %v", locale, err)
		}
		tt.Frame()
		if switchedDocker != "desktop-linux" {
			t.Fatalf("[%s] expected switched Docker context desktop-linux, got %q", locale, switchedDocker)
		}

		// Native downward menu for K8s context switching.
		if err := tt.Click("K8s: kind-dev"); err != nil {
			t.Fatalf("[%s] click K8s context select: %v", locale, err)
		}
		tt.Frame()
		if err := tt.ChooseMenuItem("prod"); err != nil {
			t.Fatalf("[%s] choose K8s context prod: %v", locale, err)
		}
		tt.Frame()
		if switchedKube != "prod" {
			t.Fatalf("[%s] expected switched K8s context prod, got %q", locale, switchedKube)
		}

		// Colima status button triggers stop when running.
		if err := tt.Click(locales.TrayColimaStop()); err != nil {
			t.Fatalf("[%s] click Colima status: %v", locale, err)
		}
		tt.Frame()
		if colimaToggledTo == nil || *colimaToggledTo {
			t.Fatalf("[%s] expected Colima toggle to request stop (false), got %v", locale, colimaToggledTo)
		}

		// Footer links open Services & Ports and update check.
		if err := tt.Click(locales.TrayPopoverAllServices()); err != nil {
			t.Fatalf("[%s] click All services link: %v", locale, err)
		}
		if !openedPorts {
			t.Fatalf("[%s] expected OnOpenPorts to be called", locale)
		}
		if err := tt.Click(locales.AppUpdateAvailable("1.2.0")); err != nil {
			t.Fatalf("[%s] click update link: %v", locale, err)
		}
		if !checkedUpdates {
			t.Fatalf("[%s] expected OnCheckAppUpdate to be called", locale)
		}
	}
}

func TestManagedScriptDetachedListenerAndRaycastExclusion(t *testing.T) {
	home := t.TempDir()
	monoDir := filepath.Join(home, "dev", "moduix")
	docsDir := filepath.Join(monoDir, "apps", "docs")
	now := time.Now()

	a := NewApp()
	a.Home = home
	a.Projects = []projects.Project{
		{
			Dir: monoDir,
			Scripts: []projects.Script{
				{Name: "dev:docs", Source: "package.json", Command: "turbo run dev -F moduix-docs", Argv: []string{"pnpm", "run", "dev:docs"}},
			},
		},
	}
	a.scriptRuns = []*scriptRun{
		{
			key:       scriptRunKey(monoDir, "package.json", "dev:docs"),
			dir:       monoDir,
			script:    projects.Script{Name: "dev:docs", Source: "package.json"},
			pid:       27716,
			running:   true,
			startedAt: now.Add(-15 * time.Second),
			stop:      func() {},
		},
	}
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Listeners: []inventory.DatabaseListener{
						// Detached worker in a monorepo subfolder (e.g. spawned via turbo daemon with different PGID).
						{
							Port:        "3100",
							Address:     "127.0.0.1",
							PID:         "27803",
							PPID:        "1",
							PGID:        "27803",
							Process:     "node",
							Args:        []string{"node", "docs/server.js"},
							MemoryBytes: 512 * 1024 * 1024,
							Own:         true,
							StartedAt:   now.Add(-10 * time.Second),
							Dirs:        []string{docsDir},
						},
						// Older pre-existing terminal dev server in the same repo must not be swallowed.
						{
							Port:      "4000",
							Address:   "127.0.0.1",
							PID:       "19001",
							Process:   "node",
							Args:      []string{"node", "node_modules/.bin/vite"},
							Own:       true,
							StartedAt: now.Add(-10 * time.Minute),
							Dirs:      []string{monoDir},
						},
						// Raycast Backend helper on :7265 must never appear as a dev server.
						{
							Port:    "7265",
							Address: "127.0.0.1",
							PID:     "963",
							Process: "node",
							Args:    []string{"Raycast", "Backend"},
							Own:     true,
						},
					},
				},
			},
		},
	}

	rows := a.portRows()
	matched := a.scriptListeners(a.scriptRuns[0], rows)
	if len(matched) != 1 || matched[0].Port != "3100" {
		t.Fatalf("expected only detached monorepo worker :3100 to link to dev:docs, got %+v", matched)
	}

	// Overview card shows :3100 merged into dev:docs with icon buttons (open, logs, restart, stop).
	a.OpenProject(monoDir)
	ttOverview := ui.NewTester(a.View, 1280, 820)
	ttOverview.Frame()
	hasTexts(t, ttOverview, "dev:docs", "localhost:3100", "Vite", "localhost:4000")
	if _, ok := ttOverview.Find(locales.ProjectScriptsLogs()); !ok {
		t.Fatalf("expected Logs icon button on Project Overview")
	}
	if ttOverview.HasText("7265") {
		t.Fatalf("Raycast Backend :7265 must not appear on Project Overview")
	}

	// Tray flyout excludes Raycast Backend :7265 and does not duplicate :3100.
	ttTray := ui.NewTester(func(c *ui.Context) {
		a.TrayPopoverView(c, TrayPopoverActions{
			OnOpenHearth: func() {},
			OnOpenPorts:  func() {},
		})
	}, trayPopoverWidth, 580)
	ttTray.Frame()
	hasTexts(t, ttTray, "moduix · dev:docs", ":3100")
	if ttTray.HasText("7265") || ttTray.HasText("Raycast") {
		t.Fatalf("Raycast Backend :7265 must not appear in Tray flyout")
	}
}

func TestTrayContextPickerView(t *testing.T) {
	var picked string
	items := []string{"colima", "desktop-linux", "default"}
	tt := ui.NewTester(func(c *ui.Context) {
		TrayContextPickerView(c, "colima", items, func(choice string) {
			picked = choice
		})
	}, 200, 140)
	tt.Frame()
	hasTexts(t, tt, "colima", "desktop-linux", "default")
	if err := tt.Click("desktop-linux"); err != nil {
		t.Fatalf("click desktop-linux: %v", err)
	}
	if picked != "desktop-linux" {
		t.Fatalf("expected picked=desktop-linux, got %q", picked)
	}
}

func TestProjectUXPolish(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{
		Tool: "docker",
		Docker: &inventory.DockerState{
			Selected: "colima",
			Contexts: []inventory.DockerContext{{Name: "colima", Local: true, Status: "available"}},
		},
	})
	a.RunScript = func(ops.ScriptSpec, func(int), func([]containerlog.LogLine), func(error)) func() {
		return func() {}
	}
	dir := home + "/dev/planeta"
	p := projects.Project{
		Dir:   dir,
		Files: []string{"package.json", "Makefile", "compose.yaml", "compose.dev.yaml", "compose.override.yaml"},
		Git: &projects.GitInfo{
			Branch:    "main",
			Remote:    "github.com/acme/planeta",
			RemoteURL: "https://github.com/acme/planeta",
		},
		Scripts: []projects.Script{
			{Name: "dev", Source: "package.json", Command: "pnpm dev"},
			{Name: "dev", Source: "Makefile", Command: "docker compose up"},
			{Name: "test", Source: "package.json", Command: "vitest"},
		},
	}
	a.Projects = []projects.Project{p}
	a.router.Push(projectPath(dir))

	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Duplicate quick-run scripts show distinct short source badges, and both
	// standalone compose files appear in the redesigned Docker Compose card.
	hasTexts(t, tt, "pkg", "make", "compose.yaml", "compose.dev.yaml", "+ compose.override.yaml")

	// Switch to Run & services tab and verify source filter pills work.
	if err := tt.Click(locales.ProjectTabRunServices()); err != nil {
		t.Fatalf("switch to Run & services: %v", err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "package.json", "Makefile", "vitest", "docker compose up")
	if err := tt.Click("Makefile"); err != nil {
		t.Fatalf("click Makefile source filter: %v", err)
	}
	tt.Frame()
	if tt.HasText("vitest") {
		t.Fatalf("package.json script should be hidden when Makefile source filter is active")
	}
	hasTexts(t, tt, "docker compose up")

	// Compact preparation loading dialog must not stretch Cancel across the whole dialog.
	a.preparing = &preparing{dir: dir, open: true, cancel: func() {}}
	tt.Frame()
	hasTexts(t, tt, locales.PrepareChecking(), locales.Cancel())
	cancelNode, ok := tt.Find(locales.Cancel())
	if !ok {
		t.Fatal("Cancel button not found in preparation dialog")
	}
	if cancelNode.W > 180 {
		t.Fatalf("Cancel button width = %.1f, want compact right-aligned button (<= 180px)", cancelNode.W)
	}
}
