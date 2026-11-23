package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestRustComponentsAndToolchainSwitch(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "rust",
		Managers: []inventory.Manager{{
			Name: "rustup", Version: "1.27.1", Shims: true,
		}},
		Installations: []inventory.Installation{
			{
				Tool: "rust", Version: "1.84.0", Path: home + "/.cargo/bin/rustc",
				RealPath: home + "/.rustup/toolchains/stable-aarch64-apple-darwin/bin/rustc",
				OnPath:   1, Default: true,
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Version: "stable-aarch64-apple-darwin"},
				Rust: &inventory.RustToolchain{
					Name:       "stable-aarch64-apple-darwin",
					Targets:    []string{"aarch64-apple-darwin"},
					Components: []string{"rustfmt"},
				},
			},
			{
				Tool: "rust", Version: "1.86.0-nightly",
				RealPath: home + "/.rustup/toolchains/nightly-aarch64-apple-darwin/bin/rustc",
				Source:   inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Version: "nightly-aarch64-apple-darwin"},
				Rust: &inventory.RustToolchain{
					Name:       "nightly-aarch64-apple-darwin",
					Targets:    []string{"aarch64-apple-darwin", "wasm32-unknown-unknown"},
					Components: []string{"clippy", "rust-src"},
				},
			},
		},
	}}}
	a.router.Push("/tools/rust")
	tt := newTester(a)

	hasTexts(t, tt, "Targets · stable", "Components · stable", "Install clippy", "Remove rustfmt")

	// Installing clippy proposes rustup component add for stable.
	if err := tt.Click("Install clippy"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("rustup component add --toolchain stable-aarch64-apple-darwin clippy") {
		t.Fatalf("missing component add plan: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Switching selected toolchain updates both Targets and Components cards.
	a.rustToolchain = "nightly-aarch64-apple-darwin"
	tt.Frame()
	hasTexts(t, tt, "Targets · nightly", "Components · nightly", "wasm32-unknown-unknown", "Remove clippy", "Install rustfmt")
}

func TestGoEnvironmentGobinAndToolchain(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "go",
		Installations: []inventory.Installation{{
			Tool: "go", Version: "1.24.1", Path: "/usr/local/go/bin/go", OnPath: 1,
		}},
		Vars: map[string]string{
			"GOPATH":      home + "/go",
			"GOBIN":       home + "/go/bin",
			"GOMODCACHE":  home + "/go/pkg/mod",
			"GOTOOLCHAIN": "local",
		},
	}}}
	a.router.Push("/tools/go")
	tt := newTester(a)
	hasTexts(t, tt, "GOPATH", "GOBIN", "~/go/bin", "GOTOOLCHAIN", "local")
}

func TestCppEmptyInstallAndCcacheClean(t *testing.T) {
	// Empty C/C++ page shows a single compiler install button in the empty state.
	empty := NewApp()
	empty.Home = home
	empty.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "cpp"}, wingetState}}
	empty.router.Push("/tools/cpp")
	ttEmpty := newTester(empty)
	if plan, ok := empty.compilerPlan(); ok {
		if n := countText(ttEmpty, plan.Title); n != 1 {
			t.Fatalf("expected 1 empty-state install button %q, got %d in %q", plan.Title, n, ttEmpty.Texts())
		}
	}

	// Present ccache with counted size shows Clean… button opening a cleanup plan.
	cacheDir := filepath.Join(t.TempDir(), "ccache")
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "cpp",
		Installations: []inventory.Installation{{
			Tool: "cc", Version: "16.0.0", Path: "/usr/bin/clang", OnPath: 1,
		}},
		Cpp: &inventory.CppState{
			Tools: []inventory.Installation{{Tool: "ccache", Version: "4.10", Path: "/opt/homebrew/bin/ccache", OnPath: 1}},
		},
	}}}
	a.Sizes = &inventory.Sizes{
		Caches: []inventory.Cache{{ID: "ccache", Path: cacheDir}},
		Paths:  map[string]inventory.Usage{cacheDir: {Bytes: 256 << 20}},
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home}, nil))
	}
	a.router.Push("/tools/cpp")
	tt := newTester(a)
	if err := tt.Click(locales.CppCleanCache()); err != nil {
		t.Fatalf("clean ccache button missing: %v (%q)", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil {
		t.Fatal("expected cleanup plan dialog for ccache")
	}
}

func TestBunAndDotnetManagerAndProjects(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{
		{
			Tool: "bun",
			Managers: []inventory.Manager{{
				Name: "mise", Version: "2025.3.0", Shims: true,
				ConfigPath: home + "/.config/mise/config.toml",
			}},
			Installations: []inventory.Installation{{
				Tool: "bun", Version: "1.2.4", Path: home + "/.local/share/mise/shims/bun", OnPath: 1,
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise"},
			}},
		},
		{
			Tool: "dotnet",
			Managers: []inventory.Manager{{
				Name: "mise", Version: "2025.3.0", Shims: true,
			}},
			Installations: []inventory.Installation{{
				Tool: "dotnet", Version: "9.0.200", Path: "/usr/local/share/dotnet/dotnet", OnPath: 1,
				Dotnet: &inventory.Dotnet{
					SDKs: []inventory.DotnetEntry{{Version: "9.0.200", Path: "/usr/local/share/dotnet/sdk/9.0.200"}},
				},
			}},
		},
	}}
	a.Projects = []projects.Project{{
		Dir: home + "/work/api",
		Requirements: []projects.Requirement{{
			Tool: "dotnet", Want: "9.0.200", File: "global.json",
		}},
	}}

	a.router.Push("/tools/bun")
	ttBun := newTester(a)
	hasTexts(t, ttBun, "Version manager · mise")

	a.router.Push("/tools/dotnet")
	ttDotnet := newTester(a)
	hasTexts(t, ttDotnet, "Version manager · mise")
	// Switch to Projects tab (tab index 2) and verify project requirement renders via toolProjectsTab.
	a.dotnetTab = 2
	ttDotnet.Frame()
	hasTexts(t, ttDotnet, "api", "global.json")
}

