//go:build promo

package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

const promoHome = "/Users/alex"

// sidebarCropX is the 2x pixel offset immediately after the 244px sidebar
// and its 1px vertical divider ((244 + 1) * 2).
const sidebarCropX = 490

// TestGeneratePromoScreenshots renders clean, deterministic Retina (2560x1640)
// screenshots of Hearth in both Light and Dark themes for the website and README.
//
// Run with:
//
//	HEARTH_PROMO_DIR=.local/promo go test -tags promo -run TestGeneratePromoScreenshots ./internal/ui
func TestGeneratePromoScreenshots(t *testing.T) {
	dir := os.Getenv("HEARTH_PROMO_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", ".local", "promo")
	}
	panelsDir := filepath.Join(dir, "panels")
	if err := os.MkdirAll(panelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	i18n.SetLocaleFunc(func() string { return "en" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	shots := []struct {
		name      string
		skipPanel bool
		setup     func(a *App, now time.Time)
		after     func(t *testing.T, a *App, tt *ui.Tester, now time.Time)
	}{
		{
			name: "overview",
			setup: func(a *App, _ time.Time) {
				a.router.Push(overview.path)
			},
		},
		{
			name: "projects-list",
			setup: func(a *App, _ time.Time) {
				a.router.Push(projectsPage.path)
			},
		},
		{
			name: "projects",
			setup: func(a *App, _ time.Time) {
				a.router.Push(projectPath(promoHome + "/dev/billing-api"))
			},
		},
		{
			name: "issues",
			setup: func(a *App, _ time.Time) {
				a.router.Push(problemsPage)
			},
		},
		{
			name: "updates",
			setup: func(a *App, _ time.Time) {
				a.router.Push(updatesPage)
			},
		},
		{
			name:      "plan-diff",
			skipPanel: true,
			setup: func(a *App, _ time.Time) {
				a.router.Push(pathPage)
				a.Propose(promoPathFixPlan())
			},
		},
		{
			name: "cleanup",
			setup: func(a *App, _ time.Time) {
				a.router.Push(cleanupPage)
			},
		},
		{
			name: "setup",
			setup: func(a *App, _ time.Time) {
				a.router.Push(setupPage)
			},
		},
		{
			name: "bun",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/bun")
			},
		},
		{
			name: "go",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/go")
			},
		},
		{
			name: "node",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/node")
			},
		},
		{
			name: "python",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/python")
			},
		},
		{
			name: "rust",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/rust")
			},
		},
		{
			name: "docker",
			setup: func(a *App, _ time.Time) {
				a.engine.view = dockerOverview
				a.router.Push(dockerEnginePath("colima"))
			},
		},
		{
			name: "kubernetes",
			setup: func(a *App, _ time.Time) {
				a.router.Push(kubePage)
			},
			after: func(t *testing.T, a *App, tt *ui.Tester, now time.Time) {
				t.Helper()
				if err := tt.Click("Open"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
				cluster := promoKubeCluster(now)
				expandGroups(a, cluster)
				tt.Frame()
				tt.Advance(time.Second)
			},
		},
		{
			name: "databases",
			setup: func(a *App, _ time.Time) {
				a.router.Push("/tools/databases")
			},
		},
		{
			name: "ports",
			setup: func(a *App, _ time.Time) {
				a.router.Push(portsPage)
			},
		},
		{
			name: "ai-usage",
			setup: func(a *App, _ time.Time) {
				a.router.Push(usagePage)
			},
		},
		{
			name: "ai-skills",
			setup: func(a *App, _ time.Time) {
				a.router.Push(skillsPage)
			},
		},
		{
			name: "ai-mcp",
			setup: func(a *App, _ time.Time) {
				a.router.Push(mcpPage)
			},
		},
		{
			name: "path",
			setup: func(a *App, _ time.Time) {
				a.router.Push(pathPage)
			},
		},
		{
			name: "settings",
			setup: func(a *App, _ time.Time) {
				a.router.Push(settings.path)
			},
		},
	}

	themes := []struct {
		suffix string
		dark   bool
		mode   string
	}{
		{"light", false, "light"},
		{"dark", true, "dark"},
	}

	for _, shot := range shots {
		for _, th := range themes {
			t.Run(shot.name+"-"+th.suffix, func(t *testing.T) {
				a := promoApp(time.Now())
				a.ThemeMode = th.mode
				shot.setup(a, time.Now())

				tt := ui.NewTesterWith(a.View, 1280, 820, ui.TesterOptions{Dark: th.dark})
				tt.SetScale(2)
				tt.SetLocale("en")

				now := tt.Now()
				syncPromoTimes(a, now)
				if shot.name == "docker" {
					attachPromoDocker(a, now)
				}
				if shot.name == "kubernetes" {
					attachPromoKube(a, now)
				}

				tt.Frame()
				tt.Advance(time.Second)
				if shot.after != nil {
					shot.after(t, a, tt, now)
				}
				tt.Advance(500 * time.Millisecond)

				img := tt.Image()
				outPath := filepath.Join(dir, shot.name+"-"+th.suffix+".png")
				if err := writePNG(outPath, img); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s (%dx%d)", outPath, img.Bounds().Dx(), img.Bounds().Dy())

				if !shot.skipPanel {
					b := img.Bounds()
					crop := img.SubImage(image.Rect(sidebarCropX, b.Min.Y, b.Max.X, b.Max.Y))
					panelPath := filepath.Join(panelsDir, shot.name+"-"+th.suffix+".png")
					if err := writePNG(panelPath, crop); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func promoApp(now time.Time) *App {
	a := NewApp()
	a.Home = promoHome
	a.Machine = "alex-macbook"
	a.OS = "macOS 15.6"
	a.AppVersion = "1.0.0"
	a.Locale = "en"
	a.StartHidden = true
	a.DockerPromoHidden = true
	a.ChooseFolder = func() {}
	a.FindProjects = func() {}
	a.OnScan = func() {}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.RunPlan = func(ops.Plan, func(ops.State)) func() { return func() {} }
	a.Snapshot = promoSnapshot(now)
	a.Projects = promoProjects(now)
	a.Sizes = promoSizes(now)
	a.NodeVersions = &versions.Node{
		Releases: []versions.Release{
			{Version: "22.15.0", LTS: "Jod"},
			{Version: "20.18.1", LTS: "Iron"},
			{Version: "18.20.7", LTS: "Hydrogen"},
		},
	}
	a.PythonVersions = &versions.Python{
		Lines: []versions.PythonLine{
			{Cycle: "3.13", Latest: "3.13.3", Release: now.AddDate(-1, 0, 0), End: now.AddDate(4, 0, 0)},
			{Cycle: "3.12", Latest: "3.12.9", Release: now.AddDate(-2, 0, 0), End: now.AddDate(3, 0, 0)},
		},
	}
	a.GoVersions = &versions.Go{Releases: []string{"1.24.2", "1.23.7"}}
	a.RustVersions = &versions.Rust{Lines: []versions.Cycle{{Cycle: "1.85", Latest: "1.85.0", Release: now.AddDate(0, 0, -10)}}}
	return a
}

func syncPromoTimes(a *App, now time.Time) {
	a.Snapshot.Taken = now.Add(-2 * time.Minute)
	if a.Sizes != nil {
		a.Sizes.Taken = now.Add(-2 * time.Minute)
	}
	a.Snapshot.Sessions = promoSessions(now)
	a.Projects = promoProjects(now)
}

func promoSnapshot(now time.Time) *inventory.Snapshot {
	brew := inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}
	npm := inventory.Source{Kind: inventory.KindPackageManager, Manager: "npm"}
	fnm := func(v string) inventory.Source {
		return inventory.Source{
			Kind:    inventory.KindVersionManager,
			Manager: "fnm",
			Root:    promoHome + "/.local/share/fnm/node-versions/v" + v,
			Version: v,
		}
	}
	uvRoot := func(v string) string {
		return promoHome + "/.local/share/uv/python/cpython-" + v + "-macos-aarch64-none"
	}
	miseRoot := promoHome + "/.local/share/mise"
	rustRoot := func(name string) string { return promoHome + "/.rustup/toolchains/" + name }
	kubeCfg := promoHome + "/.kube/config"
	kubeWork := promoHome + "/.kube/staging.yaml"

	return &inventory.Snapshot{
		Taken: now.Add(-2 * time.Minute),
		Tools: []inventory.ToolState{
			{
				Tool: "node",
				Installations: []inventory.Installation{
					{
						Tool: "node", Version: "22.14.0",
						Path:   promoHome + "/.local/state/fnm_multishells/48121/bin/node",
						Source: fnm("22.14.0"), OnPath: 1, InGUI: 1, Default: true, Corepack: true,
						Packages: []inventory.Package{
							{Name: "pnpm", Version: "10.6.2", Commands: []string{"pnpm"}},
							{Name: "typescript", Version: "5.8.2", Commands: []string{"tsc"}},
						},
					},
					{
						Tool: "node", Version: "20.18.1",
						Path:   promoHome + "/.local/share/fnm/node-versions/v20.18.1/installation/bin/node",
						Source: fnm("20.18.1"),
					},
				},
				Managers: []inventory.Manager{{Name: "fnm", Version: "1.38.1", SwitchOnCD: true, Files: []string{".node-version", ".nvmrc"}}},
			},
			{
				Tool: "python",
				Installations: []inventory.Installation{
					{
						Tool: "python", Version: "3.13.2",
						Path: uvRoot("3.13.2") + "/bin/python3", RealPath: uvRoot("3.13.2") + "/bin/python3",
						Source:  inventory.Source{Kind: inventory.KindVersionManager, Manager: "uv", Root: uvRoot("3.13.2"), Version: "3.13.2"},
						OnPath:  1,
						InGUI:   1,
						Default: true,
					},
					{
						Tool: "python", Version: "3.12.9",
						Path: uvRoot("3.12.9") + "/bin/python3", RealPath: uvRoot("3.12.9") + "/bin/python3",
						Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "uv", Root: uvRoot("3.12.9"), Version: "3.12.9"},
					},
				},
				Managers: []inventory.Manager{{Name: "uv", Version: "0.6.5"}},
			},
			{
				Tool: "go",
				Installations: []inventory.Installation{
					{
						Tool: "go", Path: miseRoot + "/shims/go", RealPath: miseRoot + "/shims/go",
						Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: miseRoot, Shim: true},
						OnPath: 1, InGUI: 1,
					},
					{
						Tool: "go", Version: "1.24.1",
						Path: miseRoot + "/installs/go/1.24.1/bin/go", RealPath: miseRoot + "/installs/go/1.24.1/bin/go",
						Source:  inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: miseRoot + "/installs/go/1.24.1", Version: "1.24.1"},
						Default: true,
					},
				},
				Managers: []inventory.Manager{{Name: "mise", Version: "2025.3.0", SwitchOnCD: true, Shims: true, ShimsInGUI: true}},
			},
			{
				Tool: "rust",
				Installations: []inventory.Installation{
					{
						Tool: "rust", Path: promoHome + "/.cargo/bin/rustc", RealPath: promoHome + "/.cargo/bin/rustup",
						Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Root: promoHome + "/.rustup", Shim: true},
						OnPath: 1, InGUI: 1,
					},
					{
						Tool: "rust", Version: "1.85.0",
						Path: rustRoot("stable-aarch64-apple-darwin") + "/bin/rustc", RealPath: rustRoot("stable-aarch64-apple-darwin") + "/bin/rustc",
						Source:  inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Root: rustRoot("stable-aarch64-apple-darwin")},
						Default: true,
						Rust:    &inventory.RustToolchain{Name: "stable-aarch64-apple-darwin", Date: "2025-02-20", Targets: []string{"aarch64-apple-darwin"}},
					},
				},
				Managers: []inventory.Manager{{Name: "rustup", Version: "1.28.1", SwitchOnCD: true, Shims: true, ShimsInGUI: true}},
			},
			{
				Tool: "bun",
				Installations: []inventory.Installation{
					{
						Tool: "bun", Version: "1.2.4",
						Path: promoHome + "/.bun/bin/bun", RealPath: promoHome + "/.bun/bin/bun",
						Source: inventory.Source{Kind: inventory.KindInstaller, Root: promoHome + "/.bun"},
						OnPath: 1, InGUI: 1, Default: true,
					},
				},
			},
			{
				Tool: "docker",
				Installations: []inventory.Installation{
					{
						Tool: "docker", Version: "27.5.1",
						Path: "/opt/homebrew/bin/docker", RealPath: "/opt/homebrew/Cellar/docker/27.5.1/bin/docker",
						OnPath: 1, InGUI: 1, Source: brew,
					},
				},
				Docker: &inventory.DockerState{
					Selected:   "colima",
					Running:    4,
					Containers: 6,
					Contexts: []inventory.DockerContext{
						{Name: "colima", Endpoint: "unix://" + promoHome + "/.colima/default/docker.sock", Engine: "Colima", Status: "available", Local: true},
						{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true},
					},
					Engines: []inventory.DockerEngine{
						{Name: "Colima", Path: "/opt/homebrew/bin/colima"},
						{Name: "Docker Desktop"},
						{Name: "OrbStack"},
					},
					Plugins: []inventory.Package{
						{Name: "compose", Commands: []string{"docker compose"}},
						{Name: "buildx", Commands: []string{"docker buildx"}},
					},
				},
			},
			{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Servers: []inventory.Database{
						{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17", Ports: []string{"5432"}},
						{Kind: "Redis", Name: "Redis", Version: "7.4.2", Path: "/opt/homebrew/bin/redis-server", Source: "brew", Status: "running", Service: "redis", Ports: []string{"6379"}},
					},
					Listeners: []inventory.DatabaseListener{
						{Address: "127.0.0.1", Port: "5432", Process: "postgres", PID: "4128"},
						{Address: "127.0.0.1", Port: "6379", Process: "redis-server", PID: "4204"},
					},
				},
			},
			{
				Tool: "cli",
				Cli: &inventory.CliState{
					Tools: []inventory.Installation{
						{Tool: "kubectl", Version: "1.32.2", Path: "/opt/homebrew/bin/kubectl", OnPath: 1, Source: brew},
						{Tool: "helm", Version: "3.17.1", Path: "/opt/homebrew/bin/helm", OnPath: 1, Source: brew},
						{Tool: "gh", Version: "2.67.0", Path: "/opt/homebrew/bin/gh", OnPath: 1, Source: brew},
					},
					Contexts: []inventory.CliContext{{Tool: "kubectl", Name: "kind-dev", File: kubeCfg}},
					KubeContexts: []inventory.KubeContext{
						{Name: "kind-dev", Cluster: "kind-dev", Namespace: "default", File: kubeCfg},
						{Name: "staging", Cluster: "eks-staging", Namespace: "shop", File: kubeWork, Auth: "aws"},
					},
					KubeFiles:     []inventory.KubeFile{{Path: kubeCfg}, {Path: kubeWork}},
					KubeconfigSet: true,
				},
			},
		},
		TerminalPath: []inventory.PathEntry{
			{Dir: promoHome + "/.local/state/fnm_multishells/48121/bin", Sources: []inventory.PathSource{{File: promoHome + "/.zshrc", Line: 4, Hook: true}}},
			{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{{File: promoHome + "/.zprofile", Line: 1, Hook: true}}},
			{Dir: promoHome + "/.local/share/mise/shims", Sources: []inventory.PathSource{{File: promoHome + "/.zshrc", Line: 7, Hook: true}}},
			{Dir: promoHome + "/.cargo/bin", Sources: []inventory.PathSource{{File: promoHome + "/.zshenv", Line: 1}}},
			{Dir: promoHome + "/.nvm/versions/node/v16.20.2/bin", Missing: true, Sources: []inventory.PathSource{{File: promoHome + "/.zshrc", Line: 14}}},
			{Dir: "/usr/local/bin"},
			{Dir: "/usr/bin"},
			{Dir: "/bin"},
		},
		GUIPath: []inventory.PathEntry{
			{Dir: "/opt/homebrew/bin"},
			{Dir: "/usr/local/bin"},
			{Dir: "/usr/bin"},
			{Dir: "/bin"},
		},
		Agents: []inventory.Agent{
			{
				ID: "claude", Dir: promoHome + "/.claude",
				Installations: []inventory.Installation{{Tool: "claude", Version: "2.1.312", Path: "/opt/homebrew/bin/claude", OnPath: 1, Source: brew}},
				Files: []inventory.AgentFile{
					{Path: promoHome + "/.claude/settings.json", Kind: inventory.FileSettings},
					{Path: promoHome + "/.claude/CLAUDE.md", Kind: inventory.FileInstructions},
					{Path: promoHome + "/.claude/skills", Kind: inventory.FileSkills},
				},
			},
			{
				ID: "codex", Dir: promoHome + "/.codex",
				Installations: []inventory.Installation{{Tool: "codex", Version: "0.168.0", Path: promoHome + "/.npm/bin/codex", OnPath: 1, InGUI: 1, Source: npm}},
				Files: []inventory.AgentFile{
					{Path: promoHome + "/.codex/AGENTS.md", Kind: inventory.FileInstructions},
					{Path: promoHome + "/.codex/config.toml", Kind: inventory.FileSettings},
				},
			},
			{
				ID: "cursor", Dir: promoHome + "/.cursor", App: "/Applications/Cursor.app",
				Files: []inventory.AgentFile{{Path: promoHome + "/.cursor/mcp.json", Kind: inventory.FileMCP}},
			},
		},
		Skills: []inventory.Skill{
			{
				Name: "golang-testing", Description: "Table-driven Go tests, fixtures, and race checks",
				Dir: promoHome + "/.agents/skills/golang-testing", Sum: "s1",
				Paths: []inventory.SkillPath{
					{Path: promoHome + "/.claude/skills/golang-testing", Link: true, Agents: []string{"claude"}},
					{Path: promoHome + "/.agents/skills/golang-testing", Agents: []string{"codex"}},
				},
			},
			{
				Name: "react-perf", Description: "Audit bundle size, suspense boundaries, and memoization",
				Dir: promoHome + "/.agents/skills/react-perf", Sum: "s2",
				Paths: []inventory.SkillPath{
					{Path: promoHome + "/.claude/skills/react-perf", Link: true, Agents: []string{"claude"}},
					{Path: promoHome + "/.agents/skills/react-perf", Agents: []string{"codex"}},
				},
			},
			{
				Name: "sql-review", Description: "Check Postgres migrations for locks and missing indexes",
				Dir: promoHome + "/.claude/skills/sql-review", Sum: "s3",
				Paths: []inventory.SkillPath{
					{Path: promoHome + "/.claude/skills/sql-review", Agents: []string{"claude"}},
				},
			},
		},
		MCP: []inventory.MCPServer{
			{
				Name: "hearth", Agent: "claude", File: promoHome + "/.claude.json", Transport: "stdio",
				Command: "/Applications/Hearth.app/Contents/MacOS/hearth", Args: []string{"mcp"},
				CommandPath: "/Applications/Hearth.app/Contents/MacOS/hearth",
			},
			{
				Name: "hearth", Agent: "codex", File: promoHome + "/.codex/config.toml", Transport: "stdio",
				Command: "/Applications/Hearth.app/Contents/MacOS/hearth", Args: []string{"mcp"},
				CommandPath: "/Applications/Hearth.app/Contents/MacOS/hearth",
			},
			{
				Name: "github", Agent: "claude", File: promoHome + "/.claude.json", Transport: "stdio",
				Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"},
				Env: []string{"GITHUB_TOKEN"}, CommandPath: "/opt/homebrew/bin/npx",
			},
			{
				Name: "github", Agent: "cursor", File: promoHome + "/.cursor/mcp.json", Transport: "stdio",
				Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"},
				Env: []string{"GITHUB_TOKEN"}, CommandPath: "/opt/homebrew/bin/npx",
			},
			{
				Name: "linear", Agent: "claude", File: promoHome + "/.claude.json", Transport: "http",
				URL: "https://mcp.linear.app/mcp", Headers: []string{"Authorization"},
			},
			{
				Name: "postgres", Agent: "codex", File: promoHome + "/.codex/config.toml", Transport: "stdio",
				Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"},
				Env: []string{"DATABASE_URL"}, CommandPath: "/opt/homebrew/bin/npx",
			},
		},
		Sessions: promoSessions(now),
	}
}

