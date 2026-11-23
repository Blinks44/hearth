package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// Block 1 (Docker): icon order, retained problem chip across engine tabs,
// Restart in container detail header, and Open in Cleanup on disk card.
func TestS14DockerPolish(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = colimaSnapshot()
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home}, nil))
	}
	a.WatchDocker = (&engineWatches{}).watch
	a.StreamDockerLogs = func(docker.Logs, func([]containerlog.LogLine), func(error)) func() { return func() {} }
	a.OpenDockerShell = func(docker.Target, string, func(error)) {}
	var receive func([]docker.Container, error, error)
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		receive = got
		return func() {}
	}
	a.engine.view = dockerContainers
	a.router.Push(dockerEnginePath("colima"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	eng := colimaEngine()
	eng.Overview.Disk.Usage = []inventory.DockerUsage{
		{Type: "Images", Bytes: 8 << 30, Reclaimable: 2 << 30},
	}
	a.engine.data = &eng
	running := docker.Container{
		ID:      strings.Repeat("a", 64),
		Name:    "shop-web-1",
		Image:   "nginx",
		Compose: docker.ComposeProject{Name: "shop", Dir: home + "/work/shop"},
	}
	running.State.Status = "running"
	crashed := docker.Container{
		ID:      strings.Repeat("b", 64),
		Name:    "shop-api-1",
		Image:   "node",
		Compose: running.Compose,
	}
	crashed.State.Status = "exited"
	crashed.State.ExitCode = 137
	crashed.State.OOMKilled = true
	killed := docker.Container{
		ID:      strings.Repeat("c", 64),
		Name:    "shop-worker-1",
		Image:   "node",
		Compose: running.Compose,
	}
	killed.State.Status = "exited"
	killed.State.ExitCode = 137
	receive([]docker.Container{running, crashed, killed}, nil, nil)
	tt.Advance(time.Second)

	// Container row actions keep variable Shell on the left and constant
	// Start/Stop, Logs, More in fixed right-aligned columns.
	screenshot(t, tt, "s14-docker-containers-1280")
	shellBox, okShell := tt.Find(locales.KubeShellAction())
	stopBox, okStop := tt.Find(ops.DockerStop.Label() + "…")
	startBox, okStart := tt.Find(ops.DockerStart.Label() + "…")
	logsBox, okLogs := tt.Find(locales.KubeLogsOpen())
	moreBox, okMore := tt.Find(locales.MoreActions())
	if !okShell || !okStop || !okStart || !okLogs || !okMore {
		t.Fatalf("missing container row action icons: shell=%v stop=%v start=%v logs=%v more=%v", okShell, okStop, okStart, okLogs, okMore)
	}
	if !(shellBox.X < stopBox.X && stopBox.X < logsBox.X && logsBox.X < moreBox.X) {
		t.Errorf("container action icon order wrong: shell=%.0f stop=%.0f logs=%.0f more=%.0f", shellBox.X, stopBox.X, logsBox.X, moreBox.X)
	}
	if stopBox.X != startBox.X {
		t.Errorf("Start/Stop column drifted across rows: stop=%.0f start=%.0f", stopBox.X, startBox.X)
	}

	// D5: container detail header has Restart button for a running container.
	a.engine.containers.selectContainer(running.ID)
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-docker-detail-1280")
	if !tt.HasText(ops.DockerRestart.Label()) {
		t.Fatalf("Restart button missing in container detail header: %q", tt.Texts())
	}

	// D4: switching away from Containers keeps the Containers problem counter
	// (only OOMKilled 137 counts as a problem; plain SIGKILL 137 does not).
	a.engine.view = dockerOverview
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-docker-overview-1280")
	if a.engine.problemCount != 1 || !tt.HasText("1") {
		t.Fatalf("Containers problem badge lost on Overview tab: count=%d texts=%q", a.engine.problemCount, tt.Texts())
	}
	// D6: disk card in Overview offers Open in Cleanup and navigates there.
	if err := tt.Click(locales.ChangesOpenCleanup()); err != nil {
		t.Fatalf("click Open in Cleanup: %v", err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage {
		t.Fatalf("expected cleanup page, got %q", a.router.Path())
	}
}

// Block 2 (Kubernetes): Back from logs returns to pod, Check access shown at 1
// context, NotReady node clickable in Overview, primary Open context in header.
func TestS14KubernetesPolish(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.CheckKube = func(_ string, _ inventory.KubeContext, done func(detect.KubeAccess)) {
		done(detect.KubeAccess{})
	}
	// Reduce to a single context and point current-context at it to verify K2 & K4.
	for i := range a.Snapshot.Tools {
		if a.Snapshot.Tools[i].Tool == "cli" && a.Snapshot.Tools[i].Cli != nil {
			cli := a.Snapshot.Tools[i].Cli
			cli.KubeContexts = cli.KubeContexts[:1]
			for j := range cli.Contexts {
				if cli.Contexts[j].Tool == "kubectl" {
					cli.Contexts[j].Name = cli.KubeContexts[0].Name
				}
			}
		}
	}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-kube-entry-1280")
	hasTexts(t, tt, locales.KubeCheckAccess(), locales.DockerOpenContext("kind-dev"))

	// K1 & K3: Overview NotReady node opens Nodes and selects it; Back from logs returns to pod.
	a.openCluster("kind-dev")
	tt.Frame()
	tt.Advance(time.Second)
	cl := shopCluster()
	cl.Overview = &kube.Overview{
		Nodes: []kube.Node{
			{Name: "kind-worker", Version: "v1.37.0", Ready: false},
		},
	}
	if err := tt.Click("Cluster overview"); err != nil {
		t.Fatal(err)
	}
	w.got[len(w.got)-1](cl, nil)
	tt.Frame()
	if err := tt.Click("kind-worker"); err != nil {
		t.Fatalf("NotReady node not clickable in Overview: %v", err)
	}
	tt.Frame()
	if a.cluster.kind != kindNodes || a.cluster.resourceLink.name != "kind-worker" {
		t.Fatalf("expected Nodes view linking kind-worker, got kind=%v link=%+v", a.cluster.kind, a.cluster.resourceLink)
	}

	// Open logs from a pod and go back -> must return to that pod, not clear selection.
	pod := cl.Pods[0]
	a.cluster.kind = kindPods
	a.cluster.pod = pod.UID
	a.openLogs(pod, pod.Containers[0].Name, false)
	tt.Frame()
	if err := tt.Click(locales.KubeLogsBack()); err != nil {
		t.Fatalf("click Back from logs: %v", err)
	}
	tt.Frame()
	if a.cluster.logs != nil || a.cluster.kind != kindPods || a.cluster.pod != pod.UID {
		t.Fatalf("Back from logs lost pod context: logs=%v kind=%v pod=%q", a.cluster.logs, a.cluster.kind, a.cluster.pod)
	}
}

// Block 3 (Projects): all requirement sources visible, inline Install for
// unversioned tools & Terraform capitalization, row click navigates while
// Install does not, and AI+Files cards stack vertically at 1000x680.
func TestS14ProjectsPolish(t *testing.T) {
	orig := goos
	goos = "darwin"
	t.Cleanup(func() { goos = orig })
	a := NewApp()
	a.Home = home
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home}, nil))
	}
	dir := filepath.Join(home, "work", "web-app")
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "node", Managers: []inventory.Manager{{Name: "fnm"}}, Installations: []inventory.Installation{
			{Tool: "node", Version: "22.12.0", OnPath: 1, Path: home + "/.fnm/node", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}},
		}},
		{Tool: "terraform"},
		{Tool: "git"},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}}},
	}}
	a.Projects = []projects.Project{{
		Dir:   dir,
		Files: []string{".nvmrc", "package.json", "versions.tf", ".git"},
		Requirements: []projects.Requirement{
			{Tool: "node", Want: "22", File: ".nvmrc"},
			{Tool: "node", Want: ">=20", File: "package.json", Key: "engines.node"},
			{Tool: "terraform", Want: "~> 1.9", File: "versions.tf"},
			{Tool: "git", File: ".git"},
		},
	}}
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// P4: both .nvmrc and package.json sources appear in "Project needs".
	screenshot(t, tt, "s14-project-1280")
	hasTexts(t, tt, ".nvmrc", "package.json", "Terraform")
	// P5: unversioned missing tool (git) gets inline Install button.
	installLabel := locales.PkgInstallTitle("Git")
	if _, ok := tt.Find(installLabel); !ok {
		t.Fatalf("missing inline Install for Git (%q): %q", installLabel, tt.Texts())
	}

	// P7: clicking Install proposes operation without navigating away from project.
	if err := tt.Click(installLabel); err != nil {
		t.Fatalf("click Install Git: %v", err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != projectPage {
		t.Fatalf("Install navigated away from project to %q", a.router.Path())
	}
	if a.op == nil {
		t.Fatal("Install Git did not open operation dialog")
	}
	a.op = nil
	tt.Frame()

	// P7: clicking the Node requirement row navigates to /tools/node.
	if err := tt.Click("Node.js"); err != nil {
		t.Fatalf("click Node.js requirement row: %v", err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/tools/node" {
		t.Fatalf("clicking Node.js row went to %q, want /tools/node", a.router.Path())
	}

	// P8: at 1000x680, AI card and Project files card stack vertically (Files below AI).
	a.router.Push(projectPath(dir))
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-project-1000")
	aiBox, okAI := tt.Find(locales.ProjectAI())
	filesBox, okFiles := tt.Find(locales.ProjectFiles())
	if !okAI || !okFiles {
		t.Fatalf("AI/Files cards missing at 1000x680: ai=%v files=%v", okAI, okFiles)
	}
	if filesBox.Y <= aiBox.Y {
		t.Errorf("at 1000x680 Project files (Y=%.0f) should stack below AI settings (Y=%.0f)", filesBox.Y, aiBox.Y)
	}
}

// Block 4 (X1): Problems row buttons align on the same line at 1280x820 and wrap cleanly at 1000x680.
func TestS14ProblemsUniformButtonsAndNarrow(t *testing.T) {
	a := NewApp()
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{testProjects()[1]}
	a.Home = home
	a.router.Push(problemsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-problems-1280")

	hideBox, okHide := tt.Find(locales.ProblemHide())
	openBox, okOpen := tt.Find(locales.IssueOpen())
	if !okHide || !okOpen {
		t.Fatalf("problem buttons missing: hide=%v open=%v", okHide, okOpen)
	}
	if hideBox.H != openBox.H || hideBox.Y != openBox.Y {
		t.Errorf("problem action buttons misaligned: hide=(Y=%.1f,H=%.1f) open=(Y=%.1f,H=%.1f)", hideBox.Y, hideBox.H, openBox.Y, openBox.H)
	}
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-problems-1000")
	titleBox, okTitle := tt.Find("legacy-admin needs Node 18")
	openNarrow, okOpenNarrow := tt.Find(locales.IssueOpen())
	if !okTitle || !okOpenNarrow {
		t.Fatalf("narrow problems elements missing: title=%v open=%v", okTitle, okOpenNarrow)
	}
	if openNarrow.Y <= titleBox.Y {
		t.Errorf("at 1000x680 action buttons (Y=%.0f) should wrap below title (Y=%.0f)", openNarrow.Y, titleBox.Y)
	}

	// Verify 3-button narrow layout in Russian as well.
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-problems-1000-ru")
	hideRu, okHideRu := tt.Find(locales.ProblemHide())
	openRu, okOpenRu := tt.Find(locales.IssueOpen())
	if !okHideRu || !okOpenRu {
		t.Fatalf("Russian narrow problems buttons missing: hide=%v open=%v", okHideRu, okOpenRu)
	}
	if hideRu.Y != openRu.Y {
		t.Errorf("Russian narrow buttons should stay aligned on the wrapped row: hide.Y=%.1f open.Y=%.1f", hideRu.Y, openRu.Y)
	}
}

// Block 4 (X3): Settings custom app validation error, PATH command acceptance, and stale hidden issue formatting.
func TestS14SettingsCustomAppAndStaleIssue(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	a.Hidden = []string{
		"stale-plain-key",
		string(issues.ProjectMissing) + ":node:" + home + "/work/demo:.nvmrc:18",
	}
	a.router.Push("/settings")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	hasTexts(t, tt, "stale-plain-key", "Node.js · demo · .nvmrc · 18")

	// Reject malformed custom app with SettingsCustomAppInvalid (not PathAddInvalid).
	a.editorCustomOpen = true
	a.editorCustomInput = "bad;cmd"
	tt.Frame()
	if err := tt.Click(locales.SettingsSave() + " " + locales.SettingsEditor()); err != nil {
		t.Fatalf("click Save: %v", err)
	}
	tt.Frame()
	if a.SettingsError != locales.SettingsCustomAppInvalid() {
		t.Fatalf("SettingsError = %q, want %q", a.SettingsError, locales.SettingsCustomAppInvalid())
	}

	// Accept bare PATH command.
	a.editorCustomInput = "nvim"
	tt.Frame()
	if err := tt.Click(locales.SettingsSave() + " " + locales.SettingsEditor()); err != nil {
		t.Fatalf("click Save valid PATH command: %v", err)
	}
	tt.Frame()
	ed, ok := a.chosenEditor()
	if a.SettingsError != "" || a.Editor != "nvim" || !ok || ed.Path != "nvim" {
		t.Fatalf("expected nvim saved cleanly: err=%q editor=%q chosen=(%+v,%v)", a.SettingsError, a.Editor, ed, ok)
	}
}

// Block 4 (X4): Mobile stays visible in sidebar when Android SDK has 0 platforms and shows Counting… before sizes finish.
func TestS14MobileVisibilityAndPendingSizes(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{
		Tool: "mobile",
		Mobile: &inventory.MobileState{
			Android: &inventory.AndroidSDK{Root: home + "/Library/Android/sdk"},
		},
	}}}
	if !a.mobileFound() {
		t.Fatal("mobileFound() must be true when Android SDK is present")
	}
	for _, p := range pages() {
		if p.path == mobilePage && a.missing(p) {
			t.Fatal("mobilePage must not be marked missing when Android SDK is present")
		}
	}
	a.router.Push(mobilePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "…", locales.MobileCounting())
	if tt.HasText("0 KB") || tt.HasText("0 B") {
		t.Fatalf("pending mobile sizes must not display 0 KB: %q", tt.Texts())
	}
}

// Block 4 (X5): JS managers Projects tab badge matches actual JS projects count.
func TestS14JSManagersProjectsBadge(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{
		{Dir: home + "/work/go-only", Files: []string{"go.mod"}},
		{
			Dir:          home + "/work/js-one",
			Files:        []string{"package.json", "pnpm-lock.yaml"},
			Requirements: []projects.Requirement{{Tool: "pnpm", File: "pnpm-lock.yaml"}},
		},
		{
			Dir:          home + "/work/js-two",
			Files:        []string{"package.json", "yarn.lock"},
			Requirements: []projects.Requirement{{Tool: "yarn", File: "yarn.lock"}},
		},
	}
	if got := a.jsProjectCount(); got != 2 {
		t.Fatalf("jsProjectCount() = %d, want 2", got)
	}
}

// Block 4 (X7): Rust newer installed stable does not offer downgrade; beta shows Installed; duplicate target disabled; zero Fetched hides VersionsAsOf.
func TestS14RustVersionsAndDuplicateTarget(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.RustVersions = &versions.Cycles{
		Lines: []versions.Cycle{{Cycle: "1.84", Latest: "1.84.0", Release: time.Date(2025, 1, 9, 0, 0, 0, 0, time.UTC)}},
		// Fetched intentionally zero to verify guard.
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{
		Tool:     "rust",
		Managers: []inventory.Manager{{Name: "rustup"}},
		Installations: []inventory.Installation{
			{
				Tool: "rust", Version: "1.85.0", OnPath: 1, Default: true, Path: home + "/.rustup/toolchains/stable",
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup"},
				Rust:   &inventory.RustToolchain{Name: "stable-aarch64-apple-darwin", Targets: []string{"aarch64-apple-darwin", "wasm32-unknown-unknown"}},
			},
			{
				Tool: "rust", Version: "1.86.0-beta.1", Path: home + "/.rustup/toolchains/beta",
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup"},
				Rust:   &inventory.RustToolchain{Name: "beta-aarch64-apple-darwin"},
			},
		},
	}}}
	a.rustTab = 1
	a.router.Push("/tools/rust")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	if tt.HasText(locales.RustUpdateChannel("stable")) {
		t.Fatalf("newer installed stable 1.85.0 must not offer update to older train 1.84.0: %q", tt.Texts())
	}
	if tt.HasText("0001") {
		t.Fatalf("zero Fetched timestamp leaked into UI: %q", tt.Texts())
	}
	if !tt.HasText(locales.NoteInstalled()) {
		t.Fatalf("installed beta/stable should show Installed note: %q", tt.Texts())
	}

	// Duplicate target must be blocked.
	a.rustTab = 0
	a.rustTargetInput = "wasm32-unknown-unknown"
	tt.Frame()
	if err := tt.Click(locales.RustTargetAddTitle("wasm32-unknown-unknown")); err == nil && a.op != nil {
		t.Fatal("duplicate rust target must not propose an operation")
	}
}

