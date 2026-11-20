package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"golang.org/x/text/collate"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
	"github.com/Blinks44/hearth/resources"
)

// App is the state of Hearth's main window.
type App struct {
	mcpPending bool // a requested MCP plan is being prepared, never confirmed
	// Machine is the caption under the logo, as "studio · macOS 15.4".
	Machine string
	// OS is the system for the report, without the computer's name, as
	// "macOS 15.4 (arm64)".
	OS string
	// ThemeMode is the saved appearance: "light", "dark" or "system".
	ThemeMode string
	// Locale is the interface language, a catalog of locales/ as "en".
	Locale string
	// collator sorts titles in collatorLocale, made again when Locale
	// changes.
	collator       *collate.Collator
	collatorLocale string
	// VersionLists allows version lists and package-manager availability queries; without
	// it, only the kept copies show.
	VersionLists bool
	// AppVersion is the build's version; "" in a development build.
	AppVersion string
	// CachePath is where the version lists are kept.
	CachePath string
	// CacheBusy is true while the version lists are being cleared.
	CacheBusy bool
	// Hidden are the keys of the issues worth knowing the user hid: they
	// leave the Issues page.
	Hidden []string
	// StartHidden keeps the introductory actions dismissed across launches.
	StartHidden bool
	// KubePinned are the Kubernetes kinds pinned above the cluster's
	// tables, by their short names; nil pins the default ones.
	KubePinned []string
	// Terminal and Editor are the apps chosen to open shells and files;
	// "" uses the system's. Terminals are those installed to choose from.
	Terminal, Editor string
	Terminals        []string
	// EditorApps are the common editors installed, beside the IDEs the
	// Editors page finds.
	EditorApps []run.App
	// OpenInEditor opens path in an editor's app or command, telling done
	// how it went; nil opens files with the system's apps only.
	OpenInEditor func(app, command, path string, done func(error))
	// OnSettings saves ThemeMode, VersionLists, Hidden, StartHidden,
	// KubePinned, Terminal and Editor after the user
	// changed them; OnLocale shows and keeps another language.
	OnSettings func()
	OnLocale   func(string)
	// ClearCache removes the kept version lists.
	ClearCache func()
	// SettingsError says why settings could not be saved or the cache
	// cleared; SettingsNotice confirms the cache was cleared.
	SettingsError  string
	SettingsNotice string
	// Home is the user's home directory, which paths show as ~.
	Home string
	// globalInstructions is the open Codex instructions preview; nil when none.
	globalInstructions *globalInstructionPreview
	// Snapshot is what the last scan found; nil before the first.
	Snapshot *inventory.Snapshot
	// Scanning reports a scan under way.
	Scanning bool
	// PackageUpdates overlays immutable local inventory with cached online results.
	PackageUpdates map[string]inventory.PackageManager
	PackageChecks  map[string]bool
	// Sizes are what the count of disk space found, kept from the last
	// one until the next ends; nil before the first. Counting reports a
	// count under way.
	Sizes    *inventory.Sizes
	Counting bool
	// History is what the last checks changed, the newest first.
	History []history.Entry
	// NodeVersions is what nodejs.org says of Node's versions; nil until
	// the list comes, or when it could not, NodeVersionsErr says why.
	NodeVersions    *versions.Node
	NodeVersionsErr string
	// JSVersions holds the latest stable package manager releases; nil
	// until fetched, with JSVersionsErr explaining a failed request.
	JSVersions    *versions.JSManagers
	JSVersionsErr string
	// BunLatest is Bun's newest stable version, as the npm registry says;
	// nil until it comes, or when it could not, BunLatestErr says why.
	BunLatest    *versions.Latest
	BunLatestErr string
	// GoVersions are Go's supported releases, as go.dev says; nil until
	// they come, or when they could not, GoVersionsErr says why.
	GoVersions    *versions.Go
	GoVersionsErr string
	// PythonVersions are Python's release lines; nil until they come, or
	// when they could not, PythonVersionsErr says why.
	PythonVersions    *versions.Python
	PythonVersionsErr string
	// PHPVersions are PHP's release lines; nil until they come, or when
	// they could not, PHPVersionsErr says why.
	PHPVersions    *versions.PHP
	PHPVersionsErr string
	// RustVersions are Rust's release lines; nil until they come, or when
	// they could not, RustVersionsErr says why.
	RustVersions      *versions.Rust
	RustVersionsErr   string
	DotnetVersions    *versions.Dotnet
	DotnetVersionsErr string
	RubyVersions      *versions.Ruby
	RubyVersionsErr   string
	JavaVersions      *versions.Java
	JavaVersionsErr   string
	// MiseOutdated is what mise says of newer versions of its tools
	// without a page of their own; nil until it comes, or without mise.
	MiseOutdated *versions.Mise
	// Shell reveals and opens files; nil hides those actions.
	Shell Shell
	// OnScan starts a scan, whose snapshot comes back through
	// Window.Update; nil disables scanning.
	OnScan func()
	// Projects are the folders the user added, as last read.
	Projects []projects.Project
	// ChooseFolder asks for a folder to add, and shows it with Offer
	// through Window.Update; nil hides adding.
	ChooseFolder func()
	// FindProjects looks for projects in the usual code folders, and shows
	// them with OfferFound through Window.Update; nil hides it.
	FindProjects func()
	// AddProjects adds the folders to the projects, and RemoveProject
	// takes one off the list, leaving the folder be.
	AddProjects   func(dirs []string)
	RemoveProject func(dir string)
	// KubeLinks are the projects' Kubernetes contexts by folder, and
	// LinkKube links one, or unlinks it when link is nil; nil hides
	// linking.
	KubeLinks    map[string]store.KubeLink
	LinkKube     func(dir string, link *store.KubeLink)
	PickManifest func(dir string, folder bool, done func(string, error))
	// LoadSkill reads a skill's folder, its files and SKILL.md, in the
	// background, and gives them to done through Window.Update; nil shows
	// neither.
	LoadSkill func(dir string, done func(entries []inventory.Entry, skillMD string, err error))
	// LoadCatalog downloads on request and publishes through Window.Update.
	// The returned function cancels a superseded request.
	LoadCatalog      func(repo skillcatalog.Repository, done func([]skillcatalog.Skill, map[string]string, error)) func()
	skillCatalogOpen bool
	skillCatalog     *skillCatalogState
	// CheckMCP starts or reaches an MCP server as its agent would, with
	// the terminal's environment, and asks for its tools, in the
	// background, giving done the answer through Window.Update; nil hides
	// the check.
	CheckMCP func(s inventory.MCPServer, done func(ai.MCPCheck, error))
	// CheckKube asks a context's cluster through kubectl, with the
	// terminal's environment, in the background, giving done the answer
	// through Window.Update; nil hides the check.
	CheckKube func(kubectl string, k inventory.KubeContext, done func(detect.KubeAccess))
	// WatchCluster reads the target's cluster now and again, giving each
	// read to got on the UI thread, until stop is called. ListNamespaces
	// asks the cluster for its namespaces once. Nil hides the cluster.
	WatchCluster func(t kube.Target, got func(kube.Cluster, error)) (stop func())
	// WatchDocker reads a local Docker engine now and again, giving each
	// read to got on the UI thread, until stop is called; nil hides the
	// engine view.
	WatchDocker           func(t docker.Target, got func(docker.Engine, error)) (stop func())
	WatchDockerContainers func(t docker.Target, got func([]docker.Container, error, error)) (stop func())
	WatchDockerImages     func(t docker.Target, got func([]docker.Image, error)) (stop func())
	ReadDockerDetail      func(docker.Target, string, func(docker.ContainerDetail, error)) (stop func())
	ReadDockerEnv         func(docker.Target, string, string, func([]byte, error)) (stop func())
	StreamDockerLogs      func(docker.Logs, func([]containerlog.LogLine), func(error)) (stop func())
	OpenDockerShell       func(docker.Target, string, func(error))
	ListNamespaces        func(t kube.Target, done func([]string, error)) (stop func())
	ListCustomDefinitions func(kube.Target, func(*kube.CustomDefinitions)) (stop func())
	// ReadSecretValue reads one selected key and delivers its bytes through Window.Update.
	ReadSecretValue  func(kube.Target, kube.Secret, string, func([]byte, error)) (stop func())
	ReadHelmHistory  func(kube.HelmTarget, func([]kube.HelmRevision, error)) (stop func())
	ReadHelmContent  func(kube.HelmTarget, int, bool, func(string, error)) (stop func())
	ReadResourceYAML func(kube.Target, kube.Resource, func(string, error)) (stop func())
	// SecretTimer schedules a callback on the UI thread and returns its cancellation.
	SecretTimer     func(time.Duration, func()) (stop func())
	secretClipboard *secretClipboard
	// StreamLogs reads a container's log in the background, giving the
	// lines as they come to got and how the read ended to done, on the UI
	// thread, until stop is called. Nil hides the logs.
	StreamLogs func(l kube.Logs, got func([]kube.LogLine), done func(error)) (stop func())
	// SaveLogs exports the shown lines to a file chosen by the user.
	SaveLogs func(name, text string, done func(path string, err error))
	// OpenKubeShell hands a container session to the system terminal.
	OpenKubeShell func(kube.Shell, func(error))
	// OpenTerminal opens the chosen terminal with the user's shell and
	// kubectl's environment, as for signing in to a cluster.
	OpenTerminal func(done func(error))

	CheckForwardPort func(port int, done func(int, error))
	RunForward       func(f ops.Forward, ready func(kube.ForwardPod), done func(error)) (stop func())

	// PlanPath makes, in the background, the plan that fixes dirs of
	// entries, the terminal's PATH, all its problems when dirs is nil, and
	// gives it to done through Window.Update; nil offers no fixes.
	PlanPath func(entries []inventory.PathEntry, dirs []string, done func(ops.Plan, error))

	// BuildPlan makes a plan in the background, as build does with the
	// terminal's environment and a way to check a new terminal, and gives
	// it to done through Window.Update; nil offers no such plans.
	BuildPlan func(build func(env sysenv.Env, terminal ops.Terminal) (ops.Plan, error), done func(ops.Plan, error))

	// RunPlan executes a plan the user confirmed in the background,
	// giving update how it goes through Window.Update, and returns what
	// stops it; nil shows plans without running them.
	RunPlan func(p ops.Plan, update func(ops.State)) (stop func())
	// JournalPath is the log of operations, which their dialog opens.
	JournalPath string
	// HearthExecutable is the installed binary offered as a read-only MCP server.
	HearthExecutable string
	// DetectTool finds a tool again, as a scan would, for an operation's
	// check; it runs on the operation's goroutine.
	DetectTool func(ctx context.Context, tool string) (inventory.ToolState, error)
	// CountDocker asks the Docker engine of s, with the terminal's env,
	// what pruning would free, for what a cleanup freed; it runs on the
	// operation's goroutine.
	CountDocker func(ctx context.Context, env sysenv.Env, s *inventory.Snapshot) ([]inventory.DockerUsage, error)
	// SkillBackups is the folder copies of skills move to when they are
	// merged; "" offers no merge.
	SkillBackups string
	// SaveSet asks where to save a set's file and writes data there, and
	// OpenSet asks for a set's file and reads it; both give done the file
	// chosen, "" when the user canceled, through Window.Update. nil hides
	// them.
	SaveSet func(data []byte, done func(path string, err error))
	OpenSet func(done func(path string, data []byte, err error))
	// DetectAgent finds an agent again, as a scan would, for an
	// operation's check; it runs on the operation's goroutine.
	DetectAgent func(ctx context.Context, id string) (inventory.Agent, error)
	// RereadPackages lists m's packages again, as the scan found m, for a
	// package operation's check, and drops m's kept online answer; it
	// runs on the operation's goroutine.
	RereadPackages func(ctx context.Context, m inventory.PackageManager) (inventory.PackageManager, error)

	preparing      *preparing
	op             *operation // the operation shown or running; nil when none
	setup          setupState // where the new computer wizard is
	search         searchState
	filterField    ui.Handle // the filter of the page shown, which / focuses
	router         *ui.Router
	notFoundOpen   bool   // the languages and tools not found show in the sidebar
	nodeTab        int    // the tab of the Node.js page shown
	jspmTab        int    // the tab of the package managers page shown
	jspmFilter     string // the package managers table's filter
	pathTab        int    // the tab of the PATH page shown
	pathFilter     string // the PATH table's filter
	pathFixErr     string // why the PATH problems asked to fix have no plan
	planErr        string // why a plan asked for could not be made, until a toast says it
	notice         string // what a toast says next, as a set saved
	packageTab     int    // the tab of the system package managers page shown
	packageManager string // the path of the package manager shown
	packageFilter  string // the package table's filter
	packageSort    int    // the package table's order: sortByName, sortByDependents or sortBySize
	// packageChosen are the updates checked to run, by manager path and
	// packageKey.
	packageChosen      map[string]bool
	gitTab             int    // the Git page's tab: installed, settings, PATH
	cppTab             int    // the C/C++ page's tab: installed, PATH
	modelsTab          int    // the local models page's tab: all, then an app each
	modelsFilter       string // the local models table's filter
	modelPull          string // the name of the Ollama model to download
	modelPullOpen      bool   // the dialog to download an Ollama model is open
	cliTab             int    // the CLI & clouds page's tab
	cliFilter          string // the CLI & clouds table's filter
	kubeTab            int    // the Kubernetes page's tab
	kubeFilter         string // the Kubernetes page's filter
	kubeNS             kubeNamespaceDialog
	kubeChecks         map[string]kubeCheck
	cluster            kubeClusterState
	engine             dockerEngineState // the Docker engine page's
	kind               kindCreateDialog
	kubeLink           kubeLinkDialog
	kubeApply          *kubeApplyDialog
	forwards           []*portForward
	portsFilter        string // the Ports table's filter
	instructionsQuery  string // the Instructions page's filter
	instructionsChosen string // the project shown on the Instructions page
	usageTab           int    // the Usage page's period, by usagePeriods
	mobileTab          int    // the mobile page's tab: all, then a group each
	mobileFilter       string // the mobile table's filter
	editorsTab         int    // the editors page's tab: all, extensions, commands
	editorsFilter      string // the editors table's filter
	changesTab         int    // the What changed page's tab: all, added, updated, removed
	changesFilter      string // the What changed table's filter
	problemsTab        int    // the Issues page's tab: all, then an area each
	problemsFilter     string // the Issues table's filter
	cleanupTab         int    // the Cleanup page's tab: all, then a group each
	cleanupFilter      string // the Cleanup table's filter
	// cleanupChoice is what the user checked or unchecked on the Cleanup
	// page, by candidateKey; the others are as Hearth offers them.
	cleanupChoice map[string]bool
	updatesTab    int    // the Updates page's tab: all, versions, packages
	updatesFilter string // the Updates table's filter
	// updateChoice is what the user checked or unchecked on the Updates
	// page, by updateRow.key; the others are as Hearth offers them.
	updateChoice   map[string]bool
	databaseTab    int     // the tab of the databases page shown
	databaseFilter string  // the databases table's filter
	dockerTab      int     // the tab of the Docker page shown
	dockerFilter   string  // the Docker contexts table's filter
	bunTab         int     // the tab of the Bun page shown
	goTab          int     // the tab of the Go page shown
	phpTab         int     // the tab of the PHP page shown
	pythonTab      int     // the tab of the Python page shown
	dotnetTab      int     // the tab of the .NET page shown
	rubyTab        int     // the tab of the Ruby page shown
	javaTab        int     // the tab of the Java page shown
	rustTab        int     // the tab of the Rust page shown
	projectsTab    int     // all projects, or the ones needing attention
	projectFilter  string  // the projects page's filter
	adding         *adding // the folder the add dialog shows; nil when closed
	addOpen        bool
	licensesOpen   bool // the licenses dialog from Settings

	// The skills and MCP pages: the agent filtered by, "" for all; the
	// scope, as scopeSegments; the search; and the key of the one shown.
	skillAgent, mcpAgent   string
	skillScope, mcpScope   int
	skillQuery, mcpQuery   string
	skillChosen, mcpChosen string
	skillFolder            skillFolder // what was read of the skill shown
	mcpCheck               mcpCheck    // the last check of an MCP server
	markdown               markdown.Options
}

