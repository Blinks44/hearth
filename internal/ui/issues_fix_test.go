package ui

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

// Each issue an operation fixes offers its plan on the overview, and
// once the plan did its part the scan finds the issue no more.
func TestIssueFixes(t *testing.T) {
	tests := []struct {
		name  string
		kind  issues.Kind
		setup func(a *App)
		plan  func(p ops.Plan) bool
		fixed func(a *App) // the system as the plan leaves it
	}{
		{
			name: "missing PATH folder", kind: issues.PathMissing,
			setup: func(a *App) {
				a.Snapshot = &inventory.Snapshot{TerminalPath: []inventory.PathEntry{
					{Dir: "/usr/bin"},
					{Dir: home + "/.nvm/versions/node/v16.20.2/bin", Missing: true, Sources: []inventory.PathSource{{File: home + "/.zshrc", Line: 12}}},
				}}
				a.PlanPath = func(_ []inventory.PathEntry, dirs []string, done func(ops.Plan, error)) {
					done(ops.Plan{Title: "Fix PATH: " + strings.Join(dirs, ", ")}, nil)
				}
			},
			plan:  func(p ops.Plan) bool { return p.Title == "Fix PATH: "+home+"/.nvm/versions/node/v16.20.2/bin" },
			fixed: func(a *App) { a.Snapshot.TerminalPath = a.Snapshot.TerminalPath[:1] },
		},
		{
			name: "project needs a missing Node", kind: issues.ProjectMissing,
			setup: func(a *App) {
				a.Snapshot = nodeSnapshot()
				a.Projects = testProjects()
				a.NodeVersions = nodeVersions(time.Now())
				a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: "18.20.5"}, versions.Release{Version: "18.20.4"})
			},
			plan: func(p ops.Plan) bool {
				return len(p.Steps) > 0 && slices.Equal(p.Steps[0].Argv[len(p.Steps[0].Argv)-2:], []string{"install", "18.20.5"})
			},
			fixed: func(a *App) {
				st := &a.Snapshot.Tools[0]
				st.Installations = append(st.Installations, inventory.Installation{Tool: "node", Version: "18.20.5", Path: home + "/.local/share/fnm/node-versions/v18.20.5/installation/bin/node",
					Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}})
			},
		},
		{
			name: "Docker engine not running", kind: issues.DockerUnavailable,
			setup: func(a *App) {
				a.Snapshot = dockerSnapshot()
				d := a.Snapshot.Tools[0].Docker
				d.Selected = "podman"
				d.Contexts = append(d.Contexts, inventory.DockerContext{Name: "podman", Engine: "Podman", Status: "unavailable", Local: true})
				d.Engines = append(d.Engines, inventory.DockerEngine{Name: "Podman", Path: "/opt/homebrew/bin/podman"})
			},
			plan:  func(p ops.Plan) bool { return p.Title == "Start Podman" },
			fixed: func(a *App) { a.Snapshot.Tools[0].Docker.Contexts[3].Status = "running" },
		},
		{
			name: "database port taken", kind: issues.DatabasePortConflict,
			setup: func(a *App) {
				a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
					{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17"},
				}, Listeners: []inventory.DatabaseListener{{Address: "*", Port: "5432", Process: "postgres", PID: "812"}}}}}}
				a.Projects = []projects.Project{{Dir: home + "/dev/shop-web", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}}}
			},
			plan: func(p ops.Plan) bool {
				return slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "stop", "postgresql@17"})
			},
			fixed: func(a *App) {
				d := a.Snapshot.Tools[0].Databases
				d.Servers[0].Status, d.Listeners = "stopped", nil
			},
		},
		{
			name: "skill copied to several folders", kind: issues.SkillCopies,
			setup: func(a *App) {
				a.Snapshot = aiSnapshot()
				a.SkillBackups = home + "/backups"
				a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
					done(ops.Plan{Title: "Merge skills"}, nil)
				}
			},
			plan: func(p ops.Plan) bool { return p.Title == "Merge skills" },
			fixed: func(a *App) {
				// One folder stays of each skill's copies.
				var kept []inventory.Skill
				for _, sk := range a.Snapshot.Skills {
					if !slices.ContainsFunc(kept, func(o inventory.Skill) bool { return o.Name == sk.Name && o.Project == sk.Project }) {
						kept = append(kept, sk)
					}
				}
				a.Snapshot.Skills = kept
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind == issues.PathMissing && runtime.GOOS == "windows" {
				t.Skip("Unix paths and startup files")
			}
			a := NewApp()
			a.Home = home
			tc.setup(a)
			now := time.Now()
			found := issues.Find(a.Snapshot, a.Projects, a.NodeVersions, now)
			i := slices.IndexFunc(found, func(is issues.Issue) bool { return is.Kind == tc.kind })
			if i < 0 {
				t.Fatalf("no %s issue in %+v", tc.kind, found)
			}
			fix := a.issueFix(found[i], found)
			if fix == nil {
				t.Fatal("no fix")
			}
			fix()
			if a.op == nil || !tc.plan(a.op.plan) {
				t.Fatalf("plan %+v", a.op)
			}
			tc.fixed(a)
			if slices.ContainsFunc(issues.Find(a.Snapshot, a.Projects, a.NodeVersions, now), func(is issues.Issue) bool { return is.Kind == tc.kind }) {
				t.Error("the issue is still there")
			}
		})
	}
}

// The overview's issue has Fix… beside Details, and it opens the plan;
// a warning without an operation has only Details.
func TestOverviewFix(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "databases", Databases: &inventory.DatabaseState{Servers: []inventory.Database{
		{Kind: "PostgreSQL", Name: "PostgreSQL", Version: "17.2", Path: "/opt/homebrew/bin/postgres", Source: "brew", Status: "running", Service: "postgresql@17"},
	}, Listeners: []inventory.DatabaseListener{{Address: "*", Port: "5432", Process: "postgres", PID: "812"}}}}}}
	a.Projects = []projects.Project{{Dir: home + "/dev/shop-web", Databases: []projects.DatabaseReference{{Kind: "PostgreSQL", Service: "db", File: "compose.yaml", Ports: []string{"5432"}}}}}
	tt := openOverview(t, a)
	screenshot(t, tt, "overview-fix")
	if err := tt.Click("Fix…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || !a.op.open {
		t.Fatalf("plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	a.Snapshot = calmSnapshot()
	a.Projects = []projects.Project{testProjects()[1]}
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Details") || tt.HasText("Fix…") {
		t.Errorf("an issue without an operation: %q", tt.Texts())
	}
}