func TestGitHeaderEmailAndMissingConfigButtons(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "git",
		Installations: []inventory.Installation{{
			Tool: "git", Version: "2.47.1", Path: "/usr/bin/git", OnPath: 1,
		}},
		Git: &inventory.GitState{
			ConfigFile:       home + "/.gitconfig",
			ConfigMissing:    true,
			SSHConfigMissing: true,
			Config: []inventory.Setting{
				{Name: "user.name", Value: "Ann"},
				{Name: "user.email", Value: "ann@example.com"},
			},
			Keys: []inventory.SSHKey{{Name: "id_ed25519", Type: "ED25519"}},
		},
	}}}
	a.router.Push("/tools/git")
	tt := newTester(a)
	hasTexts(t, tt, "Ann", "ann@example.com")
	if tt.HasText(locales.OpenFile(".gitconfig")) {
		t.Error("Open .gitconfig should be hidden when ConfigMissing is true")
	}
	if tt.HasText(locales.OpenFile("~/.ssh/config")) {
		t.Error("Open SSH config should be hidden when SSHConfigMissing is true")
	}
}

func TestEditorsCopyListAndExpand(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "editors",
		Editors: &inventory.EditorsState{Editors: []inventory.Editor{{
			ID:         "vscode",
			Name:       "Visual Studio Code",
			App:        "/Applications/Visual Studio Code.app",
			Version:    "1.98.0",
			Extensions: home + "/.vscode/extensions",
			Installed:  []string{"golang.go", "rust-lang.rust-analyzer"},
			Superseded: []string{home + "/.vscode/extensions/golang.go-0.44.0"},
		}}},
	}}}
	a.router.Push("/tools/editors")
	tt := newTester(a)

	hasTexts(t, tt, "golang.go-0.44.0")
	if tt.HasText("rust-lang.rust-analyzer") {
		t.Error("extensions list should be collapsed initially")
	}
	if err := tt.Click(locales.EditorsShowList()); err != nil {
		t.Fatal(err)
	}
	hasTexts(t, tt, "golang.go", "rust-lang.rust-analyzer", locales.EditorsHideList())
	if err := tt.Click(locales.EditorsCopyList()); err != nil {
		t.Fatalf("Copy list missing: %v (%q)", err, tt.Texts())
	}
	if got := tt.Clipboard(); got != "golang.go\nrust-lang.rust-analyzer" {
		t.Errorf("clipboard = %q", got)
	}
	hasTexts(t, tt, locales.EditorsListCopied())
}

func TestMobileFlutterCardAndDeveloperDir(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	flutterInst := inventory.Installation{Tool: "flutter", Version: "3.29.0", Path: home + "/flutter/bin/flutter"}
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "mobile",
		Mobile: &inventory.MobileState{
			DeveloperDir: "/Applications/Xcode.app/Contents/Developer",
			Flutter: &inventory.Flutter{
				Installation: flutterInst,
				Root:         home + "/flutter",
				Channel:      "stable",
			},
		},
	}}}
	a.router.Push("/tools/mobile")
	tt := newTester(a)
	hasTexts(t, tt, "Flutter", "3.29.0", "stable", "xcode-select", "/Applications/Xcode.app/Contents/Developer")
}