// NewApp returns the app showing the overview.
func NewApp() *App {
	return &App{
		router: ui.NewRouter(overview.path), ThemeMode: "system", VersionLists: true, Locale: "en", usageTab: 1,
		engine: dockerEngineState{view: dockerContainers},
	}
}

// View builds the window: the sidebar and the page shown beside it.
func (a *App) View(c *ui.Context) {
	c.SetTheme(Theme(c.Theme()))
	a.syncCluster()
	a.syncDocker()
	a.syncDockerDetail(c)
	a.syncDockerLogs()
	a.syncHelmContent()
	a.syncCustomDefinitions()
	a.syncResourceYAML()
	a.syncSecrets(c)
	a.syncLogs()
	a.syncForwardForm()
	c.OnShortcut(ui.Cmd, ui.KeyK, a.toggleSearch)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.Scan)
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.router.Push(settings.path) })
	if a.router.Path() == kubeClusterPage {
		c.OnShortcut(ui.Cmd, ui.KeyJ, a.toggleClusterKinds)
	}
	if a.router.Path() == dockerEnginePage {
		c.OnShortcut(ui.Cmd, ui.KeyJ, a.toggleDockerViews)
		switch {
		case a.engine.viewsOpen:
			c.OnShortcut(0, ui.KeyEscape, func() { a.engine.viewsOpen = false })
		case a.engine.containers.logs != nil:
			c.OnShortcut(0, ui.KeyEscape, a.engine.containers.closeLogs)
		case a.engine.containers.detail.id != "" && (a.op == nil || !a.op.open):
			c.OnShortcut(0, ui.KeyEscape, a.engine.containers.closeDetail)
		}
	}
	if a.router.Path() == kubeClusterPage {
		switch {
		case a.cluster.kindsOpen:
			c.OnShortcut(0, ui.KeyEscape, func() { a.cluster.kindsOpen = false })
		case a.cluster.logs != nil:
			// Wherever the focus is, as the button that opened the log is gone.
			c.OnShortcut(0, ui.KeyEscape, a.closeLogs)
		case len(a.cluster.bulk.selected) > 0 && (a.op == nil || !a.op.open):
			c.OnShortcut(0, ui.KeyEscape, a.cluster.bulk.clear)
		}
	}
	// A focused text field types the slash; elsewhere it goes to the
	// page's filter.
	c.OnShortcut(0, ui.KeySlash, func() { a.filterField.Focus(c) })
	if a.planErr != "" {
		// Long enough to read, and closed sooner by its button.
		c.AddToast(ui.Toast{Title: a.planErr, Type: "error", Timeout: 10 * time.Second, Action: locales.OpClose(), OnAction: func() {}})
		a.planErr = ""
	}
	if a.notice != "" {
		c.Toast(a.notice)
		a.notice = ""
	}
	t := c.Theme()
	// Vibrancy shows wherever the view draws no background: under the
	// sidebar, which then draws none.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, vibrant)
		ui.Box(c).Width(1).Shrink(0).Background(Colors(t).Divider)
		a.router.View(c, func(r *ui.Route) {
			if r.Match(agentPage) {
				id := r.Query("id")
				r.Title(agentTitle(id))
				a.agentDetail(c, id)
				return
			}
			if r.Match(kubeClusterPage) {
				name := r.Query("context")
				r.Title(name)
				a.kubeClusterView(c, name)
				return
			}
			if r.Match(dockerEnginePage) {
				name := r.Query("context")
				r.Title(name)
				a.dockerEngineView(c, name)
				return
			}
			if r.Match(changesPage.path) {
				r.Title(changesPage.title())
				a.changesView(c, changesPage)
				return
			}
			if r.Match(projectPage) {
				dir := r.Query("dir")
				r.Title(filepath.Base(dir))
				a.project(c, dir)
				return
			}
			for _, p := range pages() {
				if r.Match(p.path) {
					r.Title(p.title())
					switch p.path {
					case settings.path:
						a.settingsPage(c)
					case setupPage:
						a.setupView(c)
					case "/tools/node":
						a.node(c, p)
					case "/tools/jspm":
						a.jspm(c, p)
					case pathPage:
						a.paths(c, p)
					case "/system/packages":
						a.packages(c, p)
					case "/tools/databases":
						a.databases(c, p)
					case gitPage:
						a.gitPageView(c, p)
					case cliPage:
						a.cliView(c, p)
					case kubePage:
						a.kubeView(c, p)
					case portsPage:
						a.portsView(c, p)
					case instructionsPage:
						a.instructionsView(c, p)
					case usagePage:
						a.usageView(c, p)
					case mobilePage:
						a.mobileView(c, p)
					case editorsPage:
						a.editorsView(c, p)
					case cppPage:
						a.cppView(c, p)
					case modelsPage:
						a.modelsView(c, p)
					case "/tools/docker":
						a.docker(c, p)
					case "/tools/bun":
						a.bun(c, p)
					case "/tools/go":
						a.goPage(c, p)
					case "/tools/php":
						a.php(c, p)
					case "/tools/python":
						a.python(c, p)
					case "/tools/dotnet":
						a.dotnet(c, p)
					case "/tools/ruby":
						a.ruby(c, p)
					case "/tools/java":
						a.javaPage(c, p)
					case "/tools/rust":
						a.rust(c, p)
					case projectsPage.path:
						a.projectList(c, p)
					case cleanupPage:
						a.cleanupView(c, p)
					case updatesPage:
						a.updatesView(c, p)
					case problemsPage:
						a.problemsView(c, p)
					case overview.path:
						a.overviewPage(c, p)
					case "/ai/agents":
						a.agentsPage(c, p)
					case skillsPage:
						a.skillsPage(c, p)
					case mcpPage:
						a.mcpPageView(c, p)
					default:
						a.stub(c, p)
					}
					return
				}
			}
			r.Title(overview.title())
			a.overviewPage(c, overview)
		}).Grow(1).MinWidth(0).Background(t.Background)
	})
	a.addDialog(c)
	a.searchDialog(c)
	a.preparationView(c)
	a.operationView(c)
	a.globalInstructionsDialog(c)
}

