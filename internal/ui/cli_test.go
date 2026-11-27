package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func cliTestApp() *App {
	a := NewApp()
	a.Home = home
	cp := func(name, version string, def bool, term inventory.PathRank) inventory.Installation {
		root := home + "/.local/share/mise/installs/" + name + "/" + version
		return inventory.Installation{Tool: name, Version: version, Path: root, Default: def, OnPath: term,
			Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: root}}
	}
	brew := func(name, version string) inventory.Installation {
		root := "/opt/homebrew/Cellar/" + name + "/" + version
		return inventory.Installation{Tool: name, Version: version, Path: "/opt/homebrew/bin/" + name, OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: root}}
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "mise", MiseTools: []inventory.MiseTool{
			{Name: "deno", Installations: []inventory.Installation{cp("deno", "2.1.4", true, 1)}},
			{Name: "elixir", Installations: []inventory.Installation{cp("elixir", "1.17.3", false, 0)}},
			{Name: "helm", Installations: []inventory.Installation{cp("helm", "3.15.0", false, 0)}},
			{Name: "opentofu", Installations: []inventory.Installation{cp("opentofu", "1.8.5", true, 0)}},
			{Name: "zig", Installations: []inventory.Installation{cp("zig", "0.13.0", true, 0), cp("zig", "0.12.0", false, 0)}},
		}},
		{Tool: "cli", Cli: &inventory.CliState{
			Tools:     []inventory.Installation{brew("kubectl", "1.31.4"), brew("helm", "3.16.3"), brew("aws", "2.22.1"), brew("gcloud", "503.0.0"), brew("terraform", "1.9.8"), brew("tofu", "1.8.5")},
			HelmRepos: 3,
			Contexts: []inventory.CliContext{
				{Tool: "kubectl", Name: "kind-dev", File: home + "/.kube/config"},
				{
					Tool: "aws", Name: "staging", Region: "eu-central-1", NameSource: "AWS_PROFILE", RegionSource: "file",
					File: home + "/.aws/config", HasSSO: true,
					Choices: []inventory.CliChoice{
						{Name: "default", Region: "us-east-1"},
						{Name: "staging", Region: "eu-central-1", HasSSO: true},
					},
				},
				{
					Tool: "gcloud", Name: "default", NameSource: "file",
					File: home + "/.config/gcloud/configurations/config_default",
					Choices: []inventory.CliChoice{
						{Name: "default"},
						{Name: "work", Project: "acme-prod"},
					},
				},
			},
		}},
		{Tool: "node", Managers: []inventory.Manager{{Name: "mise", Activated: true, SwitchOnCD: true, Files: []string{"mise.toml", ".tool-versions"}}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
			Packages: []inventory.SystemPackage{{Name: "kubernetes-cli", Version: "1.31.4", Available: "1.32.0", Path: "/opt/homebrew/Cellar/kubectl/1.31.4"}}}}},
	}}
	a.Projects = []projects.Project{
		{
			Dir: home + "/work/payments-infra",
			Requirements: []projects.Requirement{
				{Tool: "terraform", Want: "~> 1.10", File: "versions.tf", Key: "terraform.required_version"},
			},
		},
		{
			Dir: home + "/work/infra-live",
			Requirements: []projects.Requirement{
				{Tool: "tofu", Want: ">= 1.8", File: "versions.tofu", Key: "terraform.required_version"},
			},
		},
	}
	return a
}