func TestPathOpenSourceAndRemoveHearthEntry(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	hearthDir := filepath.Join(dir, "hearth-bin")
	body := strings.Join([]string{
		ops.BlockStart,
		`export PATH="` + hearthDir + `:$PATH"`,
		ops.BlockEnd,
	}, "\n") + "\n"
	if err := os.WriteFile(rc, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.Home = dir
	shell := &fakeShell{}
	a.Shell = shell
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "SHELL=/bin/zsh"}, nil))
	}
	a.Snapshot = &inventory.Snapshot{
		TerminalPath: []inventory.PathEntry{{
			Dir: hearthDir,
			Sources: []inventory.PathSource{{
				File:   rc,
				Line:   2,
				Hearth: true,
			}},
		}},
	}
	a.router.Push("/system/path")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Open source file button opens ~/.zshrc.
	if err := tt.Click(locales.PathOpenSource()); err != nil {
		t.Fatalf("Open source file missing: %v (%q)", err, tt.Texts())
	}
	if !slices.Contains(shell.opened, rc) {
		t.Errorf("opened = %v, want %s", shell.opened, rc)
	}

	// Remove action in More menu opens PathRemovePlan dialog for the Hearth-managed entry.
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatalf("More button missing: %v (%q)", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem(locales.PathRemove()); err != nil {
		t.Fatalf("Remove menu item missing: %v", err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	if a.op == nil || !strings.Contains(a.op.plan.Title, hearthDir) {
		t.Fatalf("expected PathRemovePlan dialog, got op=%+v", a.op)
	}
}

func TestSkillDetailFooterAndActiveAgentFilterChip(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Shell = &fakeShell{}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(nil, nil))
	}
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Dir: home + "/.claude", Installations: []inventory.Installation{{Tool: "claude", Version: "2.1.0", Path: "/opt/homebrew/bin/claude", OnPath: 1}}},
			{ID: "codex", Dir: home + "/.codex", Installations: []inventory.Installation{{Tool: "codex", Version: "0.40.0", Path: "/opt/homebrew/bin/codex", OnPath: 1}}},
			{ID: "antigravity", Dir: home + "/.gemini/antigravity-cli", Installations: []inventory.Installation{{Tool: "antigravity", Version: "1.5.0", Path: "/opt/homebrew/bin/agy", OnPath: 1}}},
		},
		Skills: []inventory.Skill{{
			Name:        "planeta-frontend-data",
			Description: "Load and cache server data",
			Dir:         home + "/.agents/skills/planeta-frontend-data",
			Sum:         "abc",
			Paths: []inventory.SkillPath{
				{Path: home + "/.agents/skills/planeta-frontend-data", Agents: []string{"codex", "antigravity"}},
			},
		}},
	}
	a.LoadSkill = func(dir string, done func([]inventory.Entry, string, error)) {
		done([]inventory.Entry{{Name: "SKILL.md", Size: 2048}}, "---\nname: planeta-frontend-data\n---\n# Title\n", nil)
	}
	a.skillAgent = "antigravity"
	a.router.Push(skillsPage)

	for _, loc := range []string{"en", "ru"} {
		i18n.SetLocaleFunc(func() string { return loc })
		for _, w := range []int{1280, 1000} {
			tt := ui.NewTester(a.View, w, 820)
			tt.Advance(time.Second)
			hasTexts(t, tt, locales.OpenShort(), showInFolderShort(), locales.SkillRemoveButton())
			// All three footer buttons must remain inside the window viewport without right-edge clipping.
			for _, label := range []string{locales.OpenInEditor(), showInFolder(), locales.SkillRemoveButton()} {
				r, ok := tt.Find(label)
				if !ok {
					i18n.SetLocaleFunc(nil)
					t.Fatalf("locale %s width %d: button %q not found in tree", loc, w, label)
				}
				if r.X+r.W > float32(w)-20 {
					i18n.SetLocaleFunc(nil)
					t.Errorf("locale %s width %d: button %q clips right edge: rect=%+v", loc, w, label, r)
				}
			}
			if w == 1280 && loc == "ru" {
				screenshot(t, tt, "skills-active-antigravity-footer-ru")
			}
		}
	}
	i18n.SetLocaleFunc(nil)
	// Active Codex chip also renders without error (uses inverted dark logo variant).
	a.skillAgent = "codex"
	ttCodex := ui.NewTester(a.View, 1280, 820)
	ttCodex.Advance(time.Second)
	screenshot(t, ttCodex, "skills-active-codex-footer-en")
}
