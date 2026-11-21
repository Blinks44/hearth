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

func TestDotnetRowsPreserveSourceRoot(t *testing.T) {
	miseRoot := home + "/.local/share/mise/installs/dotnet/9.0.102"
	sdkPath := miseRoot + "/sdk/9.0.102"
	rtPath := miseRoot + "/shared/Microsoft.NETCore.App/9.0.1"
	st := inventory.ToolState{Tool: "dotnet", Installations: []inventory.Installation{{
		Tool: "dotnet", Version: "9.0.102", Path: miseRoot + "/dotnet", RealPath: miseRoot + "/dotnet", OnPath: 1,
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: miseRoot, Version: "9.0.102"},
		Dotnet: &inventory.Dotnet{
			SDKs:     []inventory.DotnetEntry{{Name: "SDK", Version: "9.0.102", Path: sdkPath}},
			Runtimes: []inventory.DotnetEntry{{Name: "Microsoft.NETCore.App", Version: "9.0.1", Path: rtPath}},
		},
	}}}
	ins, active, sdks, runtimes := dotnetRows(st)
	if active != 0 || sdks != 1 || runtimes != 1 || len(ins) != 2 {
		t.Fatalf("rows=%d active=%d sdks=%d runtimes=%d", len(ins), active, sdks, runtimes)
	}
	if ins[0].Source.Root != miseRoot || ins[1].Source.Root != miseRoot {
		t.Fatalf("Source.Root overwritten: sdk=%q runtime=%q, want %q", ins[0].Source.Root, ins[1].Source.Root, miseRoot)
	}
	if got := displayPath(ins[0], home); got != tilde(sdkPath, home) {
		t.Errorf("sdk displayPath=%q, want %q", got, tilde(sdkPath, home))
	}
	if got := displayPath(ins[1], home); got != tilde(rtPath, home) {
		t.Errorf("runtime displayPath=%q, want %q", got, tilde(rtPath, home))
	}
}