func promoProjects(now time.Time) []projects.Project {
	return []projects.Project{
		{
			Dir:   promoHome + "/dev/acme-web",
			Files: []string{".nvmrc", "pnpm-lock.yaml", "compose.yaml"},
			AI:    []string{"CLAUDE.md", "AGENTS.md"},
			Requirements: []projects.Requirement{
				{Tool: "node", Want: "22", File: ".nvmrc"},
				{Tool: "pnpm", File: "pnpm-lock.yaml"},
			},
			Changed: now.AddDate(0, 0, -2),
		},
		{
			Dir:   promoHome + "/dev/billing-api",
			Files: []string{".nvmrc", "go.mod", "compose.yaml"},
			AI:    []string{"CLAUDE.md", ".mcp.json"},
			Requirements: []projects.Requirement{
				{Tool: "node", Want: "18", File: ".nvmrc"},
				{Tool: "go", Want: "1.24", File: "go.mod"},
			},
			Databases: []projects.DatabaseReference{
				{Kind: "PostgreSQL", Service: "postgres", File: "compose.yaml", Ports: []string{"5432"}},
			},
			Changed: now.AddDate(0, 0, -5),
		},
		{
			Dir:   promoHome + "/dev/ml-worker",
			Files: []string{".python-version", "uv.lock"},
			AI:    []string{"AGENTS.md"},
			Requirements: []projects.Requirement{
				{Tool: "python", Want: "3.12", File: ".python-version"},
			},
			Changed: now.AddDate(0, 0, -1),
		},
		{
			Dir:      promoHome + "/archive/legacy-landing",
			Installs: []string{"node_modules"},
			Changed:  now.AddDate(0, -8, 0),
		},
	}
}

