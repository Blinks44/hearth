package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// The known commands by group, found or not, with where they point and an
// update brew has; mise's tools without a page after them: the version
// that runs, else the default, and the others.
func TestCliPage(t *testing.T) {
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
			{Name: "zig", Installations: []inventory.Installation{cp("zig", "0.13.0", true, 0), cp("zig", "0.12.0", false, 0)}},
		}},
		{Tool: "cli", Cli: &inventory.CliState{
			Tools:     []inventory.Installation{brew("kubectl", "1.31.4"), brew("helm", "3.16.3"), brew("aws", "2.22.1"), brew("gcloud", "503.0.0"), brew("terraform", "1.9.8")},
			HelmRepos: 3,
			Contexts: []inventory.CliContext{
				{Tool: "kubectl", Name: "kind-dev", File: home + "/.kube/config"},
				{Tool: "aws", Name: "default", Region: "eu-central-1", File: home + "/.aws/config"},
				{Tool: "gcloud", File: home + "/.config/gcloud/configurations/config_default"},
			},
		}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
			Packages: []inventory.SystemPackage{{Name: "kubernetes-cli", Version: "1.31.4", Available: "1.32.0", Path: "/opt/homebrew/Cellar/kubectl/1.31.4"}}}}},
	}}
	a.router.Push(cliPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cli")
	hasTexts(t, tt, "CLI & clouds", "Tools · command line", "of 8 known", "CLOUDS", "INFRASTRUCTURE",
		"brew · profile default · eu-central-1",
		"brew · no project chosen", "503.0.0", "not found · Azure CLI", "not found · OpenTofu",
		"Current contexts", "aws · default", "from ~/.aws/config", "gcloud · no project chosen",
		"FROM MISE · NO PAGE OF THEIR OWN", "deno", "mise · in PATH", "2.1.4", "elixir", "mise · not set up", "zig",
		"mise · the default version · 1 more version: 0.12.0", "0.13.0", "mise's folders", "installs/ without running mise")
	// Kubernetes' tools and contexts have their own page.
	if tt.HasText("3.15.0") || tt.HasText("kubectl") || tt.HasText("KUBERNETES") || tt.HasText("kubectl · kind-dev") {
		t.Errorf("Kubernetes on CLI & clouds: %q", tt.Texts())
	}
	a.cliFilter = "zi"
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("deno") || !tt.HasText("aws · default") || !tt.HasText("zig") || tt.HasText("503.0.0") {
		t.Errorf("filter: %q", tt.Texts())
	}
	a.cliFilter = ""
	a.cliTab = 1 // clouds
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("aws") || tt.HasText("1.9.8") || tt.HasText("deno") {
		t.Errorf("clouds tab: %q", tt.Texts())
	}
}