// The known commands by group, found or not, with where they point, on-demand
// sign-in card, and Terraform/OpenTofu project requirements; mise's other tools
// after them without duplicating opentofu.
func TestCliPage(t *testing.T) {
	orig := goos
	goos = "darwin"
	t.Cleanup(func() { goos = orig })
	a := cliTestApp()
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, ops.Terminal(nil))
		done(p, err)
	}
	a.CheckCloudAuth = func(tool, _ string, done func(detect.CloudAuth)) {
		if tool == "aws" {
			done(detect.CloudAuth{Tool: "aws", State: detect.CloudAuthVerified, Identity: "Developer/sergey", Account: "4821…7730"})
			return
		}
		done(detect.CloudAuth{Tool: tool, State: detect.CloudAuthNotLoggedIn})
	}
	a.router.Push(cliPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	hasTexts(t, tt,
		"CLI & clouds", "Infrastructure · command line", "of 16 known",
		"CLOUDS", "INFRASTRUCTURE", "UTILITIES", "FROM MISE · NO PAGE OF THEIR OWN",
		"Sign-in checked", "0 of 2", "not checked yet",
		"Projects need", "1", "Terraform ~> 1.10",
		"brew · profile staging · eu-central-1",
		"brew · default · no project chosen", "503.0.0",
		"not found · Azure CLI · not needed by projects",
		"brew · project payments-infra asks ~> 1.10", "1.9.8",
		"Where commands go", "aws · staging", "from AWS_PROFILE · eu-central-1",
		"gcloud · default", "from "+filepath.FromSlash("~/.config/gcloud")+" · no project chosen",
		"Who the provider sees", "aws: not checked", "gcloud: not checked", "Hearth checks only when you click",
		"Needed by projects", "payments-infra: Terraform ~> 1.10", "installed 1.9.8 · does not match",
		"infra-live: OpenTofu >= 1.8", "installed 1.8.5 · matches",
		"Project code is never run", "Hearth never runs init, plan or apply",
		"deno", "mise · in PATH", "2.1.4", "elixir", "mise · not set up", "zig",
		"mise · the default version · 1 more version: 0.12.0", "0.13.0",
	)
	// Kubernetes and opentofu must not duplicate on CLI & clouds.
	if tt.HasText("3.15.0") || tt.HasText("kubectl") || tt.HasText("KUBERNETES") || tt.HasText("kubectl · kind-dev") || tt.HasText("opentofu") {
		t.Errorf("unexpected Kubernetes or opentofu duplicate on CLI & clouds: %q", tt.Texts())
	}

	// Trigger on-demand check (all installed cloud CLIs).
	a.checkAllCloudAuth(a.cliState())
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "cli")

	hasTexts(t, tt,
		"1 of 2", "aws · gcloud not signed in",
		"aws: Developer/sergey", "account 4821…7730",
		"gcloud: not signed in", "Sign in…", "Check again",
	)

	// Narrow 1000x680 layout (Frame 148).
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "cli_narrow")
	hasTexts(t, ttNarrow, "Where commands go", "Who the provider sees", "Needed by projects")

	// Filter and tabs.
	a.cliFilter = "zi"
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("deno") || !tt.HasText("aws · staging") || !tt.HasText("zig") || tt.HasText("503.0.0") {
		t.Errorf("filter: %q", tt.Texts())
	}
	a.cliFilter = ""
	a.cliTab = 1 // clouds
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("aws") || tt.HasText("deno") {
		t.Errorf("clouds tab: %q", tt.Texts())
	}
}

func TestCliLoginDialogAndSwitchPlans(t *testing.T) {
	orig := goos
	goos = "darwin"
	t.Cleanup(func() { goos = orig })
	a := cliTestApp()
	var proposed ops.Plan
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		p, err := build(sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, ops.Terminal(nil))
		if err == nil {
			proposed = p
		}
		done(p, err)
	}
	a.router.Push(cliPage)

	// Open Frame 99 native sign-in modal for gcloud.
	gcloudIn, _ := cliCopy(a.cliState(), "gcloud")
	a.openCloudLogin("gcloud", gcloudIn, cliContext(a.cliState(), "gcloud"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cli_login")

	hasTexts(t, tt,
		"Sign in to Google Cloud",
		"gcloud auth login",
		"gcloud auth list",
		"Choose a project",
		"Hearth only starts the native CLI sign-in",
		"gcloud 503.0.0 · brew",
		"Open sign-in in browser",
	)

	// Close login modal and test infra version install plan (S12-03).
	a.cloudLogin.open = false
	a.installInfraVersion("terraform", "Terraform", "~> 1.10")
	hasTerraformStep := false
	for _, s := range proposed.Steps {
		if strings.Contains(strings.Join(s.Argv, " "), "terraform@1.10") {
			hasTerraformStep = true
			break
		}
	}
	if !hasTerraformStep {
		t.Fatalf("expected terraform@1.10 install plan, got %+v", proposed.Steps)
	}

	// Test AWS switch dialog opens and shows profiles.
	a.openCloudSwitch("aws", cliContext(a.cliState(), "aws"))
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Switch AWS profile", "default", "staging", "Review plan…")
}

func TestCliMissingInvalidAmbiguousStates(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "cli", Cli: &inventory.CliState{
			Tools: []inventory.Installation{
				{Tool: "aws", Version: "2.22.1", Path: "/usr/local/bin/aws", OnPath: 1},
				{Tool: "gcloud", Version: "503.0.0", Path: "/usr/local/bin/gcloud", OnPath: 1},
				{Tool: "az", Version: "2.67.0", Path: "/usr/local/bin/az", OnPath: 1},
			},
			Contexts: []inventory.CliContext{
				{Tool: "aws", Name: "staging", NameSource: "AWS_PROFILE", ConfigEnv: "AWS_CONFIG_FILE", File: "/tmp/aws.cfg", Status: "ambiguous", Issue: "env_creds"},
				{Tool: "gcloud", Name: "work", NameSource: "CLOUDSDK_ACTIVE_CONFIG_NAME", ConfigEnv: "CLOUDSDK_CONFIG", File: "/tmp/gcloud", Status: "missing", Issue: "active_config_missing"},
				{Tool: "az", File: "/tmp/azureProfile.json", ConfigEnv: "AZURE_CONFIG_DIR", Status: "invalid", Issue: "config_invalid"},
			},
		}},
	}}
	a.router.Push(cliPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt,
		"config from AWS_CONFIG_FILE", "environment credentials override profile",
		"config from CLOUDSDK_CONFIG", "active configuration file missing",
		"config from AZURE_CONFIG_DIR", "config file could not be read",
	)
}
