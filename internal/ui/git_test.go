package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// The page shows brew's Git with its update and the system's one the OS
// needs, the global settings, the helpers and GitHub CLI without its
// token, and SSH keys by name.
func TestGitPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	brew := inventory.Installation{Tool: "git", Version: "2.47.1", Path: "/opt/homebrew/bin/git", OnPath: 1, InGUI: 2,
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/git/2.47.1"}}
	system := inventory.Installation{Tool: "git", Version: "2.39.5", Path: "/usr/bin/git", OnPath: 2, InGUI: 1,
		Source: inventory.Source{Kind: inventory.KindSystem, Root: "/usr"}}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "git", Installations: []inventory.Installation{brew, system}, Git: &inventory.GitState{
			ConfigFile: home + "/.gitconfig",
			Config: []inventory.Setting{{Name: "user.name", Value: "Dev User"}, {Name: "user.email", Value: "dev@example.com"},
				{Name: "init.defaultbranch", Value: "main"}, {Name: "core.editor", Value: "code --wait"}},
			Helpers: []string{"osxkeychain"},
			GitHub:  &inventory.GitHubCLI{Version: "2.63.2", Hosts: []inventory.GitHubHost{{Host: "github.com", User: "devuser"}}},
			Keys: []inventory.SSHKey{{Name: "id_ed25519", Type: "ED25519", Hosts: []string{"github.com"}, InAgent: true},
				{Name: "id_rsa", Type: "RSA", Bits: 2048}},
		}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew",
			Packages: []inventory.SystemPackage{{Name: "git", Version: "2.47.1", Available: "2.48.1", Path: "/opt/homebrew/Cellar/git/2.47.1"}}}}},
	}}
	a.router.Push(gitPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "git")
	hasTexts(t, tt, "Tools · version control", "2 copies", whichCommand+" git → brew", "Dev User", "~/.gitconfig",
		"2.47.1", "available 2.48.1", "2.39.5", "Global settings", "init.defaultBranch", "main", "code --wait",
		"macOS keychain", "osxkeychain · passwords and tokens are kept there",
		"GitHub CLI 2.63.2", "signed in to github.com · devuser · the token is not shown",
		"SSH keys", "id_ed25519", "for github.com · in ssh-agent", "ED25519", "id_rsa", "not in the SSH config or the agent", "RSA 2048",
		"The system's Git is the OS's own (on macOS from the Command Line Tools): Hearth does not remove or update it.")
	// The sidebar shows the version the terminal runs.
	if !tt.HasText("2.47") {
		t.Errorf("no version in the sidebar: %q", tt.Texts())
	}
}