// Block 4 (X8): php.ini opens via OpenInEditor and Homebrew Node offers Update to X.
func TestS14OpenConfigsAndBrewNodeUpdate(t *testing.T) {
	var opened []string
	a := NewApp()
	a.Home = home
	a.Editor = "code"
	a.OpenInEditor = func(_, _, path string, done func(error)) {
		opened = append(opened, path)
		if done != nil {
			done(nil)
		}
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{
			Tool: "node",
			Installations: []inventory.Installation{{
				Tool: "node", Version: "22.12.0", OnPath: 1, Path: "/opt/homebrew/bin/node",
				Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/node/22.12.0"},
			}},
		},
		{
			Tool: "php",
			Installations: []inventory.Installation{{
				Tool: "php", Version: "8.4.0", OnPath: 1, Path: "/opt/homebrew/bin/php",
				Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
				PHP:    &inventory.PHPConfig{Files: []string{"/opt/homebrew/etc/php/8.4/php.ini"}},
			}},
		},
		{
			Tool: "packages",
			PackageManagers: []inventory.PackageManager{{
				ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
				Packages: []inventory.SystemPackage{{Name: "node", Version: "22.12.0", Available: "22.14.0", Path: "/opt/homebrew/Cellar/node/22.12.0"}},
			}},
		},
	}}
	// Check Homebrew Node has an upgradeable package match.
	st, _ := a.Snapshot.Tool("node")
	pm, pkg, ok := a.copyPackage(st.Installations[0])
	if !ok || pkg.Available != "22.14.0" || !a.packageCan(pm, pkg) {
		t.Fatalf("Homebrew Node package lookup failed: ok=%v pm=%+v pkg=%+v", ok, pm, pkg)
	}
	// php.ini uses OpenInEditor.
	a.router.Push("/tools/php")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if err := tt.Click(locales.PhpOpenIni()); err != nil {
		t.Fatalf("click Open php.ini: %v", err)
	}
	if len(opened) != 1 || opened[0] != "/opt/homebrew/etc/php/8.4/php.ini" {
		t.Fatalf("OpenInEditor calls = %q", opened)
	}
}

