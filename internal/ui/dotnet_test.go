package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

func dotnetSnapshot() *inventory.Snapshot {
	root := "/usr/local/share/dotnet"
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "dotnet", Vars: map[string]string{}, Installations: []inventory.Installation{{
		Tool: "dotnet", Version: "9.0.102", Path: root + "/dotnet", RealPath: root + "/dotnet", OnPath: 1, InGUI: 1,
		Source: inventory.Source{Kind: inventory.KindInstaller},
		Dotnet: &inventory.Dotnet{SDKs: []inventory.DotnetEntry{{Name: "SDK", Version: "9.0.102", Path: root + "/sdk/9.0.102"}, {Name: "SDK", Version: "8.0.405", Path: root + "/sdk/8.0.405"}},
			Runtimes: []inventory.DotnetEntry{{Name: "Microsoft.NETCore.App", Version: "9.0.1", Path: root + "/shared/Microsoft.NETCore.App/9.0.1"}, {Name: "Microsoft.AspNetCore.App", Version: "9.0.1", Path: root + "/shared/Microsoft.AspNetCore.App/9.0.1"}}},
		Packages: []inventory.Package{{Name: "dotnet-ef", Version: "9.0.1", Commands: []string{"dotnet-ef"}}, {Name: "csharpier", Version: "0.30.6"}},
	}}}}}
}
func openDotnet(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/dotnet")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}
func TestDotnetPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = dotnetSnapshot()
	a.DotnetVersions = &versions.Dotnet{Lines: []versions.DotnetLine{{Version: "9.0.200", Support: "sts", Phase: "active"}, {Version: "8.0.406", Support: "lts", Phase: "maintenance"}, {Version: "10.0.100-preview.1", Support: "lts", Phase: "preview"}}}
	a.Projects = []projects.Project{
		{Dir: home + "/dev/crm-api", Requirements: []projects.Requirement{{Tool: "dotnet", File: "global.json", Want: "8.0.400", SDK: &projects.SDKPolicy{Version: "8.0.400", RollForward: "latestPatch"}}}},
		{Dir: home + "/dev/nuget-tools", Requirements: []projects.Requirement{{Tool: "dotnet", File: "global.json", Want: "6.0.428", SDK: &projects.SDKPolicy{Version: "6.0.428"}}}},
	}
	tt := openDotnet(a)
	screenshot(t, tt, "dotnet")
	for _, text := range []string{"2 SDKs", "+ 2 runtimes", "SDK 9.0.102", "SDK 8.0.405", "Runtime 9.0.1", "Which SDK dotnet selects", "No compatible SDK is installed for nuget-tools", "8.0.405", "SDK 9.0.200", "DOTNET_ROOT", "dotnet-ef"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("How the terminal finds dotnet") || tt.HasText("dotnet-ef") {
		t.Fatalf("%q", tt.Texts())
	}
}
func TestDotnetEmpty(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "dotnet"}}}
	tt := openDotnet(a)
	if !tt.HasText(".NET is not installed") {
		t.Fatalf("%q", tt.Texts())
	}
}
