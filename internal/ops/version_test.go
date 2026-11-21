package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestVersionPlanCommands(t *testing.T) {
	const v = "22.14.0"
	shell := func(script, name string, args ...string) []string {
		return append([]string{"bash", "-c", script, name}, args...)
	}
	tests := []struct {
		tool, manager string
		action        VersionAction
		version, root string
		asdfSet       bool
		want          []string // nil: Hearth cannot
	}{
		{"node", "fnm", VersionInstall, v, "", false, []string{"fnm", "install", v}},
		{"node", "fnm", VersionUninstall, v, "", false, []string{"fnm", "uninstall", v}},
		{"node", "fnm", VersionDefault, v, "", false, []string{"fnm", "default", v}},
		{"node", "nvm", VersionInstall, v, "", false, shell(nvmScript, "nvm", "install", v)},
		{"node", "nvm", VersionUninstall, v, "", false, shell(nvmScript, "nvm", "uninstall", v)},
		{"node", "nvm", VersionDefault, v, "", false, shell(nvmScript, "nvm", "alias", "default", v)},
		{"node", "nvm-windows", VersionInstall, v, "", false, []string{"nvm", "install", v}},
		{"node", "nvm-windows", VersionUninstall, v, "", false, []string{"nvm", "uninstall", v}},
		{"node", "nvm-windows", VersionDefault, v, "", false, []string{"nvm", "use", v}},
		{"node", "mise", VersionInstall, v, "", false, []string{"mise", "install", "node@" + v}},
		{"node", "mise", VersionUninstall, v, "/home/u/.local/share/mise/installs/node/22.14.0", false, []string{"mise", "uninstall", "node@" + v}},
		{"node", "mise", VersionDefault, v, "", false, []string{"mise", "use", "-g", "node@" + v}},
		{"node", "asdf", VersionInstall, v, "", true, []string{"asdf", "install", "nodejs", v}},
		{"node", "asdf", VersionUninstall, v, "", true, []string{"asdf", "uninstall", "nodejs", v}},
		{"node", "asdf", VersionDefault, v, "", true, []string{"asdf", "set", "-u", "nodejs", v}},
		{"node", "asdf", VersionDefault, v, "", false, []string{"asdf", "global", "nodejs", v}},
		{"node", "nodenv", VersionInstall, v, "", false, []string{"nodenv", "install", v}},
		{"node", "nodenv", VersionUninstall, v, "", false, []string{"nodenv", "uninstall", "-f", v}},
		{"node", "nodenv", VersionDefault, v, "", false, []string{"nodenv", "global", v}},
		{"node", "volta", VersionInstall, v, "", false, []string{"volta", "fetch", "node@" + v}},
		{"node", "volta", VersionUninstall, v, "", false, nil},
		{"node", "volta", VersionDefault, v, "", false, []string{"volta", "install", "node@" + v}},
		{"node", "brew", VersionUninstall, v, "/opt/homebrew/Cellar/node@22/22.14.0", false, []string{"brew", "uninstall", "node@22"}},
		{"node", "brew", VersionInstall, v, "/opt/homebrew/Cellar/node@22/22.14.0", false, nil},
		{"node", "brew", VersionDefault, v, "", false, nil},
		{"node", "n", VersionInstall, v, "", false, []string{"n", "download", v}},
		{"node", "n", VersionUninstall, v, "", false, []string{"n", "rm", v}},
		{"node", "n", VersionDefault, v, "", false, []string{"n", v}},
		{"node", "", VersionInstall, v, "", false, nil},

		{"python", "pyenv", VersionInstall, "3.13.1", "", false, []string{"pyenv", "install", "3.13.1"}},
		{"python", "pyenv", VersionUninstall, "3.12.8", "", false, []string{"pyenv", "uninstall", "-f", "3.12.8"}},
		{"python", "pyenv", VersionDefault, "3.13.1", "", false, []string{"pyenv", "global", "3.13.1"}},
		{"python", "uv", VersionInstall, "3.13.1", "", false, []string{"uv", "python", "install", "3.13.1"}},
		{"python", "uv", VersionUninstall, "3.12.8", "", false, []string{"uv", "python", "uninstall", "3.12.8"}},
		{"python", "uv", VersionDefault, "3.13.1", "", false, nil},
		{"python", "mise", VersionInstall, "3.13.1", "", false, []string{"mise", "install", "python@3.13.1"}},
		{"python", "mise", VersionDefault, "3.13.1", "", false, []string{"mise", "use", "-g", "python@3.13.1"}},
		{"python", "asdf", VersionInstall, "3.13.1", "", true, []string{"asdf", "install", "python", "3.13.1"}},
		{"python", "asdf", VersionDefault, "3.13.1", "", true, nil},
		{"python", "fnm", VersionInstall, "3.13.1", "", false, nil},

		{"ruby", "rbenv", VersionInstall, "3.4.1", "", false, []string{"rbenv", "install", "3.4.1"}},
		{"ruby", "rbenv", VersionUninstall, "3.3.6", "", false, []string{"rbenv", "uninstall", "-f", "3.3.6"}},
		{"ruby", "rbenv", VersionDefault, "3.4.1", "", false, []string{"rbenv", "global", "3.4.1"}},
		{"ruby", "rvm", VersionInstall, "3.4.1", "", false, shell(rvmScript, "rvm", "install", "3.4.1")},
		{"ruby", "rvm", VersionUninstall, "ruby-3.3.6", "", false, shell(rvmScript, "rvm", "remove", "ruby-3.3.6")},
		{"ruby", "rvm", VersionDefault, "ruby-3.4.1", "", false, shell(rvmScript, "rvm", "alias", "create", "default", "ruby-3.4.1")},
		{"ruby", "asdf", VersionDefault, "3.4.1", "", true, []string{"asdf", "set", "-u", "ruby", "3.4.1"}},
		{"ruby", "chruby", VersionInstall, "3.4.1", "", false, nil},

		{"java", "sdkman", VersionInstall, "25", "", false, shell(sdkTemurinScript, "sdk", "install", "java", "25")},
		{"java", "sdkman", VersionUninstall, "17.0.13-tem", "", false, shell(sdkScript, "sdk", "uninstall", "java", "17.0.13-tem")},
		{"java", "sdkman", VersionDefault, "21.0.5-tem", "", false, shell(sdkScript, "sdk", "default", "java", "21.0.5-tem")},
		{"java", "mise", VersionInstall, "25", "", false, []string{"mise", "install", "java@temurin-25"}},
		{"java", "mise", VersionUninstall, "temurin-17.0.13+11.0.LTS", "/m/installs/java/temurin-17.0.13+11.0.LTS", false, []string{"mise", "uninstall", "java@temurin-17.0.13+11.0.LTS"}},
		{"java", "asdf", VersionInstall, "25", "", true, nil},
		{"java", "asdf", VersionUninstall, "temurin-17.0.13+11.0.LTS", "/a/installs/java/temurin-17.0.13+11.0.LTS", true, []string{"asdf", "uninstall", "java", "temurin-17.0.13+11.0.LTS"}},

		{"go", "mise", VersionInstall, "1.23.4", "", false, []string{"mise", "install", "go@1.23.4"}},
		{"go", "mise", VersionDefault, "1.23.4", "", false, []string{"mise", "use", "-g", "go@1.23.4"}},
		{"go", "asdf", VersionInstall, "1.23.4", "", true, []string{"asdf", "install", "golang", "1.23.4"}},
		{"go", "asdf", VersionDefault, "1.23.4", "", true, nil},

		{"php", "mise", VersionInstall, "8.4.2", "", false, nil},
		{"php", "mise", VersionUninstall, "8.3.15", "/m/installs/php/8.3.15", false, []string{"mise", "uninstall", "php@8.3.15"}},
		{"php", "mise", VersionDefault, "8.3.15", "/m/installs/php/8.3.15", false, []string{"mise", "use", "-g", "php@8.3.15"}},
		{"dotnet", "mise", VersionInstall, "9.0.101", "", false, []string{"mise", "install", "dotnet@9.0.101"}},
		{"dotnet", "mise", VersionDefault, "9.0.101", "", false, []string{"mise", "use", "-g", "dotnet@9.0.101"}},
		{"dotnet", "asdf", VersionInstall, "9.0.101", "", true, []string{"asdf", "install", "dotnet-core", "9.0.101"}},
		{"bun", "mise", VersionInstall, "1.1.42", "", false, []string{"mise", "install", "bun@1.1.42"}},
		{"bun", "mise", VersionDefault, "1.1.42", "", false, []string{"mise", "use", "-g", "bun@1.1.42"}},

		{"pnpm", "corepack", VersionInstall, "9.15.0", "", false, []string{"corepack", "install", "-g", "pnpm@9.15.0"}},
		{"pnpm", "corepack", VersionUninstall, "9.15.0", "", false, nil},
		{"yarn", "corepack", VersionDefault, "4.5.3", "", false, []string{"corepack", "install", "-g", "yarn@4.5.3"}},
		{"pnpm", "mise", VersionInstall, "9.15.0", "", false, []string{"mise", "install", "pnpm@9.15.0"}},
		{"node", "corepack", VersionInstall, v, "", false, nil},
		{"yarn", "mise", VersionInstall, "4.5.3", "", false, nil},
	}
	for _, tt := range tests {
		target := VersionTarget{Tool: tt.tool, Title: tt.tool, Manager: tt.manager, Version: tt.version, Root: tt.root, AsdfSet: tt.asdfSet}
		p, ok := VersionPlan(tt.action, target, nil)
		if ok != (tt.want != nil) {
			t.Errorf("%s %s %d: ok = %v, want %v", tt.tool, tt.manager, tt.action, ok, tt.want != nil)
			continue
		}
		if !ok {
			continue
		}
		if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.want) {
			t.Errorf("%s %s %d: steps %v, want %q", tt.tool, tt.manager, tt.action, p.Steps, tt.want)
		}
		if p.Title == "" || p.Why == "" || len(p.Changes) == 0 {
			t.Errorf("%s %s %d: plan says nothing: %+v", tt.tool, tt.manager, tt.action, p)
		}
	}
}