// Scan starts a scan, as the header and the sidebar do, unless one is
// under way. It runs on the UI thread.
func (a *App) Scan() {
	if a.OnScan != nil && !a.Scanning {
		a.Scanning = true
		a.OnScan()
	}
}

// stub is a page still to be built: its header and "Coming soon".
func (a *App) stub(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Icon(c, mustIcon(p.icon)).Size(28, 28).TextColor(Colors(c.Theme()).TextFaint)
			ui.Text(c, locales.ComingSoon()).TextColor(c.Theme().TextMuted)
		})
	})
}

// header is the top of a page: its title and subtitle, and on the right
// its actions. It drags the window, and keeps clear of the window
// controls on Windows and Linux.
func (a *App) header(c *ui.Context, title, subtitle string) {
	a.headerWith(c, title, subtitle, headerExtras{})
}

// headerExtras are what a page adds to its header: a button going back
// before the title, and actions before and after Scan.
type headerExtras struct {
	back, before, after func()
}

// headerWith is header with the page's extras.
func (a *App) headerWith(c *ui.Context, title, subtitle string, x headerExtras) {
	t := c.Theme()
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(8).Padding(0, right, 0, 28).
		BorderWidth(0, 0, 1, 0).BorderColor(Colors(t).Divider).DragWindow().Children(func() {
		if x.back != nil {
			headerBackButton(c, locales.BackToProjects(), x.back)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine()
			ui.Text(c, subtitle).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
		if x.before != nil {
			x.before()
		}
		if x.after != nil {
			x.after()
		}
	})
}

func headerBackButton(c *ui.Context, label string, back func()) {
	ui.Button(c.Key("back"), "").Label(label).Tooltip(label).
		Size(28, 26).Shrink(0).Padding(0).Margin(0, 4, 0, 0).OnClick(back).Children(func() {
		ui.Icon(c, Icon("chevron-left")).Size(14, 14).TextColor(c.Theme().TextMuted)
	})
}

var (
	logoOnce sync.Once
	// logo is the app icon in the sidebar, decoded once.
	logo ui.ImageSource
)

// sidebar is the navigation: the logo, the pages by section, and settings
// with the scan status at the bottom.
func (a *App) sidebar(c *ui.Context, vibrant bool) {
	t := c.Theme()
	pal := Colors(t)
	side := ui.Column(c).Width(244).Shrink(0).Padding(0, 10, 12, 10)
	if !vibrant {
		side.Background(pal.Sidebar)
	}
	side.Children(func() {
		// macOS puts the traffic lights in a row of their own above the
		// logo; Windows and Linux put their buttons over the header.
		if bar := c.TitleBar(); bar.Left > 0 {
			ui.Box(c).Height(52).Shrink(0).DragWindow()
		} else {
			ui.Box(c).Height(14).Shrink(0).DragWindow()
		}
		brand(c, a.Machine)
		a.searchButton(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				a.item(c, overview, nil)
				a.item(c, projectsPage, func() {
					if len(a.Projects) > 0 {
						ui.Text(c, strconv.Itoa(len(a.Projects))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					}
				})
				sectionTitle(c, locales.SectionMaintenance())
				a.item(c, problemsNav, a.problemsEnd(c))
				a.item(c, updatesNav, a.updatesEnd(c))
				a.item(c, cleanupNav, a.cleanupEnd(c))
				a.item(c, installNav, nil)
				missing := a.languages(c)
				for _, s := range sections {
					var shown []page
					for _, p := range s.pages {
						if a.missing(p) && !within(a.router.Path(), p.path) {
							missing = append(missing, p)
						} else {
							shown = append(shown, p)
						}
					}
					if len(shown) == 0 {
						continue
					}
					sectionTitle(c, s.title())
					for _, p := range shown {
						end := a.pageEnd(c, p)
						if a.missing(p) {
							end = notInstalled(c)
						}
						a.item(c, p, end)
					}
				}
				a.notFound(c, missing)
			})
		})
		ui.Box(c).Height(1).Shrink(0).Margin(12, 0, 4, 0).Background(pal.Divider)
		a.item(c, settings, nil)
		a.status(c)
	})
}