func promoSizes(now time.Time) *inventory.Sizes {
	return &inventory.Sizes{
		Taken: now.Add(-2 * time.Minute),
		Paths: map[string]inventory.Usage{
			promoHome + "/Library/pnpm/store":                                  {Bytes: 4_200_000_000},
			promoHome + "/.bun/install/cache":                                  {Bytes: 1_800_000_000},
			promoHome + "/.cache/uv":                                           {Bytes: 1_900_000_000},
			promoHome + "/.local/share/fnm/node-versions/v20.18.1":             {Bytes: 240_000_000},
			filepath.Join(promoHome+"/archive/legacy-landing", "node_modules"): {Bytes: 860_000_000},
		},
		Caches: []inventory.Cache{
			{ID: "pnpm", Path: promoHome + "/Library/pnpm/store"},
			{ID: "bun", Path: promoHome + "/.bun/install/cache"},
			{ID: "uv", Path: promoHome + "/.cache/uv"},
		},
		Docker: []inventory.DockerUsage{
			{Type: "Images", Count: 14, Active: 10, Bytes: 5_800_000_000, Reclaimable: 1_400_000_000},
			{Type: "Containers", Count: 6, Active: 4, Bytes: 320_000_000, Reclaimable: 180_000_000},
			{Type: "Local Volumes", Count: 4, Active: 4, Bytes: 1_100_000_000, Reclaimable: 0},
			{Type: "Build Cache", Count: 34, Bytes: 3_900_000_000, Reclaimable: 3_900_000_000},
		},
	}
}