// The check after a plan ran looks at Node found again.
func TestVersionPlanVerify(t *testing.T) {
	const v = "22.14.0"
	copyOf := func(manager string, isDefault bool) inventory.Installation {
		return inventory.Installation{Tool: "node", Version: v, Default: isDefault, Source: inventory.Source{Manager: manager}}
	}
	tests := []struct {
		name   string
		action VersionAction
		found  []inventory.Installation
		ok     bool
	}{
		{"installed", VersionInstall, []inventory.Installation{copyOf("fnm", false)}, true},
		{"not installed", VersionInstall, nil, false},
		{"another manager's copy is not it", VersionInstall, []inventory.Installation{copyOf("brew", false)}, false},
		{"removed", VersionUninstall, nil, true},
		{"still there", VersionUninstall, []inventory.Installation{copyOf("fnm", false)}, false},
		{"default", VersionDefault, []inventory.Installation{copyOf("fnm", true)}, true},
		{"not default", VersionDefault, []inventory.Installation{copyOf("fnm", false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detect := func(context.Context) (inventory.ToolState, error) {
				return inventory.ToolState{Tool: "node", Installations: tt.found}, nil
			}
			p, _ := VersionPlan(tt.action, VersionTarget{Tool: "node", Title: "Node", Manager: "fnm", Version: v}, detect)
			f := &run.Fake{}
			f.Stdout("", p.Steps[0].Argv...)
			got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
			if tt.action == VersionDefault && tt.ok && got.Verified != "fnm starts Node 22.14.0 by default" {
				t.Errorf("Verified = %q", got.Verified)
			}
			if (got.Phase == Succeeded) != tt.ok || got.Phase != Succeeded && got.Phase != NotVerified {
				t.Errorf("phase %d, err %q; want succeeded %v", got.Phase, got.Err, tt.ok)
			}
		})
	}
}

func TestAsdfSet(t *testing.T) {
	for version, want := range map[string]bool{"0.15.0": false, "v0.14.1": false, "0.16.0": true, "0.18.0": true, "": true, "1.0.0": true} {
		if got := AsdfSet(version); got != want {
			t.Errorf("AsdfSet(%q) = %v, want %v", version, got, want)
		}
	}
}

// A Java installed by its line is found by its own name.
func TestVersionPlanVerifyJava(t *testing.T) {
	found := []inventory.Installation{{Tool: "java", Version: "25.0.5", Source: inventory.Source{Manager: "mise", Version: "temurin-25.0.5+8.0.LTS"}}}
	detect := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "java", Installations: found}, nil
	}
	p, _ := VersionPlan(VersionInstall, VersionTarget{Tool: "java", Title: "Java", Manager: "mise", Version: "25"}, detect)
	f := &run.Fake{}
	f.Stdout("", p.Steps[0].Argv...)
	if got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Errorf("phase %d, err %q", got.Phase, got.Err)
	}
}
