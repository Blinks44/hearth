package catalog

import (
	"errors"
	"slices"
	"testing"
)

func TestEveryToolInstallsOnItsSystems(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Tools {
		if seen[e.ID] {
			t.Errorf("%s is in the catalog twice", e.ID)
		}
		seen[e.ID] = true
		if About(e.ID) == "" {
			t.Errorf("%s has no description", e.ID)
		}
		for _, goos := range e.Systems() {
			if len(e.On(goos)) == 0 {
				t.Errorf("%s has no way to install on %s", e.ID, goos)
			}
		}
		for _, m := range e.Methods {
			if m.Package == "" {
				t.Errorf("%s: a method through %s names no package", e.ID, m.Manager)
			}
			if m.Manager == "brew" && m.Kind != "formula" && m.Kind != "cask" {
				t.Errorf("%s: Homebrew's %s is neither formula nor cask", e.ID, m.Package)
			}
		}
	}
}

func TestNeedsInstallOnTheSameSystem(t *testing.T) {
	for _, e := range Tools {
		for _, goos := range e.Systems() {
			for _, m := range e.On(goos) {
				for _, need := range e.NeedsWith(m) {
					n, ok := Find(need)
					if !ok {
						t.Errorf("%s through %s needs %s, which is not in the catalog", e.ID, m.Manager, need)
						continue
					}
					if !slices.Contains(n.Systems(), goos) {
						t.Errorf("%s through %s on %s needs %s, which does not install there", e.ID, m.Manager, goos, need)
					}
				}
			}
		}
	}
}

// TestNeedsHaveNoCycle follows what each tool needs through the method
// recommended on each system: a tool that needs itself would never
// install.
func TestNeedsHaveNoCycle(t *testing.T) {
	for _, goos := range All {
		var visit func(id string, path []string)
		visit = func(id string, path []string) {
			if slices.Contains(path, id) {
				t.Errorf("on %s: %v needs %s again", goos, path, id)
				return
			}
			e, _ := Find(id)
			methods := e.On(goos)
			if len(methods) == 0 {
				return
			}
			for _, need := range e.NeedsWith(methods[0]) {
				visit(need, append(path, id))
			}
		}
		for _, e := range Tools {
			visit(e.ID, nil)
		}
	}
}

func TestRecommended(t *testing.T) {
	tests := []struct {
		id, goos, manager string
		needs             []string
	}{
		{"node", "darwin", "mise", []string{"mise"}},
		{"mise", "darwin", "brew", []string{"homebrew"}},
		{"mise", "windows", "winget", nil},
		{"mise", "linux", "pacman", nil},
		{"pnpm", "linux", "mise", []string{"node", "mise"}},
		{"rust", "windows", "winget", nil},
		{"rust", "linux", "installer", nil},
		{"php", "windows", "winget", nil},
		{"git", "darwin", "xcode-select", nil},
		{"docker", "darwin", "brew", []string{"homebrew"}},
		{"codex", "windows", "npm", []string{"node"}},
		{"claude", "linux", "installer", nil},
		{"claude", "windows", "winget", nil},
		{"gemini", "linux", "npm", []string{"node"}},
		{"composer", "darwin", "brew", []string{"php", "homebrew"}},
		{"composer", "windows", "scoop", []string{"php"}},
		{"composer", "linux", "apt", []string{"php"}},
		{"uv", "windows", "mise", []string{"mise"}},
		{"pipx", "darwin", "mise", []string{"python", "mise"}},
		{"bundler", "linux", "mise", []string{"ruby", "mise"}},
		{"maven", "windows", "mise", []string{"java", "mise"}},
		{"gradle", "darwin", "mise", []string{"java", "mise"}},
		{"postgresql", "darwin", "brew", []string{"homebrew"}},
		{"postgresql", "windows", "winget", nil},
		{"postgresql", "linux", "apt", nil},
		{"mongodb", "darwin", "brew", []string{"homebrew"}},
	}
	for _, tt := range tests {
		t.Run(tt.id+" on "+tt.goos, func(t *testing.T) {
			e, ok := Find(tt.id)
			if !ok {
				t.Fatalf("%s is not in the catalog", tt.id)
			}
			m := e.On(tt.goos)[0]
			if m.Manager != tt.manager {
				t.Errorf("recommended through %s, want %s", m.Manager, tt.manager)
			}
			if got := e.NeedsWith(m); !slices.Equal(got, tt.needs) {
				t.Errorf("needs %v, want %v", got, tt.needs)
			}
		})
	}
}

func TestToolsNotOnASystem(t *testing.T) {
	for _, tt := range []struct{ id, goos string }{{"homebrew", "windows"}, {"homebrew", "linux"}, {"cursor", "linux"}, {"kimi", "darwin"}, {"redis", "windows"}, {"mariadb", "windows"}, {"mongodb", "linux"}} {
		e, _ := Find(tt.id)
		if slices.Contains(e.Systems(), tt.goos) {
			t.Errorf("%s installs on %s", tt.id, tt.goos)
		}
	}
	if About("nothing") != "" {
		t.Error("a tool not in the catalog has a description")
	}
}

// Each engine the wizard offers is one of Engines, with its name.
func TestDockerEngines(t *testing.T) {
	e, _ := Find("docker")
	for _, m := range e.Methods {
		if m.Engine == "" {
			t.Errorf("no engine name: %+v", m)
		}
		if got, ok := EngineMethod(m.Engine, m.Manager); !ok || got.Package != m.Package {
			t.Errorf("%s through %s: %+v", m.Engine, m.Manager, got)
		}
	}
}

// Kiri is in no system manager: Hearth only links its download page.
func TestEngineKiri(t *testing.T) {
	for _, manager := range []string{"brew", "winget", "apt", "dnf"} {
		if _, ok := EngineMethod("Kiri", manager); ok {
			t.Errorf("Kiri through %s", manager)
		}
	}
	if m, ok := EngineMethod("Kiri", "installer"); !ok || m.Package != "https://runkiri.com/releases/" {
		t.Errorf("Kiri's page: %+v", m)
	}
}

// Each database has its kind, and what it clashes with clashes with it.
func TestDatabases(t *testing.T) {
	for _, e := range Tools {
		if e.Group != Database {
			continue
		}
		if DatabaseKinds[e.ID] == "" {
			t.Errorf("%s has no kind", e.ID)
		}
		for _, id := range e.Conflicts {
			if other, _ := Find(id); !slices.Contains(other.Conflicts, e.ID) {
				t.Errorf("%s clashes with %s, not back", e.ID, id)
			}
		}
	}
}

// A set read back is the tools saved, in the catalog's order, without
// those a newer Hearth knows; a file Hearth did not save, or a newer one,
// is not read.
func TestSet(t *testing.T) {
	data, err := MarshalSet([]string{"claude", "node", "homebrew"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data string
		want []string
		err  error
	}{
		{"saved", string(data), []string{"homebrew", "node", "claude"}, nil},
		{"unknown tool", `{"hearth": "set", "version": 1, "tools": ["node", "zig"]}`, []string{"node"}, nil},
		{"other json", `{"tools": ["node"]}`, nil, ErrNotSet},
		{"not json", "node\n", nil, ErrNotSet},
		{"newer", `{"hearth": "set", "version": 2, "tools": ["node"]}`, nil, ErrNewerSet},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSet([]byte(tc.data))
			if !errors.Is(err, tc.err) || !slices.Equal(got, tc.want) {
				t.Errorf("ParseSet = %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}
}
