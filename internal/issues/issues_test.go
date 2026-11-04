package issues

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// abs makes a Unix path absolute on this OS too: on Windows it has no
// volume and would count as relative.
func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return `C:` + filepath.FromSlash(p)
}

var now = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func brew(v string, term, gui inventory.PathRank) inventory.Installation {
	return inventory.Installation{Tool: "node", Version: v, Path: "/opt/homebrew/bin/node", OnPath: term, InGUI: gui,
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
}

func fnm(v string, term, gui inventory.PathRank) inventory.Installation {
	return inventory.Installation{Tool: "node", Version: v, Path: "/fnm/v" + v + "/bin/node", OnPath: term, InGUI: gui,
		Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}}
}

func nodeSnap(ins ...inventory.Installation) *inventory.Snapshot {
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Installations: ins}}}
}

// nodeLines has Node 18 out of support at now, and 22 supported.
var nodeLines = &versions.Node{Lines: map[int]versions.Line{
	18: {LTS: time.Date(2022, 10, 25, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC), Codename: "Hydrogen"},
	22: {LTS: time.Date(2024, 10, 29, 0, 0, 0, 0, time.UTC), End: time.Date(2027, 4, 30, 0, 0, 0, 0, time.UTC), Codename: "Jod"},
}}

func TestFind(t *testing.T) {
	project := func(want string) []projects.Project {
		return []projects.Project{{Dir: "/src/app", Requirements: []projects.Requirement{{Tool: "node", Want: want, File: ".nvmrc"}}}}
	}
	tests := []struct {
		name     string
		snap     *inventory.Snapshot
		projects []projects.Project
		node     *versions.Node
		want     []Issue
	}{
		{name: "no scan yet"},
		{name: "one copy everywhere", snap: nodeSnap(brew("22.12.0", 1, 1)), node: nodeLines},

		{name: "duplicate on PATH", snap: nodeSnap(fnm("22.12.0", 2, 0), brew("23.6.0", 1, 1)),
			want: []Issue{{Kind: Duplicate, Severity: Warning, Tool: "node",
				Copies: []inventory.Installation{brew("23.6.0", 1, 1), fnm("22.12.0", 2, 0)}}}},
		{name: "other copy only in a manager's directory", snap: nodeSnap(brew("22.12.0", 1, 1), fnm("20.18.1", 0, 0))},

		{name: "apps run another version", snap: nodeSnap(fnm("22.12.0", 1, 0), brew("23.6.0", 0, 1)),
			want: []Issue{{Kind: GUIDiffers, Severity: Warning, Tool: "node",
				Copies: []inventory.Installation{fnm("22.12.0", 1, 0), brew("23.6.0", 0, 1)}}}},
		{name: "apps do not find it", snap: nodeSnap(fnm("22.12.0", 1, 0)),
			want: []Issue{{Kind: GUIDiffers, Severity: Warning, Tool: "node",
				Copies: []inventory.Installation{fnm("22.12.0", 1, 0)}}}},
		{name: "apps run the same version elsewhere", snap: nodeSnap(fnm("22.12.0", 1, 0), brew("22.12.0", 0, 1))},
		{name: "not in the terminal", snap: nodeSnap(brew("22.12.0", 0, 1))},

		{name: "project needs a version not installed", snap: nodeSnap(brew("22.12.0", 1, 1)), projects: project("20"),
			want: []Issue{{Kind: ProjectMissing, Severity: Warning, Tool: "node", Project: "/src/app",
				Check:  projects.Check{Tool: "node", Want: "20", File: ".nvmrc"},
				Copies: []inventory.Installation{brew("22.12.0", 1, 1)}}}},
		{name: "project's version installed, terminal runs another",
			snap: nodeSnap(brew("22.12.0", 1, 1), fnm("20.18.1", 0, 0)), projects: project("20"),
			want: []Issue{{Kind: ProjectTerminal, Severity: Warning, Tool: "node", Project: "/src/app",
				Check: func() projects.Check {
					in := fnm("20.18.1", 0, 0)
					return projects.Check{Tool: "node", Want: "20", File: ".nvmrc", Installed: &in}
				}(),
				Copies: []inventory.Installation{brew("22.12.0", 1, 1)}}}},
		{name: "project's version runs", snap: nodeSnap(brew("22.12.0", 1, 1)), projects: project("22")},
		{name: "project's tool not checked", snap: &inventory.Snapshot{}, projects: project("20")},

		{name: "terminal runs a version out of support", snap: nodeSnap(brew("18.20.4", 1, 1)), node: nodeLines,
			want: []Issue{{Kind: Unsupported, Severity: Warning, Tool: "node",
				Copies: []inventory.Installation{brew("18.20.4", 1, 1)}}}},
		{name: "a version out of support is only installed",
			snap: nodeSnap(brew("22.12.0", 1, 1), fnm("18.20.4", 0, 0)), node: nodeLines,
			want: []Issue{{Kind: Unsupported, Severity: Info, Tool: "node",
				Copies: []inventory.Installation{fnm("18.20.4", 0, 0)}}}},
		{name: "support not known", snap: nodeSnap(brew("18.20.4", 1, 1))},

		{name: "current folder on PATH",
			snap: &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: abs("/usr/bin")}, {Dir: "."}, {Dir: ""}}, GUIPath: []inventory.PathEntry{{Dir: "."}}},
			want: []Issue{{Kind: PathCurrentDir, Severity: Warning, Dirs: []string{".", ""}}}},
		{name: "missing directory on PATH",
			snap: &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: abs("/usr/bin")}, {Dir: abs("/opt/gone"), Missing: true}}, GUIPath: []inventory.PathEntry{{Dir: abs("/opt/gone"), Missing: true}}},
			want: []Issue{{Kind: PathMissing, Severity: Info, Dirs: []string{abs("/opt/gone")}}}},
		{name: "macOS's own missing directories on PATH", snap: &inventory.Snapshot{TerminalPath: []inventory.PathEntry{
			{Dir: "/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin", Missing: true},
			{Dir: "/pkg/env/global/bin", Missing: true}}}},
		{name: "PATH in order", snap: &inventory.Snapshot{TerminalPath: []inventory.PathEntry{{Dir: abs("/usr/bin")}}, GUIPath: []inventory.PathEntry{{Dir: abs("/usr/bin")}}}},

		{name: "tool that failed to check",
			snap: &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "node", Err: "timeout",
				Installations: []inventory.Installation{fnm("22.12.0", 1, 0), brew("23.6.0", 2, 0)}}}}},
	}
	for _, tt := range tests {
		got := Find(tt.snap, tt.projects, tt.node, now)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", tt.name, got, tt.want)
		}
	}
}