// pageEnd is what an item shows on its right: how many agents, skills or
// MCP servers were found on the AI pages', and the PATH's issues on its.
func (a *App) pageEnd(c *ui.Context, p page) func() {
	n := 0
	switch p.path {
	case "/ai/agents":
		n = len(a.foundAgents())
	case cliPage:
		n = a.cliCount()
	case mobilePage:
		n = a.mobileCount()
	case editorsPage:
		n = len(a.editorsState().Editors)
	case modelsPage:
		n = a.modelCount()
	case skillsPage:
		n = len(a.allSkills())
	case instructionsPage:
		n = len(a.Projects)
	case mcpPage:
		n = len(a.mcpItems())
	case "/tools/docker", "/tools/databases":
		for _, is := range a.issues(c) {
			if issuePage(is) == p.path {
				return func() { ui.Box(c).Size(6, 6).Radius(3).Background(c.Theme().Warning) }
			}
		}
		if p.path == "/tools/databases" {
			n = len(inventory.DatabaseServers(a.Snapshot))
		} else if a.dockerAnswers() {
			// The engine Docker goes to answers.
			return func() { ui.Box(c).Size(6, 6).Radius(3).Background(c.Theme().Success) }
		}
	case pathPage:
		return a.issueCount(c)
	case kubePage:
		// kubectl's context, as the design has it.
		if cur := kubeCurrent(a.cliState()); cur != nil {
			return func() {
				ui.Text(c, cur.Name).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine().MaxWidth(110)
			}
		}
	case gitPage:
		// The version the terminal runs, as the languages show theirs.
		if v := a.version("git"); v != "" {
			return func() {
				ui.Text(c, v).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			}
		}
	}
	if n == 0 {
		return nil
	}
	return func() {
		ui.Text(c, strconv.Itoa(n)).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	}
}

