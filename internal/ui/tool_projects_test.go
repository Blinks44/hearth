package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// Every language page replaces the old "Coming soon" stub on the Projects tab
// and populates its top Project card from added projects using that language.
func TestLanguagePagesProjectsCardAndTab(t *testing.T) {
	cases := []struct {
		tool     string
		route    string
		snapshot func() *inventory.Snapshot
		project  projects.Project
		wantVer  string
		wantFile string
		wantDef  string
	}{
		{
			tool: "node", route: "/tools/node", snapshot: nodeSnapshot,
			project: projects.Project{
				Dir: home + "/dev/shop-web", Files: []string{".nvmrc"},
				Requirements: []projects.Requirement{{Tool: "node", Want: "22", File: ".nvmrc"}},
			},
			wantVer: "v22.12.0", wantFile: ".nvmrc", wantDef: "v22.12.0",
		},
		{
			tool: "bun", route: "/tools/bun", snapshot: bunSnapshot,
			project: projects.Project{
				Dir: home + "/dev/bun-api", Files: []string{".bun-version"},
				Requirements: []projects.Requirement{{Tool: "bun", Want: "1.2.5", File: ".bun-version"}},
			},
			wantVer: "v1.2.5", wantFile: ".bun-version", wantDef: "v1.2.5",
		},
		{
			tool: "python", route: "/tools/python", snapshot: pythonSnapshot,
			project: projects.Project{
				Dir: home + "/dev/ml-app", Files: []string{".python-version"},
				Requirements: []projects.Requirement{{Tool: "python", Want: "3.13", File: ".python-version"}},
			},
			wantVer: "3.13.1", wantFile: ".python-version", wantDef: "3.13.1",
		},
		{
			tool: "php", route: "/tools/php", snapshot: phpSnapshot,
			project: projects.Project{
				Dir: home + "/dev/laravel-app", Files: []string{".php-version"},
				Requirements: []projects.Requirement{{Tool: "php", Want: "8.4", File: ".php-version"}},
			},
			wantVer: "8.4.3", wantFile: ".php-version", wantDef: "8.4.3",
		},
		{
			tool: "rust", route: "/tools/rust", snapshot: rustSnapshot,
			project: projects.Project{
				Dir: home + "/dev/cli-rs", Files: []string{"rust-toolchain.toml"},
				Requirements: []projects.Requirement{{Tool: "rust", Want: "1.84.0", File: "rust-toolchain.toml"}},
			},
			wantVer: "stable 1.84.0", wantFile: "rust-toolchain.toml", wantDef: "stable 1.84.0",
		},
		{
			tool: "go", route: "/tools/go", snapshot: goSnapshot,
			project: projects.Project{
				Dir: home + "/dev/api-go", Files: []string{"go.mod"},
				Requirements: []projects.Requirement{{Tool: "go", Want: "1.23", File: "go.mod"}},
			},
			wantVer: "1.23.4", wantFile: "go.mod", wantDef: "1.23.4",
		},
		{
			tool: "java", route: "/tools/java", snapshot: javaSnapshot,
			project: projects.Project{
				Dir: home + "/dev/orders-jvm", Files: []string{"pom.xml"},
				Requirements: []projects.Requirement{{Tool: "java", Want: "21", File: "pom.xml"}},
			},
			wantVer: "21.0.5", wantFile: "pom.xml", wantDef: "21.0.5",
		},
		{
			tool: "ruby", route: "/tools/ruby", snapshot: rubySnapshot,
			project: projects.Project{
				Dir: home + "/dev/rails-app", Files: []string{".ruby-version"},
				Requirements: []projects.Requirement{{Tool: "ruby", Want: "3.3.6", File: ".ruby-version"}},
			},
			wantVer: "3.3.6", wantFile: ".ruby-version", wantDef: "3.3.6",
		},
	}

	clickProjectsTab := func(t *testing.T, tt *ui.Tester) {
		t.Helper()
		avail, okA := tt.Find("Available")
		pathTab, okP := tt.Find("PATH")
		if !okA || !okP {
			t.Fatalf("missing Available (%v) or PATH (%v) tab in %q", okA, okP, tt.Texts())
		}
		tt.ClickAt((avail.X+avail.W+pathTab.X)/2, avail.Y+avail.H/2)
		tt.Frame()
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			a := NewApp()
			a.Home = home
			a.Snapshot = tc.snapshot()
			a.router.Push(tc.route)
			tt := ui.NewTester(a.View, 1280, 820)
			tt.Advance(time.Second)

			// Empty projects state on the language page: top card says no projects yet,
			// and the Projects tab shows a real empty state instead of "Coming soon".
			hasTexts(t, tt, "no projects yet")
			clickProjectsTab(t, tt)
			if tt.HasText("Coming soon") {
				t.Fatalf("%s Projects tab still shows Coming soon", tc.tool)
			}
			hasTexts(t, tt, "Other folders → terminal default", tc.wantDef)

			// With an added project using the tool, the top card and Projects tab
			// reflect it, and Details navigates to the project page.
			a.Projects = []projects.Project{tc.project}
			tt.Frame()
			name := tc.project.Dir[len(home+"/dev/"):]
			hasTexts(t, tt, name+" · "+tc.wantFile, tc.wantVer, name, "Other folders → terminal default", "Details")
			if err := tt.Click("Details"); err != nil {
				t.Fatalf("click Details on %s: %v", tc.tool, err)
			}
			tt.Advance(time.Second)
			if want := projectPath(tc.project.Dir); a.router.Location() != want {
				t.Errorf("%s Details navigated to %q, want %q", tc.tool, a.router.Location(), want)
			}
		})
	}
}

// On a language's Projects tab, a project missing its required version surfaces
// both on the top Project card (ahead of ready projects) and with an Install
// button that proposes the right version plan.
func TestLanguageProjectsTabProblemAndQuickInstall(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = nodeSnapshot()
	a.NodeVersions = nodeVersions(time.Now())
	a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: "18.20.5"})
	a.Projects = []projects.Project{
		{
			Dir: home + "/dev/shop-web", Files: []string{".nvmrc"},
			Requirements: []projects.Requirement{{Tool: "node", Want: "22", File: ".nvmrc"}},
		},
		{
			Dir: home + "/work/legacy-admin", Files: []string{".nvmrc"},
			Requirements: []projects.Requirement{{Tool: "node", Want: "18", File: ".nvmrc"}},
		},
	}
	a.router.Push("/tools/node")
	a.nodeTab = 2
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	// Top card surfaces the problem project (legacy-admin) rather than the first ready one.
	hasTexts(t, tt, "legacy-admin · .nvmrc", "Projects using Node", "shop-web", "legacy-admin",
		"All set", "Install", "Other folders → terminal default", "v22.12.0")
	if err := tt.Click("Install Node v18.20.5"); err != nil {
		t.Fatalf("click Install on Projects tab: %v; texts %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "fnm install 18.20.5")
}