func TestSkillIssues(t *testing.T) {
	path := func(p string, agents ...string) inventory.SkillPath {
		return inventory.SkillPath{Path: p, Agents: agents}
	}
	skill := func(name, dir, sum string, paths ...inventory.SkillPath) inventory.Skill {
		return inventory.Skill{Name: name, Dir: dir, Sum: sum, Paths: paths}
	}
	claudePDF := skill("pdf", "/h/.claude/skills/pdf", "a", path("/h/.claude/skills/pdf", "claude"))
	sharedPDF := skill("pdf", "/h/.agents/skills/pdf", "a", path("/h/.agents/skills/pdf", "codex"))
	otherPDF := skill("pdf", "/h/.agents/skills/pdf", "b", path("/h/.agents/skills/pdf", "codex"))
	linked := skill("plan", "/h/.agents/skills/plan", "c",
		path("/h/.claude/skills/plan", "claude", "cursor"), path("/h/.agents/skills/plan", "codex", "cursor"))
	broken := inventory.Skill{Name: "pdf", Dir: "/gone", Broken: true, Paths: []inventory.SkillPath{path("/h/.claude/skills/pdf", "claude")}}
	tests := []struct {
		name   string
		skills []inventory.Skill
		want   []Issue
	}{
		{name: "one copy", skills: []inventory.Skill{claudePDF}},
		{name: "same copies", skills: []inventory.Skill{claudePDF, sharedPDF},
			want: []Issue{{Kind: SkillCopies, Severity: Info, Skills: []inventory.Skill{claudePDF, sharedPDF}}}},
		{name: "copies differ", skills: []inventory.Skill{claudePDF, otherPDF},
			want: []Issue{{Kind: SkillDiffers, Severity: Warning, Skills: []inventory.Skill{claudePDF, otherPDF}}}},
		{name: "broken link is no copy", skills: []inventory.Skill{claudePDF, broken}},
		{name: "one folder by two paths", skills: []inventory.Skill{linked}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Find(&inventory.Snapshot{Skills: tt.skills}, nil, nil, now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// A project's skills are compared among themselves, not with the user's.
func TestProjectSkillIssues(t *testing.T) {
	user := inventory.Skill{Name: "pdf", Dir: "/h/.agents/skills/pdf", Sum: "a"}
	mine := inventory.Skill{Name: "pdf", Dir: "/src/app/.agents/skills/pdf", Sum: "b", Project: "/src/app"}
	mine2 := inventory.Skill{Name: "pdf", Dir: "/src/app/.claude/skills/pdf", Sum: "b", Project: "/src/app"}
	snap := &inventory.Snapshot{Skills: []inventory.Skill{user}}
	if got := Find(snap, []projects.Project{{Dir: "/src/app", Skills: []inventory.Skill{mine}}}, nil, now); len(got) != 0 {
		t.Errorf("user and project: %+v", got)
	}
	got := Find(snap, []projects.Project{{Dir: "/src/app", Skills: []inventory.Skill{mine, mine2}}}, nil, now)
	want := []Issue{{Kind: SkillCopies, Severity: Info, Project: "/src/app", Skills: []inventory.Skill{mine, mine2}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestMCPMissing(t *testing.T) {
	server := func(name, agent, command, project string, missing, disabled bool) inventory.MCPServer {
		return inventory.MCPServer{Name: name, Agent: agent, Transport: "stdio", Command: command,
			Project: project, Missing: missing, Disabled: disabled}
	}
	found := server("github", "claude", "docker", "", false, false)
	gone := server("github", "claude", "docker", "", true, false)
	goneCursor := server("github", "cursor", "docker", "", true, false)
	goneUvx := server("fs", "codex", "uvx", "", true, false)
	off := server("pg", "codex", "pg-mcp", "", true, true)
	inProject := server("tool", "claude", "./bin/tool", "/src/app", true, false)
	tests := []struct {
		name    string
		user    []inventory.MCPServer
		project []inventory.MCPServer
		want    []Issue
	}{
		{name: "command found", user: []inventory.MCPServer{found}},
		{name: "turned off", user: []inventory.MCPServer{off}},
		{name: "one issue a command", user: []inventory.MCPServer{gone, goneUvx, goneCursor},
			want: []Issue{
				{Kind: MCPMissing, Severity: Warning, MCP: []inventory.MCPServer{gone, goneCursor}},
				{Kind: MCPMissing, Severity: Warning, MCP: []inventory.MCPServer{goneUvx}},
			}},
		{name: "project", project: []inventory.MCPServer{inProject},
			want: []Issue{{Kind: MCPMissing, Severity: Warning, Project: "/src/app", MCP: []inventory.MCPServer{inProject}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := []projects.Project{{Dir: "/src/app", MCP: tt.project}}
			if got := Find(&inventory.Snapshot{MCP: tt.user}, ps, nil, now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