// issueCount shows how many issues the PATH has; nil for none.
func (a *App) issueCount(c *ui.Context) func() {
	n := 0
	for _, is := range a.issues(c) {
		if issuePage(is) == pathPage {
			n++
		}
	}
	return warningCount(c, n)
}

// status tells when the computer was last checked, with a button that
// checks it again.
func (a *App) status(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	dot, text := pal.TextFaint, locales.NotChecked()
	switch {
	case a.Scanning:
		dot, text = pal.Info, locales.Checking()
	case a.Snapshot != nil:
		dot, text = t.Success, checked(c, a.Snapshot.Taken)
	}
	ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(2, 2, 0, 9).Children(func() {
		// The status opens what the checks changed.
		status := ui.ButtonBase(c.Key("changes")).Row().Grow(1).MinWidth(0).Gap(7).AlignItems(ui.Center).Radius(6).
			Label(locales.ChangesTitle()).Tooltip(locales.ChangesTitle()).OnClick(func() { a.router.Push(changesPage.path) })
		textColor := pal.TextFaint
		if status.Hovered() {
			textColor = t.TextMuted
		}
		status.Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
			ui.Text(c, text).FontSize(11.5).TextColor(textColor).Grow(1).SingleLine()
		})
		icon, mode := themeLook(a.ThemeMode)
		theme := ui.ButtonBase(c.Key("theme")).Size(24, 24).Radius(6).Center().
			Label(locales.ThemeButton(mode)).Tooltip(locales.ThemeButton(mode)).
			OnClick(func() { a.ThemeMode = nextTheme(a.ThemeMode); a.savePreferences() })
		if theme.Hovered() {
			theme.Background(pal.SidebarHover)
		}
		theme.Children(func() {
			ui.Icon(c, Icon(icon)).Size(13, 13).TextColor(pal.TextFaint)
		})
		again := ui.ButtonBase(c.Key("scan-again")).Size(24, 24).Radius(6).Center().
			Label(locales.Scan()).Tooltip(locales.Scan() + " · " + cmdKey("R")).Disabled(a.OnScan == nil || a.Scanning).OnClick(a.Scan)
		if again.Hovered() {
			again.Background(pal.SidebarHover)
		}
		again.Children(func() { spinIcon(c, "refresh-cw", 12, pal.TextFaint, a.Scanning) })
	})
}