func promoSessions(now time.Time) []inventory.AgentSession {
	day := func(ago int) string { return now.AddDate(0, 0, -ago).Format(time.DateOnly) }
	web := promoHome + "/dev/acme-web"
	api := promoHome + "/dev/billing-api"
	ml := promoHome + "/dev/ml-worker"

	// Full 30-day realistic weekday/weekend curve across Claude Code and Codex.
	weights := []float64{
		0.72, 1.05, 0.65, 1.25, 0.42, 0.28, 0.90,
		1.12, 0.78, 0.96, 0.58, 0.30, 0.22, 0.85,
		1.02, 0.64, 0.88, 1.18, 0.48, 0.25, 0.76,
		0.94, 0.68, 1.10, 0.82, 0.35, 0.20, 0.74,
		0.92, 0.66,
	}
	var claudeWeb, codexAPI, claudeML []inventory.TokenUsage
	for ago, w := range weights {
		cIn := int64(float64(680_000) * w)
		cOut := int64(float64(150_000) * w)
		xIn := int64(float64(440_000) * w)
		xOut := int64(float64(96_000) * w)
		claudeWeb = append(claudeWeb, inventory.TokenUsage{
			Day: day(ago), Model: "claude-sonnet-4-6",
			Input: cIn, CacheRead: cIn * 3, Output: cOut,
		})
		codexAPI = append(codexAPI, inventory.TokenUsage{
			Day: day(ago), Model: "gpt-5.4",
			Input: xIn, CacheRead: xIn * 2, Output: xOut,
		})
		if ago%2 == 0 {
			mlIn := int64(float64(210_000) * w)
			mlOut := int64(float64(48_000) * w)
			claudeML = append(claudeML, inventory.TokenUsage{
				Day: day(ago), Model: "claude-opus-4-6",
				Input: mlIn, CacheRead: mlIn * 2, Output: mlOut,
			})
		}
	}

	return []inventory.AgentSession{
		{Agent: "claude", Dir: web, Size: 18_400_000, Modified: now.Add(-15 * time.Minute), Usage: claudeWeb},
		{Agent: "codex", Dir: api, Size: 12_100_000, Modified: now.Add(-42 * time.Minute), Usage: codexAPI},
		{Agent: "claude", Dir: ml, Size: 9_600_000, Modified: now.Add(-2 * time.Hour), Usage: claudeML},
		{Agent: "claude", Dir: promoHome + "/archive/old-proto", Size: 42_000_000, Modified: now.AddDate(0, -2, 0)},
	}
}

