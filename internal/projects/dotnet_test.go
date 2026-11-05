package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestDotnetSDKSelection(t *testing.T) {
	no := false
	for _, tt := range []struct {
		name, version, roll, want string
		pre                       *bool
		bad                       bool
	}{
		{"default exact", "8.0.400", "", "8.0.400", nil, false},
		{"default patch fallback", "8.0.401", "", "8.0.405", nil, false},
		{"latest patch", "8.0.400", "latestPatch", "8.0.405", nil, false},
		{"feature nearest", "8.0.100", "feature", "8.0.405", nil, false},
		{"latest feature", "8.0.100", "latestFeature", "8.0.501", nil, false},
		{"minor nearest", "8.0.100", "minor", "8.0.405", nil, false},
		{"major nearest", "7.0.100", "major", "8.0.405", nil, false},
		{"latest major", "8.0.100", "latestMajor", "10.0.100-preview.1", nil, false},
		{"no preview", "8.0.100", "latestMajor", "9.0.102", &no, false},
		{"disabled missing", "8.0.401", "disable", "", nil, false},
		{"no rollback", "9.0.200", "patch", "", nil, false},
		{"latest without pin", "", "", "10.0.100-preview.1", nil, false},
		{"runtime is not SDK", "6.0.428", "", "", nil, false},
		{"invalid version", "8.0", "", "", nil, true},
		{"invalid policy", "8.0.400", "surprise", "", nil, true},
		{"policy needs version", "", "feature", "", nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Project{Requirements: []Requirement{{Tool: "dotnet", Want: tt.version, File: "global.json", Key: "sdk.version", SDK: &SDKPolicy{Version: tt.version, RollForward: tt.roll, AllowPrerelease: tt.pre}}}}
			info := &inventory.Dotnet{Runtimes: []inventory.DotnetEntry{{Version: "6.0.428"}}}
			for _, v := range []string{"8.0.400", "8.0.405", "8.0.501", "9.0.102", "10.0.100-preview.1"} {
				info.SDKs = append(info.SDKs, inventory.DotnetEntry{Version: v, Path: "/sdk/" + v})
			}
			snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "dotnet", Installations: []inventory.Installation{{Tool: "dotnet", OnPath: 1, Dotnet: info}}}}}
			st := Resolve(p, snap, nil)[0]
			got := ""
			if st.Terminal != nil {
				got = st.Terminal.Version
			}
			if got != tt.want || (st.Checks[0].Err != "") != tt.bad || st.Checks[0].Met != (tt.want != "") {
				t.Fatalf("terminal %s checks %+v", got, st.Checks)
			}
		})
	}
}

func TestReadGlobalJSON(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nested")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "global.json")
	data := `{"sdk":{/* comment */"version":"8.0.400","rollForward":"latestPatch","allowPrerelease":false}}`
	if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := Read(dir); len(p.Requirements) != 0 {
		t.Fatalf("a folder without a .NET project inherits global.json: %+v", p)
	}
	if err := os.WriteFile(filepath.Join(dir, "App.csproj"), []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Read(dir)
	if len(p.Errs) != 0 || len(p.Requirements) != 1 || p.Requirements[0].SDK == nil || p.Requirements[0].SDK.RollForward != "latestPatch" {
		t.Fatalf("%+v", p)
	}
	if err := os.WriteFile(filepath.Join(dir, "global.json"), []byte(`{"sdk":{"version":"9.0.100"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p = Read(dir)
	if p.Requirements[0].Want != "9.0.100" {
		t.Fatalf("%+v", p)
	}
}

func TestDotnetHostIsolation(t *testing.T) {
	req := Requirement{Tool: "dotnet", Want: "8.0.400", SDK: &SDKPolicy{Version: "8.0.400"}}
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "dotnet", Installations: []inventory.Installation{
		{Tool: "dotnet", OnPath: 1, Dotnet: &inventory.Dotnet{SDKs: []inventory.DotnetEntry{{Version: "9.0.102"}}}},
		{Tool: "dotnet", Dotnet: &inventory.Dotnet{SDKs: []inventory.DotnetEntry{{Version: "8.0.405"}}}},
	}}}}
	st := Resolve(Project{Requirements: []Requirement{req}}, snap, nil)[0]
	if st.Terminal != nil || st.Checks[0].Met || st.Checks[0].Installed == nil {
		t.Fatalf("%+v", st)
	}
	// A failed list of a host off PATH leaves the check to the one on it.
	snap.Tools[0].Installations[1].Dotnet.Err = "--list-sdks: exit 1"
	if st := Resolve(Project{Requirements: []Requirement{req}}, snap, nil)[0]; !st.Checked {
		t.Fatalf("%+v", st)
	}
	snap.Tools[0].Installations[0].Dotnet.Err = "--list-sdks: exit 1"
	if st := Resolve(Project{Requirements: []Requirement{req}}, snap, nil)[0]; st.Checked {
		t.Fatalf("%+v", st)
	}
}

func TestDotnetUnknownPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, data string
		parseError bool
	}{
		{"paths", `{"sdk":{"version":"9.0.100","paths":[".dotnet","$host$"]}}`, false},
		{"invalid type", `{"sdk":{"allowPrerelease":"false"}}`, true},
		{"unterminated comment", "{} /*\n", true},
		{"line comment", "{// comment\n\"sdk\":{\"version\":\"9.0.100\"}}", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reqs, err := readGlobalJSON([]byte(tt.data))
			if (err != nil) != tt.parseError {
				t.Fatalf("reqs %+v error %v", reqs, err)
			}
			if tt.name == "paths" {
				snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "dotnet"}}}
				st := Resolve(Project{Requirements: reqs}, snap, nil)[0]
				if _, bad := st.Unmet(); bad || st.Checks[0].Err == "" {
					t.Fatalf("%+v", st)
				}
			}
		})
	}
}