// themeLook is the icon of the appearance mode and its name.
func themeLook(mode string) (icon, name string) {
	switch mode {
	case "light":
		return "sun", locales.SettingsLight()
	case "dark":
		return "moon", locales.SettingsDark()
	}
	return "sun-moon", locales.SettingsSystem()
}

// nextTheme is the appearance after mode, as the sidebar's button turns
// through them: system, light, dark.
func nextTheme(mode string) string {
	switch mode {
	case "system":
		return "light"
	case "light":
		return "dark"
	}
	return "system"
}

// checked says how long ago taken was, and asks for a frame when the text
// would change.
func checked(c *ui.Context, taken time.Time) string {
	ago := c.Now().Sub(taken)
	switch {
	case ago < time.Minute:
		c.After(time.Minute - ago)
		return locales.CheckedJustNow()
	case ago < time.Hour:
		c.After(time.Minute - ago%time.Minute)
		return locales.CheckedMinutesAgo(int(ago / time.Minute))
	case ago < 24*time.Hour:
		c.After(time.Hour - ago%time.Hour)
		return locales.CheckedHoursAgo(int(ago / time.Hour))
	}
	return locales.CheckedOn(taken)
}

// languages is the "Languages & runtimes" section: the languages found
// and the page shown if it is another one. It returns the rest, which
// notFound lists.
func (a *App) languages(c *ui.Context) []page {
	var shown []language
	var rest []page
	for _, l := range slices.SortedFunc(slices.Values(languages), func(x, y language) int { return a.byTitle(x.title(), y.title()) }) {
		if a.version(l.id) != "" || within(a.router.Path(), l.path) {
			shown = append(shown, l)
		} else {
			rest = append(rest, l.page)
		}
	}
	if len(shown) == 0 {
		return rest
	}
	sectionTitle(c, locales.SectionLanguages())
	pal := Colors(c.Theme())
	for _, l := range shown {
		v := a.version(l.id)
		if v == "" {
			a.item(c, l.page, notInstalled(c))
			continue
		}
		a.item(c, l.page, func() {
			ui.Text(c, v).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		})
	}
	return rest
}

