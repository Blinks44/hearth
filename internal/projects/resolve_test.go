package projects

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// node is an installation of Node by its manager, as "fnm", or "brew".
func node(manager, version string, onPath inventory.PathRank) inventory.Installation {
	kind := inventory.KindVersionManager
	if manager == "brew" {
		kind = inventory.KindPackageManager
	}
	return inventory.Installation{
		Tool: "node", Version: version, OnPath: onPath,
		Path:   "/" + manager + "/" + version + "/bin/node",
		Source: inventory.Source{Kind: kind, Manager: manager},
	}
}

var lts = map[int]versions.Line{
	20: {Codename: "Iron", LTS: time.Date(2023, 10, 24, 0, 0, 0, 0, time.UTC)},
	22: {Codename: "Jod", LTS: time.Date(2024, 10, 29, 0, 0, 0, 0, time.UTC)},
	23: {},
}

func TestResolve(t *testing.T) {
	fnmOnCD := inventory.ToolState{
		Tool:     "node",
		Managers: []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".node-version", ".nvmrc"}}},
		Installations: []inventory.Installation{
			node("brew", "23.3.0", 1),
			node("fnm", "20.11.0", 0),
			node("fnm", "20.18.1", 0),
			node("fnm", "22.12.0", 0),
		},
	}
	nvmNoHook := fnmOnCD
	nvmNoHook.Managers = []inventory.Manager{{Name: "nvm"}}
	nvmNoHook.Installations = []inventory.Installation{node("brew", "23.3.0", 1), node("nvm", "20.18.1", 0)}

	volta := inventory.ToolState{
		Tool:          "node",
		Managers:      []inventory.Manager{{Name: "volta", SwitchOnCD: true, Files: []string{"package.json"}}},
		Installations: []inventory.Installation{node("volta", "20.11.0", 0), node("volta", "22.12.0", 0)},
	}

	miseShim := inventory.ToolState{
		Tool:     "node",
		Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true, Files: []string{"mise.toml", ".tool-versions"}}},
		Installations: []inventory.Installation{
			{Tool: "node", Path: "/mise/shims/node", OnPath: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Shim: true}},
			node("mise", "20.18.1", 0),
			node("mise", "22.12.0", 0),
		},
	}
	miseShim.Installations[2].Default = true

	nvmrc := func(want string) Requirement { return Requirement{Tool: "node", Want: want, File: ".nvmrc"} }
	engines := func(want string) Requirement {
		return Requirement{Tool: "node", Want: want, File: "package.json", Key: "engines.node"}
	}

	tests := []struct {
		name  string
		tool  inventory.ToolState
		reqs  []Requirement
		lines map[int]versions.Line
		// Paths of the copies, "" for none.
		wantTerminal  string
		wantInstalled []string
		wantMet       []bool
		wantErr       bool
	}{
		{
			name: "fnm switches by .nvmrc", tool: fnmOnCD, reqs: []Requirement{nvmrc("20")},
			wantTerminal: "/fnm/20.18.1/bin/node", wantInstalled: []string{"/fnm/20.18.1/bin/node"}, wantMet: []bool{true},
		},
		{
			name: "fnm without the version leaves the one in use", tool: fnmOnCD, reqs: []Requirement{nvmrc("18")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{""}, wantMet: []bool{false},
		},
		{
			name: "engines alone leave the terminal on PATH", tool: fnmOnCD, reqs: []Requirement{engines(">=18 <21")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{"/fnm/20.18.1/bin/node"}, wantMet: []bool{false},
		},
		{
			name: "nvm without its hook does not switch", tool: nvmNoHook, reqs: []Requirement{nvmrc("v20.18.1")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{"/nvm/20.18.1/bin/node"}, wantMet: []bool{false},
		},
		{
			name: "lts/iron", tool: fnmOnCD, reqs: []Requirement{nvmrc("lts/iron")}, lines: lts,
			wantTerminal: "/fnm/20.18.1/bin/node", wantInstalled: []string{"/fnm/20.18.1/bin/node"}, wantMet: []bool{true},
		},
		{
			name: "lts/* is the newest LTS, not the newest", tool: fnmOnCD, reqs: []Requirement{engines("lts/*")}, lines: lts,
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{"/fnm/22.12.0/bin/node"}, wantMet: []bool{false},
		},
		{
			name: "lts/* without the release lines meets nothing", tool: fnmOnCD, reqs: []Requirement{nvmrc("lts/*")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{""}, wantMet: []bool{false},
		},
		{
			name: "system is a copy no version manager keeps", tool: fnmOnCD, reqs: []Requirement{engines("system")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{"/brew/23.3.0/bin/node"}, wantMet: []bool{true},
		},
		{
			name: "volta reads its field, not engines", tool: volta,
			reqs: []Requirement{
				engines(">=22"),
				{Tool: "node", Want: "20.11.0", File: "package.json", Key: "volta.node"},
			},
			wantTerminal:  "/volta/20.11.0/bin/node",
			wantInstalled: []string{"/volta/22.12.0/bin/node", "/volta/20.11.0/bin/node"},
			wantMet:       []bool{false, true},
		},
		{
			name: "a shim with no file to go by runs the default", tool: miseShim, reqs: []Requirement{engines("^20")},
			wantTerminal: "/mise/22.12.0/bin/node", wantInstalled: []string{"/mise/20.18.1/bin/node"}, wantMet: []bool{false},
		},
		{
			name: "a shim switches by mise.toml", tool: miseShim,
			reqs:         []Requirement{{Tool: "node", Want: "20", File: ".mise.toml", Key: "tools.node"}},
			wantTerminal: "/mise/20.18.1/bin/node", wantInstalled: []string{"/mise/20.18.1/bin/node"}, wantMet: []bool{true},
		},
		{
			name: "only needed", tool: fnmOnCD, reqs: []Requirement{{Tool: "node", File: "Dockerfile"}},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{"/brew/23.3.0/bin/node"}, wantMet: []bool{true},
		},
		{
			name: "a version it cannot read", tool: fnmOnCD, reqs: []Requirement{engines("~=20")},
			wantTerminal: "/brew/23.3.0/bin/node", wantInstalled: []string{""}, wantMet: []bool{false}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &inventory.Snapshot{Tools: []inventory.ToolState{tt.tool}}
			got := Resolve(Project{Requirements: tt.reqs}, s, tt.lines)
			if len(got) != 1 || got[0].Tool != "node" || !got[0].Checked {
				t.Fatalf("got %+v, want one checked status of node", got)
			}
			if p := pathOf(got[0].Terminal); p != tt.wantTerminal {
				t.Errorf("terminal %q, want %q", p, tt.wantTerminal)
			}
			for i, c := range got[0].Checks {
				if p := pathOf(c.Installed); p != tt.wantInstalled[i] {
					t.Errorf("check %d installed %q, want %q", i, p, tt.wantInstalled[i])
				}
				if c.Met != tt.wantMet[i] {
					t.Errorf("check %d met %v, want %v", i, c.Met, tt.wantMet[i])
				}
				if (c.Err != "") != tt.wantErr {
					t.Errorf("check %d err %q", i, c.Err)
				}
			}
		})
	}
}

func TestResolveUncheckedTools(t *testing.T) {
	s := &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "node", Installations: []inventory.Installation{node("brew", "22.12.0", 1)}},
		{Tool: "bun", Err: "timed out"},
	}}
	p := Project{Requirements: []Requirement{
		{Tool: "python", Want: "3.12", File: ".python-version"},
		{Tool: "node", Want: "22", File: ".nvmrc"},
		{Tool: "bun", File: "bun.lock"},
		{Tool: "python", Want: ">=3.12", File: "pyproject.toml"},
	}}
	got := Resolve(p, s, nil)
	want := []struct {
		tool    string
		checked bool
		checks  int
	}{{"python", false, 2}, {"node", true, 1}, {"bun", false, 1}}
	if len(got) != len(want) {
		t.Fatalf("got %d statuses, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Tool != w.tool || got[i].Checked != w.checked || len(got[i].Checks) != w.checks {
			t.Errorf("status %d: %s checked %v with %d checks, want %+v", i, got[i].Tool, got[i].Checked, len(got[i].Checks), w)
		}
	}
	if !got[1].Checks[0].Met {
		t.Error("node 22 is not met by 22.12.0")
	}
}

func pathOf(in *inventory.Installation) string {
	if in == nil {
		return ""
	}
	return in.Path
}
