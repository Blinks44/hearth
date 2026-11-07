package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestNodePlanCommands(t *testing.T) {
	const v = "22.14.0"
	nvm := []string{"bash", "-c", nvmScript, "nvm"}
	tests := []struct {
		manager string
		action  NodeAction
		root    string
		asdfSet bool
		want    []string // nil: Hearth cannot
	}{
		{"fnm", NodeInstall, "", false, []string{"fnm", "install", v}},
		{"fnm", NodeUninstall, "", false, []string{"fnm", "uninstall", v}},
		{"fnm", NodeDefault, "", false, []string{"fnm", "default", v}},
		{"nvm", NodeInstall, "", false, append(slices.Clone(nvm), "install", v)},
		{"nvm", NodeUninstall, "", false, append(slices.Clone(nvm), "uninstall", v)},
		{"nvm", NodeDefault, "", false, append(slices.Clone(nvm), "alias", "default", v)},
		{"nvm-windows", NodeInstall, "", false, []string{"nvm", "install", v}},
		{"nvm-windows", NodeUninstall, "", false, []string{"nvm", "uninstall", v}},
		{"nvm-windows", NodeDefault, "", false, []string{"nvm", "use", v}},
		{"mise", NodeInstall, "", false, []string{"mise", "install", "node@" + v}},
		{"mise", NodeUninstall, "", false, []string{"mise", "uninstall", "node@" + v}},
		{"mise", NodeDefault, "", false, []string{"mise", "use", "-g", "node@" + v}},
		{"asdf", NodeInstall, "", true, []string{"asdf", "install", "nodejs", v}},
		{"asdf", NodeUninstall, "", true, []string{"asdf", "uninstall", "nodejs", v}},
		{"asdf", NodeDefault, "", true, []string{"asdf", "set", "-u", "nodejs", v}},
		{"asdf", NodeDefault, "", false, []string{"asdf", "global", "nodejs", v}},
		{"nodenv", NodeInstall, "", false, []string{"nodenv", "install", v}},
		{"nodenv", NodeUninstall, "", false, []string{"nodenv", "uninstall", "-f", v}},
		{"nodenv", NodeDefault, "", false, []string{"nodenv", "global", v}},
		{"volta", NodeInstall, "", false, []string{"volta", "fetch", "node@" + v}},
		{"volta", NodeUninstall, "", false, nil},
		{"volta", NodeDefault, "", false, []string{"volta", "install", "node@" + v}},
		{"brew", NodeUninstall, "/opt/homebrew/Cellar/node@22/22.14.0", false, []string{"brew", "uninstall", "node@22"}},
		{"brew", NodeInstall, "/opt/homebrew/Cellar/node@22/22.14.0", false, nil},
		{"brew", NodeDefault, "", false, nil},
		{"n", NodeInstall, "", false, []string{"n", "download", v}},
		{"n", NodeUninstall, "", false, []string{"n", "rm", v}},
		{"n", NodeDefault, "", false, []string{"n", v}},
		{"", NodeInstall, "", false, nil},
	}
	for _, tt := range tests {
		p, ok := NodePlan(tt.action, NodeTarget{Manager: tt.manager, Version: v, Root: tt.root, AsdfSet: tt.asdfSet}, nil)
		if ok != (tt.want != nil) {
			t.Errorf("%s %d: ok = %v, want %v", tt.manager, tt.action, ok, tt.want != nil)
			continue
		}
		if !ok {
			continue
		}
		if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.want) {
			t.Errorf("%s %d: steps %v, want %q", tt.manager, tt.action, p.Steps, tt.want)
		}
		if p.Title == "" || p.Why == "" || len(p.Changes) == 0 {
			t.Errorf("%s %d: plan says nothing: %+v", tt.manager, tt.action, p)
		}
	}
}

// The check after a plan ran looks at Node found again.
func TestNodePlanVerify(t *testing.T) {
	const v = "22.14.0"
	copyOf := func(manager string, isDefault bool) inventory.Installation {
		return inventory.Installation{Tool: "node", Version: v, Default: isDefault, Source: inventory.Source{Manager: manager}}
	}
	tests := []struct {
		name   string
		action NodeAction
		found  []inventory.Installation
		ok     bool
	}{
		{"installed", NodeInstall, []inventory.Installation{copyOf("fnm", false)}, true},
		{"not installed", NodeInstall, nil, false},
		{"another manager's copy is not it", NodeInstall, []inventory.Installation{copyOf("brew", false)}, false},
		{"removed", NodeUninstall, nil, true},
		{"still there", NodeUninstall, []inventory.Installation{copyOf("fnm", false)}, false},
		{"default", NodeDefault, []inventory.Installation{copyOf("fnm", true)}, true},
		{"not default", NodeDefault, []inventory.Installation{copyOf("fnm", false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detect := func(context.Context) (inventory.ToolState, error) {
				return inventory.ToolState{Tool: "node", Installations: tt.found}, nil
			}
			p, _ := NodePlan(tt.action, NodeTarget{Manager: "fnm", Version: v}, detect)
			f := &run.Fake{}
			f.Stdout("", p.Steps[0].Argv...)
			got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
			if tt.action == NodeDefault && tt.ok && got.Verified != "fnm starts v22.14.0 by default" {
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