func promoPathFixPlan() ops.Plan {
	before := strings.Join([]string{
		`eval "$(fnm env --use-on-cd --shell zsh)"`,
		`eval "$(mise activate zsh)"`,
		`export PATH="$HOME/.nvm/versions/node/v16.20.2/bin:$PATH"`,
		`export EDITOR="cursor --wait"`,
	}, "\n") + "\n"
	after := ops.CommentOut(before, []int{3}, "no such folder")
	return ops.Plan{
		Title:           locales.PathFixTitle(1),
		Why:             locales.PathFixWhy(),
		Action:          locales.PathFixComment(1),
		RestartTerminal: true,
		Steps: []ops.Step{
			{Edit: &ops.FileEdit{Path: promoHome + "/.zshrc", Before: before, After: after}},
		},
		Changes: []string{
			"− " + promoHome + "/.nvm/versions/node/v16.20.2/bin",
		},
	}
}

func attachPromoDocker(a *App, now time.Time) {
	problem := docker.Container{
		ID: strings.Repeat("a", 64), Name: "billing-worker", Image: "ghcr.io/acme/billing:2.4.1",
		RestartCount: 8,
		Compose:      docker.ComposeProject{Name: "billing-api", Dir: promoHome + "/dev/billing-api"},
	}
	problem.State.Status, problem.State.ExitCode = "restarting", 1

	e := docker.Engine{
		Taken:      now,
		Version:    "27.5.1",
		OS:         "Ubuntu 24.04.2 LTS",
		Arch:       "aarch64",
		CPUs:       4,
		Memory:     8 << 30,
		Containers: 6,
		Running:    4,
		Stopped:    2,
		Images:     14,
		Overview: docker.Overview{
			Problems:   []docker.Container{problem},
			CPURead:    true,
			MemoryRead: true,
			Cores:      1.3,
			Memory:     4_300_000_000,
			Disk: docker.OverviewDisk{
				Usage: []inventory.DockerUsage{
					{Type: "Images", Count: 14, Active: 10, Bytes: 5_800_000_000, Reclaimable: 1_400_000_000},
					{Type: "Containers", Count: 6, Active: 4, Bytes: 320_000_000, Reclaimable: 180_000_000},
					{Type: "Local Volumes", Count: 4, Active: 4, Bytes: 1_100_000_000, Reclaimable: 0},
					{Type: "Build Cache", Count: 34, Bytes: 3_900_000_000, Reclaimable: 3_900_000_000},
				},
				VM: docker.VMDisk{
					Path:      promoHome + "/.colima/_lima/_disks/colima/datadisk",
					Allocated: 11_100_000_000,
				},
			},
			Events: docker.OverviewEvents{
				Items: []docker.Event{
					{Action: "die", Name: "billing-worker", ExitCode: "1", Count: 8},
				},
			},
		},
	}

	a.WatchDocker = func(_ docker.Target, _ bool, got func(docker.Engine, error)) func() {
		got(e, nil)
		return func() {}
	}
	a.WatchDockerContainers = func(_ docker.Target, got func([]docker.Container, error, error)) func() {
		got([]docker.Container{problem}, nil, nil)
		return func() {}
	}
}