// notInstalled is the end of an item whose tool the scan did not find.
func notInstalled(c *ui.Context) func() {
	return func() {
		ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(Colors(c.Theme()).TextFaint).Tooltip(locales.NotInstalledHint())
	}
}

// missing reports that the last scan found nothing a tool page p shows,
// which then waits under "Not found". Pages that always have something
// to show, and every page before a scan, are never missing.
func (a *App) missing(p page) bool {
	if a.Snapshot == nil {
		return false
	}
	switch p.path {
	case "/tools/docker":
		if a.version("docker") != "" {
			return false
		}
		st, _ := a.Snapshot.Tool("docker")
		return st.Docker == nil || !slices.ContainsFunc(st.Docker.Engines, func(e inventory.DockerEngine) bool { return e.Path != "" })
	case kubePage:
		_, ok := cliCopy(a.cliState(), "kubectl")
		return !ok
	case "/tools/databases":
		return len(inventory.DatabaseServers(a.Snapshot)) == 0
	case cliPage:
		return a.cliCount() == 0
	case gitPage:
		return a.version("git") == ""
	case "/tools/jspm":
		return a.jsCount() == 0
	case editorsPage:
		return len(a.editorsState().Editors) == 0
	case mobilePage:
		return a.mobileCount() == 0
	case modelsPage:
		return len(a.modelApps()) == 0
	}
	return false
}

