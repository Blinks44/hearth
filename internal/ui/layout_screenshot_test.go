package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
)

// The pages share one layout check: Russian text, wide and narrow windows.
func TestLayoutScreenshots(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	defer i18n.SetLocaleFunc(nil)
	many := &inventory.Snapshot{}
	for _, s := range []*inventory.Snapshot{nodeSnapshot(), bunSnapshot(), rubySnapshot(), javaSnapshot(), goSnapshot(), dotnetSnapshot(), jsSnapshot(), dockerSnapshot(), aiSnapshot()} {
		many.Tools = append(many.Tools, s.Tools...)
		many.Agents = append(many.Agents, s.Agents...)
	}
	for _, tc := range []struct {
		name string
		app  func() *App
	}{
		{"catalog", func() *App {
			a := catalogApp()
			a.skillCatalog.skills[0].Description = strings.Repeat("Длинное описание скилла, которое не помещается в четыре строки. ", 12)
			a.router.Push(skillsPage)
			a.skillCatalogOpen = true
			return a
		}},
		{"overview-projects", func() *App {
			a := NewApp()
			a.Snapshot = many
			a.Projects = testProjects()
			return a
		}},
		{"overview-empty", func() *App {
			a := NewApp()
			a.Snapshot = many
			a.ChooseFolder, a.FindProjects = func() {}, func() {}
			return a
		}},
		{"codex-linked", func() *App {
			a := NewApp()
			a.Snapshot = aiSnapshot()
			for i := range a.Snapshot.Agents {
				if ag := &a.Snapshot.Agents[i]; ag.ID == "codex" {
					shared := home + "/.agents/AGENTS.md"
					ag.Instructions = &inventory.GlobalInstructions{State: "linked", Active: home + "/.codex/AGENTS.md",
						Shared: inventory.InstructionFile{Path: shared, Exists: true},
						Native: inventory.InstructionFile{Path: home + "/.codex/AGENTS.md", Target: shared, Exists: true, Link: true}}
				}
			}
			a.Shell = &fakeShell{}
			a.router.Push(agentPath("codex"))
			return a
		}},
		{"instructions-empty", func() *App {
			a := NewApp()
			a.Snapshot = aiSnapshot()
			a.ChooseFolder = func() {}
			a.router.Push(instructionsPage)
			return a
		}},
		{"projects-list", func() *App {
			a := NewApp()
			a.Snapshot = many
			a.Projects = testProjects()
			a.ChooseFolder, a.FindProjects = func() {}, func() {}
			a.router.Push("/projects")
			return a
		}},
		{"project-detail-db", func() *App {
			a := NewApp()
			snap := *many
			snap.Tools = append(append([]inventory.ToolState(nil), many.Tools...), inventory.ToolState{
				Tool: "databases",
				Databases: &inventory.DatabaseState{
					Servers: []inventory.Database{
						{Kind: "PostgreSQL", Name: "postgresql@16", Status: "running", Ports: []string{"5432"}},
						{Kind: "Redis", Name: "redis", Status: "stopped", Ports: []string{"6379"}},
					},
					Listeners: []inventory.DatabaseListener{
						{Port: "5432", Process: "postgres", Path: "/opt/homebrew/bin/postgres"},
					},
				},
			})
			a.Snapshot = &snap
			a.NodeVersions = nodeVersions(time.Now())
			a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: "18.20.5"})
			projs := testProjects()
			projs[1].Databases = []projects.DatabaseReference{
				{Kind: "PostgreSQL", Service: "postgres", File: "compose.yaml", Ports: []string{"5432"}},
				{Kind: "Redis", Service: "cache", File: ".env", Ports: []string{"6379"}},
			}
			a.Projects = projs
			a.router.Push(projectPath(projs[1].Dir))
			return a
		}},
		{"node-projects-tab", func() *App {
			a := NewApp()
			a.Snapshot = many
			a.NodeVersions = nodeVersions(time.Now())
			a.NodeVersions.Releases = append(a.NodeVersions.Releases, versions.Release{Version: "18.20.5"})
			a.Projects = testProjects()
			a.router.Push("/tools/node")
			a.nodeTab = 2
			return a
		}},
	} {
		for _, w := range []int{1280, 1000} {
			a := tc.app()
			a.Home = home
			h := 940
			if strings.HasPrefix(tc.name, "overview") {
				h = 1500
			}
			tt := ui.NewTester(a.View, w, h)
			tt.Advance(time.Second)
			name := tc.name
			if w == 1000 {
				name += "-narrow"
			}
			screenshot(t, tt, name)
		}
	}
}