// Block 4 (X9): CLI search matches product names, AWS without SSO shows terminal
// lead + Copy command fallback.
func TestS14CLIPolish(t *testing.T) {
	a := cliTestApp()
	a.router.Push(cliPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Search by full product name "Google Cloud CLI".
	a.cliFilter = "Google Cloud"
	tt.Frame()
	if !tt.HasText("gcloud") {
		t.Fatalf("CLI search did not match product name Google Cloud: %q", tt.Texts())
	}
	a.cliFilter = ""
	tt.Frame()

	// AWS login dialog without SSO and without OpenCloudLogin shows terminal lead & Copy command.
	a.cloudLogin = cloudLoginDialog{open: true, tool: "aws", profile: "default", dir: home + "/.aws", hasSSO: false}
	a.OpenCloudLogin = nil
	tt.Frame()
	hasTexts(t, tt, locales.CloudLoginLeadTerminal("aws", home+"/.aws"), locales.CloudLoginCopyCommand())
}

// Block 4 (X10): Project AGENTS.md uses project template; non-matching search shows NothingMatches and hides empty Projects card.
func TestS14InstructionsPolish(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = aiSnapshot()
	a.Projects = []projects.Project{{Dir: home + "/work/app"}}
	a.router.Push(instructionsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	a.openCreateProjectInstructions(home + "/work/app")
	if a.projectInstructionsCreate == nil || a.projectInstructionsCreate.text != locales.ProjectInstructionsTemplate() {
		t.Fatalf("project instructions template wrong: %+v", a.projectInstructionsCreate)
	}
	a.projectInstructionsCreate = nil

	a.instructionsQuery = "definitely-no-match"
	tt.Frame()
	hasTexts(t, tt, locales.NothingMatches())
	projectsHeaders := 0
	for _, txt := range tt.Texts() {
		if txt == locales.InstructionsProjects() {
			projectsHeaders++
		}
	}
	// Only the sidebar "Projects" nav item may appear, not an empty Projects card header.
	if projectsHeaders > 1 {
		t.Fatalf("empty Projects header should be hidden during non-matching search: %q", tt.Texts())
	}
}

// Block 4 (X11): Search ranks title matches ahead of 60+ detail matches, caches
// index builds, resets page tab on navigation, and skips EOL available versions.
func TestS14SearchRankingCacheTabAndEOL(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	// Add 70 projects whose paths mention "node" in detail, while the Node.js page has "Node.js" in title.
	for i := range 70 {
		a.Projects = append(a.Projects, projects.Project{
			Dir: filepath.Join(home, "node-workspace", "app-"+string(rune('a'+i%26))+string(rune('0'+i/26))),
		})
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	a.NodeVersions = &versions.Node{
		Releases: []versions.Release{{Version: "22.14.0"}, {Version: "16.20.2"}},
		Lines: map[int]versions.Line{
			22: {End: time.Date(2027, 4, 30, 0, 0, 0, 0, time.UTC)},
			16: {End: time.Date(2023, 9, 11, 0, 0, 0, 0, time.UTC)},
		},
	}

	a.search.query = "node"
	res1 := a.searchResultsAt(now)
	res2 := a.searchResultsAt(now)
	if len(res1) == 0 || res1[0].id != "/tools/node" {
		t.Fatalf("title match /tools/node should rank first ahead of 70 detail matches, got %q", res1[0].id)
	}
	if a.search.indexBuilds != 1 || len(res2) != len(res1) {
		t.Fatalf("search index rebuilt unnecessarily: builds=%d", a.search.indexBuilds)
	}

	// Opening /tools/node resets nodeTab to 0.
	a.nodeTab = 2
	a.openSearchResult(res1[0])
	if a.nodeTab != 0 || a.router.Path() != "/tools/node" {
		t.Fatalf("opening search result did not reset tab: nodeTab=%d loc=%q", a.nodeTab, a.router.Path())
	}

	// Available search skips EOL Node 16 while keeping supported Node 22.
	a.search.available = true
	a.search.query = "node"
	avail := a.searchResultsAt(now)
	var has22, has16 bool
	for _, r := range avail {
		if r.id == "available:node:22.14.0" {
			has22 = true
		}
		if r.id == "available:node:16.20.2" {
			has16 = true
		}
	}
	if !has22 || has16 {
		t.Fatalf("EOL filtering failed: has22=%v has16=%v", has22, has16)
	}
}

// Block 4 (X12): Changes view has Back to overview button and localized skill title.
func TestS14ChangesBackAndSkillTitle(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	a.History = []history.Entry{{
		Taken: time.Now(),
		Changes: []history.Change{{
			Area: history.Skill,
			Kind: history.Added,
			Name: "golang-testing",
			Path: home + "/.agents/skills/golang-testing",
		}},
	}}
	a.router.Push(changesPage.path)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "s14-changes-1280")

	hasTexts(t, tt, locales.BackToOverview(), locales.ChangeSkillTitle("golang-testing"))
	backBox, okBack := tt.Find(locales.BackToOverview())
	subBox, okSub := tt.Find(locales.ChangesSubtitle())
	if !okBack || !okSub || backBox.X >= subBox.X {
		t.Fatalf("Back to overview should sit left of header subtitle: back=(%+v,%v) sub=(%+v,%v)", backBox, okBack, subBox, okSub)
	}
	if err := tt.Click(locales.BackToOverview()); err != nil {
		t.Fatalf("click Back to overview: %v", err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != "/" {
		t.Fatalf("Back to overview went to %q, want /", a.router.Path())
	}
}

// Block 4 (X13): PATH Differences tab respects pathFilter and shows EnvPathNoMatches when empty.
func TestS14PathDifferencesFilter(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = calmSnapshot()
	a.Snapshot.TerminalPath = []inventory.PathEntry{
		{Dir: "/opt/homebrew/bin"},
		{Dir: home + "/.cargo/bin"},
	}
	a.Snapshot.GUIPath = []inventory.PathEntry{{Dir: "/usr/bin"}}
	a.pathTab = 1
	a.pathFilter = "cargo"
	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "~/.cargo/bin")
	if tt.HasText("/opt/homebrew/bin") {
		t.Fatalf("pathFilter did not filter Differences tab: %q", tt.Texts())
	}

	a.pathFilter = "no-such-dir"
	tt.Frame()
	hasTexts(t, tt, locales.EnvPathNoMatches())
}