// dockerAnswers reports that the engine of Docker's selected context
// answered the last scan.
func (a *App) dockerAnswers() bool {
	st, _ := a.Snapshot.Tool("docker")
	return st.Docker != nil && slices.ContainsFunc(st.Docker.Contexts, func(ctx inventory.DockerContext) bool {
		return ctx.Name == st.Docker.Selected && ctx.Status == "available"
	})
}

// notFound is the "Not found" item at the end of the sections: it opens
// the languages and tools the scan did not find, pages kept by the order
// of their sections.
func (a *App) notFound(c *ui.Context, rest []page) {
	if len(rest) == 0 {
		return
	}
	pal := Colors(c.Theme())
	ui.Box(c).Height(8).Shrink(0)
	more := ui.ButtonBase(c.Key("not-found")).Expanded(a.notFoundOpen).Height(30).Shrink(0).Gap(9).Padding(0, 8).Radius(7).AlignItems(ui.Center)
	if more.Hovered() {
		more.Background(pal.SidebarHover)
	}
	more.Children(func() {
		ui.Icon(c, Icon("layers")).Size(16, 16).TextColor(pal.TextFaint)
		ui.Text(c, locales.NotFoundSection()).FontSize(13).TextColor(pal.TextFaint).Grow(1).SingleLine()
		ui.Text(c, strconv.Itoa(len(rest))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		chevron := "chevron-right"
		if a.notFoundOpen {
			chevron = "chevron-down"
		}
		ui.Icon(c, Icon(chevron)).Size(14, 14).TextColor(pal.TextFaint)
	})
	if more.Clicked() {
		a.notFoundOpen = !a.notFoundOpen
	}
	if a.notFoundOpen {
		ui.Column(c.Key("not-found-pages")).Gap(1).Shrink(0).Transition(rowMotion).Children(func() {
			for _, p := range rest {
				a.item(c, p, notInstalled(c))
			}
		})
	}
}

// version is the sidebar's version of a tool the last scan found, as
// 22.12: the one the terminal runs, else the first known; "…" when its
// version is unknown, and "" when the scan found no copy but stubs.
func (a *App) version(tool string) string {
	if a.Snapshot == nil {
		return ""
	}
	st, _ := a.Snapshot.Tool(tool)
	if !slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool { return !in.Stub }) {
		return ""
	}
	v := ""
	if i := inventory.TerminalCopy(st.Installations); i >= 0 {
		v = st.Installations[i].Version
	}
	for _, in := range st.Installations {
		if v == "" {
			v = in.Version
		}
	}
	if v == "" {
		return "…"
	}
	if major, rest, ok := strings.Cut(v, "."); ok {
		minor, _, _ := strings.Cut(rest, ".")
		return major + "." + minor
	}
	return v
}

// item is a sidebar entry going to page p; it stands out while p is
// shown. end, if not nil, builds what shows on its right.
func (a *App) item(c *ui.Context, p page, end func()) {
	t := c.Theme()
	pal := Colors(t)
	chosen := within(a.router.Path(), p.path)
	e := ui.ButtonBase(c.Key(p.path)).Transition(sidebarMotion).Height(30).Shrink(0).Gap(9).Padding(0, 8).Radius(7).AlignItems(ui.Center)
	iconColor := t.TextMuted
	switch {
	case chosen:
		e.Background(pal.SidebarChosen).InsetShadow(0, 0, 0, 1, ui.RGBA(0, 0, 0, 0.05)).Shadow(0, 1, 2, 0, pal.Shadow)
		iconColor = t.Accent
	case e.Hovered():
		e.Background(pal.SidebarHover)
	}
	e.Children(func() {
		ui.Icon(c, mustIcon(p.icon)).Size(16, 16).TextColor(iconColor)
		ui.Text(c, p.title()).FontSize(13).Grow(1).SingleLine()
		if end != nil {
			end()
		}
	})
	if e.Clicked() && !chosen {
		a.router.Push(a.entryPath(p.path))
	}
}

func sectionTitle(c *ui.Context, title string) {
	ui.Text(c.Key("section:"+title), title).Transition(sidebarMotion).FontSize(11).FontWeight(500).TextColor(Colors(c.Theme()).TextFaint).
		Padding(12, 8, 5, 8).SingleLine()
}

// brand is the logo, the app's name and the machine it looks at.
func brand(c *ui.Context, machine string) {
	logoOnce.Do(func() { logo, _ = ui.DecodeImage(resources.Logo) })
	pal := Colors(c.Theme())
	ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(2, 8, 14, 8).Shrink(0).Children(func() {
		ui.Box(c).Size(28, 28).Radius(7).Clip().Shadow(0, 2, 6, 0, ui.Hex("#C2410C").Alpha(0.25)).Children(func() {
			ui.Image(c, logo).Size(28, 28)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, "Hearth").FontSize(14).FontWeight(600).SingleLine()
			if machine != "" {
				ui.Text(c, machine).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
	})
}