func attachPromoKube(a *App, now time.Time) {
	cluster := promoKubeCluster(now)
	a.WatchCluster = func(_ kube.Target, got func(kube.Cluster, error)) func() {
		got(cluster, nil)
		return func() {}
	}
	a.ListNamespaces = func(_ kube.Target, got func([]string, error)) func() {
		got([]string{"default", "kube-system", "monitoring"}, nil)
		return func() {}
	}
}

func promoKubeCluster(now time.Time) kube.Cluster {
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	checkout := kube.Owner{Kind: "Deployment", Name: "checkout-api"}
	web := kube.Owner{Kind: "Deployment", Name: "acme-web"}
	worker := kube.Owner{Kind: "Deployment", Name: "billing-worker"}
	running := func(name, image string) kube.Container {
		return kube.Container{Name: name, Image: image, Ready: true, State: "running"}
	}
	api := running("api", "ghcr.io/acme/checkout:2.8.1")

	return kube.Cluster{
		Taken: now,
		Pods: []kube.Pod{
			{
				Namespace: "default", Name: "checkout-api-7d9f8c5b-x2k4p", UID: "p1",
				Phase: "Running", Reason: "CrashLoopBackOff", Ready: 1, Restarts: 9,
				Usage:    &kube.Resources{CPU: 0.46, Memory: 94 << 20},
				Requests: kube.Resources{CPU: 0.1, Memory: 32 << 20},
				Limits:   kube.Resources{CPU: 0.5, Memory: 100 << 20},
				Containers: []kube.Container{
					{
						Name: "api", Image: "ghcr.io/acme/checkout:2.8.1", Restarts: 9,
						State: "waiting", Reason: "CrashLoopBackOff",
						Message: "back-off 2m40s restarting failed container=api",
						Last:    &kube.Run{Reason: "Error", ExitCode: 1, Started: ago(3 * time.Minute), Finished: ago(3*time.Minute - 4*time.Second)},
					},
					running("envoy", "envoyproxy/envoy:v1.32.2"),
				},
				Node: "kind-worker", IP: "10.244.1.17", QoS: "Burstable",
				Created: ago(2 * time.Hour), Owner: checkout,
			},
			{
				Namespace: "default", Name: "checkout-api-7d9f8c5b-m8q2z", UID: "p2",
				Phase: "Running", Ready: 2,
				Usage:      &kube.Resources{CPU: 0.14, Memory: 68 << 20},
				Containers: []kube.Container{api, running("envoy", "envoyproxy/envoy:v1.32.2")},
				Node:       "kind-worker2", Created: ago(2 * time.Hour), Owner: checkout,
			},
			{
				Namespace: "default", Name: "acme-web-6b4d9c8f-j4n7w", UID: "p3",
				Phase: "Running", Ready: 1,
				Usage:      &kube.Resources{CPU: 0.08, Memory: 84 << 20},
				Containers: []kube.Container{running("web", "ghcr.io/acme/web:1.12.0")},
				Node:       "kind-worker", Created: ago(6 * time.Hour), Owner: web,
			},
			{
				Namespace: "default", Name: "acme-web-6b4d9c8f-t9v2r", UID: "p4",
				Phase: "Running", Ready: 1,
				Usage:      &kube.Resources{CPU: 0.06, Memory: 76 << 20},
				Containers: []kube.Container{running("web", "ghcr.io/acme/web:1.12.0")},
				Node:       "kind-worker2", Created: ago(6 * time.Hour), Owner: web,
			},
			{
				Namespace: "default", Name: "billing-worker-5f8a2-k7p3d", UID: "p5",
				Phase: "Running", Ready: 1,
				Usage:      &kube.Resources{CPU: 0.19, Memory: 96 << 20},
				Containers: []kube.Container{running("worker", "ghcr.io/acme/billing:2.4.1")},
				Node:       "kind-worker", Created: ago(14 * time.Hour), Owner: worker,
			},
			{
				Namespace: "default", Name: "db-migrate-29410-abcde", UID: "p6",
				Phase:      "Succeeded",
				Containers: []kube.Container{{Name: "migrate", Image: "ghcr.io/acme/migrate:4.2", State: "terminated", Reason: "Completed"}},
				Created:    ago(38 * time.Minute), Owner: kube.Owner{Kind: "Job", Name: "db-migrate-29410"},
			},
		},
		Deployments: []kube.Deployment{
			{Namespace: "default", Name: "checkout-api", UID: "d1", Replicas: 2, Pods: 2, Ready: 1, Updated: 2, Available: 1, Created: ago(12 * 24 * time.Hour)},
			{Namespace: "default", Name: "acme-web", UID: "d2", Replicas: 2, Pods: 2, Ready: 2, Updated: 2, Available: 2, Created: ago(18 * 24 * time.Hour)},
			{Namespace: "default", Name: "billing-worker", UID: "d3", Replicas: 1, Pods: 1, Ready: 1, Updated: 1, Available: 1, Created: ago(7 * 24 * time.Hour)},
		},
	}
}
