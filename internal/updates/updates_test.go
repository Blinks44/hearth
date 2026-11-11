package updates

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

var now = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// lists reads the fixtures: Node's index and schedule of 2026-11-04,
// Python's lines, Go's releases; .NET's, Java's and Bun's are written out.
func lists(t *testing.T) Lists {
	t.Helper()
	var node versions.Node
	var err error
	if node.Releases, err = versions.ParseNodeIndex(fixture(t, "node-index.json")); err != nil {
		t.Fatal(err)
	}
	if node.Lines, err = versions.ParseNodeSchedule(fixture(t, "node-schedule.json")); err != nil {
		t.Fatal(err)
	}
	python := versions.Python{}
	if python.Lines, err = versions.ParsePythonCycles(fixture(t, "python-cycles.json")); err != nil {
		t.Fatal(err)
	}
	goReleases, err := versions.ParseGoReleases(fixture(t, "go-releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	return Lists{
		Node:   &node,
		Python: &python,
		Go:     &versions.Go{Releases: goReleases},
		Java: &versions.Java{Lines: []versions.Cycle{
			{Cycle: "27", Latest: "27.0.1.0.2+1"},
			{Cycle: "25", Latest: "25.0.5+8", LTS: true},
			{Cycle: "21", Latest: "21.0.9+10", LTS: true},
		}},
		Dotnet: &versions.Dotnet{Lines: []versions.DotnetLine{
			{Version: "11.0.100-preview.1", Phase: "preview"},
			{Version: "10.0.100", Phase: "active"},
			{Version: "9.0.300", Phase: "active"},
			{Version: "8.0.400", Phase: "active"},
			{Version: "6.0.428", Phase: "eol"},
		}},
		Rust: &versions.Rust{Lines: []versions.Cycle{{Cycle: "1.99", Latest: "1.99.1"}, {Cycle: "1.98", Latest: "1.98.1"}}},
		Bun:  &versions.Latest{Version: "1.4.4"},
	}
}

func copyOf(tool, v string, kind inventory.Kind, manager string) inventory.Installation {
	return inventory.Installation{Tool: tool, Version: v, Path: "/" + manager + "/" + tool + "/" + v,
		Source: inventory.Source{Kind: kind, Manager: manager}}
}

func fnm(v string) inventory.Installation {
	return copyOf("node", v, inventory.KindVersionManager, "fnm")
}

func toolchain(name, v string) inventory.Installation {
	in := copyOf("rust", v, inventory.KindVersionManager, "rustup")
	in.Rust = &inventory.RustToolchain{Name: name}
	return in
}

func project(tool, want string) projects.Project {
	return projects.Project{Dir: "/code/app", Requirements: []projects.Requirement{{Tool: tool, Want: want}}}
}

func patch(in inventory.Installation, to, lts string) Update {
	return Update{Kind: Patch, Tool: in.Tool, Copy: in, Version: to, LTS: lts}
}

func newLine(tool, v, lts string) Update {
	return Update{Kind: NewLine, Tool: tool, Version: v, LTS: lts}
}

func TestFind(t *testing.T) {
	stub := copyOf("python", "3.13.0", inventory.KindInstaller, "")
	stub.Stub = true
	shim := copyOf("node", "", inventory.KindVersionManager, "mise")
	shim.Source.Shim = true
	tests := []struct {
		name   string
		tool   string
		copies []inventory.Installation
		ps     []projects.Project
		lists  func(Lists) Lists
		want   []Update
	}{
		{
			name:   "patch in the line and the new LTS line apart",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0")},
			want:   []Update{patch(fnm("22.12.0"), "22.23.4", "Jod"), newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "on the newest LTS line: only the patch",
			tool:   "node",
			copies: []inventory.Installation{fnm("26.12.1")},
			want:   []Update{patch(fnm("26.12.1"), "26.13.0", "Lithium")},
		},
		{
			name:   "up to date",
			tool:   "node",
			copies: []inventory.Installation{fnm("26.13.0")},
		},
		{
			name:   "end of support with the line's last release: the new line only",
			tool:   "node",
			copies: []inventory.Installation{fnm("18.20.8")},
			want:   []Update{newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "end of support with a newer release in the line",
			tool:   "node",
			copies: []inventory.Installation{fnm("25.8.2")},
			want:   []Update{patch(fnm("25.8.2"), "25.9.0", ""), newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "the patch is installed already",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0"), fnm("22.23.4")},
			want:   []Update{newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "a project pins the exact version",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0")},
			ps:     []projects.Project{project("node", "22.12.0")},
			want:   []Update{newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "a project asks for the line",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0")},
			ps:     []projects.Project{project("node", "22")},
			want:   []Update{patch(fnm("22.12.0"), "22.23.4", "Jod"), newLine("node", "26.13.0", "Lithium")},
		},
		{
			name:   "a project pinning another copy does not hold this one",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0"), fnm("26.12.1")},
			ps:     []projects.Project{project("node", "26.12.1")},
			want:   []Update{patch(fnm("22.12.0"), "22.23.4", "Jod")},
		},
		{
			name: "system copies, shims and copies of no known source get none",
			tool: "node",
			copies: []inventory.Installation{
				copyOf("node", "22.12.0", inventory.KindSystem, "apt"),
				copyOf("node", "20.20.1", inventory.KindUnknown, ""),
				shim,
			},
		},
		{
			name:   "no list",
			tool:   "node",
			copies: []inventory.Installation{fnm("22.12.0")},
			lists:  func(Lists) Lists { return Lists{} },
		},
		{
			name:   "Python: patch and new line",
			tool:   "python",
			copies: []inventory.Installation{copyOf("python", "3.12.7", inventory.KindPackageManager, "brew")},
			want: []Update{
				patch(copyOf("python", "3.12.7", inventory.KindPackageManager, "brew"), "3.12.12", ""),
				newLine("python", "3.14.2", ""),
			},
		},
		{
			name:   "Python: an ended line's last release",
			tool:   "python",
			copies: []inventory.Installation{copyOf("python", "3.8.20", inventory.KindVersionManager, "pyenv")},
			want:   []Update{newLine("python", "3.14.2", "")},
		},
		{
			name: "Python: prereleases and stand-ins get none",
			tool: "python",
			copies: []inventory.Installation{
				copyOf("python", "3.15.0a1", inventory.KindVersionManager, "uv"),
				stub,
			},
		},
		{
			name:   "Go: line by minor",
			tool:   "go",
			copies: []inventory.Installation{copyOf("go", "1.26.3", inventory.KindVersionManager, "mise")},
			want: []Update{
				patch(copyOf("go", "1.26.3", inventory.KindVersionManager, "mise"), "1.26.10", ""),
				newLine("go", "1.27.3", ""),
			},
		},
		{
			name:   ".NET: previews and ended lines are not new lines",
			tool:   "dotnet",
			copies: []inventory.Installation{copyOf("dotnet", "8.0.100", inventory.KindInstaller, "")},
			want: []Update{
				patch(copyOf("dotnet", "8.0.100", inventory.KindInstaller, ""), "8.0.400", ""),
				newLine("dotnet", "10.0.100", ""),
			},
		},
		{
			name:   "Java: the new line is the newest LTS; Temurin's build numbers",
			tool:   "java",
			copies: []inventory.Installation{copyOf("java", "21.0.5", inventory.KindVersionManager, "sdkman")},
			want: []Update{
				patch(copyOf("java", "21.0.5", inventory.KindVersionManager, "sdkman"), "21.0.9+10", ""),
				newLine("java", "25.0.5+8", ""),
			},
		},
		{
			name:   "Bun: no lines",
			tool:   "bun",
			copies: []inventory.Installation{copyOf("bun", "1.2.5", inventory.KindInstaller, inventory.ScriptInstall)},
			want:   []Update{patch(copyOf("bun", "1.2.5", inventory.KindInstaller, inventory.ScriptInstall), "1.4.4", "")},
		},
		{
			name:   "Rust: the stable toolchain moves in place, covering the pinned one's new line",
			tool:   "rust",
			copies: []inventory.Installation{toolchain("stable-aarch64-apple-darwin", "1.98.0"), toolchain("1.98.1-aarch64-apple-darwin", "1.98.1")},
			want:   []Update{patch(toolchain("stable-aarch64-apple-darwin", "1.98.0"), "1.99.1", "")},
		},
		{
			name:   "Rust: a pinned toolchain alone",
			tool:   "rust",
			copies: []inventory.Installation{toolchain("1.98.1-aarch64-apple-darwin", "1.98.1")},
			want:   []Update{newLine("rust", "1.99.1", "")},
		},
		{
			name:   "Ruby: no list",
			tool:   "ruby",
			copies: []inventory.Installation{copyOf("ruby", "3.3.0", inventory.KindVersionManager, "rbenv")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lists(t)
			if tt.lists != nil {
				l = tt.lists(l)
			}
			s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: tt.tool, Installations: tt.copies}}}
			got := Find(s, tt.ps, l, now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func miseCopy(tool, v string) inventory.Installation {
	return copyOf(tool, v, inventory.KindVersionManager, "mise")
}

func TestFindMise(t *testing.T) {
	l := Lists{Mise: &versions.Mise{Tools: map[string]versions.MiseTool{
		"go":           {Current: "1.26.3", InRange: "1.26.10", Latest: "1.27.3"},
		"jq":           {Current: "1.7", Latest: "1.8.2"},
		"shellcheck":   {Current: "0.9.0", Latest: "0.11.0"},
		"npm:prettier": {Current: "3.5.0", InRange: "3.6.2", Latest: "3.6.2"},
		"zig":          {Current: "0.13.0", Latest: "0.15.1"},
	}}}
	tools := []inventory.MiseTool{
		{Name: "jq", Installations: []inventory.Installation{miseCopy("jq", "1.7")}},
		{Name: "shellcheck", Installations: []inventory.Installation{miseCopy("shellcheck", "0.9.0")}},
		{Name: "npm-prettier", Installations: []inventory.Installation{miseCopy("npm-prettier", "3.5.0")}},
		{Name: "zig", Installations: []inventory.Installation{miseCopy("zig", "0.15.1"), miseCopy("zig", "0.13.0")}},
		{Name: "terraform", Installations: []inventory.Installation{miseCopy("terraform", "1.9.0")}},
	}
	s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "mise", MiseTools: tools}}}
	mise := func(u Update, name string) Update {
		u.Mise = name
		return u
	}
	want := []Update{
		// jq = "1.7" in the config allows no newer 1.7.
		mise(newLine("jq", "1.8.2", ""), "jq"),
		// shellcheck is in no config: the new version goes in beside.
		mise(newLine("shellcheck", "0.11.0", ""), "shellcheck"),
		// Its folder is npm-prettier; the newest overall is in the range.
		mise(patch(miseCopy("npm-prettier", "3.5.0"), "3.6.2", ""), "npm:prettier"),
		// zig's newest is installed already; terraform is up to date.
	}
	if got := Find(s, nil, l, now); !reflect.DeepEqual(got, want) {
		t.Errorf("Find() =\n%+v\nwant\n%+v", got, want)
	}
	if got := Find(s, nil, Lists{}, now); got != nil {
		t.Errorf("Find() without mise's answer = %+v", got)
	}
}

// A package manager's copy is the package's to update: its own, or an
// installer's whose package it lists at the copy's version.
func TestHeld(t *testing.T) {
	winget := []inventory.PackageManager{{ID: "winget", Packages: []inventory.SystemPackage{
		{Name: "GoLang.Go", Version: "1.25.4", Available: "1.25.5"},
		{Name: "Microsoft.DotNet.SDK.10", Version: "10.0.100"},
	}}}
	installer := func(tool, v string) inventory.Installation { return copyOf(tool, v, inventory.KindInstaller, "") }
	for _, tc := range []struct {
		name string
		in   inventory.Installation
		want bool
	}{
		{"brew's", copyOf("node", "22.12.0", inventory.KindPackageManager, "brew"), true},
		{"WinGet's Go", installer("go", "1.25.4"), true},
		{"WinGet's .NET", installer("dotnet", "10.0.100"), true},
		{"another version", installer("go", "1.24.0"), false},
		{"no package", installer("python", "3.13.1"), false},
		{"a known installer", copyOf("bun", "1.2.0", inventory.KindInstaller, inventory.ScriptInstall), false},
		{"a version manager's", fnm("1.25.4"), false},
	} {
		if got := Held(tc.in, winget); got != tc.want {
			t.Errorf("%s: Held() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFindSkipsFailedTools(t *testing.T) {
	s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Err: "timeout", Installations: []inventory.Installation{fnm("22.12.0")}}}}
	if got := Find(s, nil, lists(t), now); got != nil {
		t.Errorf("Find() = %+v, want none", got)
	}
	if got := Find(nil, nil, lists(t), now); got != nil {
		t.Errorf("Find(nil) = %+v, want none", got)
	}
}
