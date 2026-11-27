package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestGlobalPackagePlanCommands(t *testing.T) {
	tests := []struct {
		name   string
		action GlobalPkgAction
		pkg    inventory.Package
		want   []string
	}{
		{"npm update", GlobalPkgUpdate, inventory.Package{Name: "@angular/cli", Manager: "npm"}, []string{"npm", "install", "-g", "@angular/cli@latest"}},
		{"npm uninstall", GlobalPkgUninstall, inventory.Package{Name: "typescript", Manager: "npm"}, []string{"npm", "uninstall", "-g", "typescript"}},
		{"pnpm update", GlobalPkgUpdate, inventory.Package{Name: "tsx", Manager: "pnpm"}, []string{"pnpm", "add", "-g", "tsx@latest"}},
		{"pnpm uninstall", GlobalPkgUninstall, inventory.Package{Name: "tsx", Manager: "pnpm"}, []string{"pnpm", "remove", "-g", "tsx"}},
		{"yarn update", GlobalPkgUpdate, inventory.Package{Name: "eslint", Manager: "yarn"}, []string{"yarn", "global", "upgrade", "eslint"}},
		{"yarn uninstall", GlobalPkgUninstall, inventory.Package{Name: "eslint", Manager: "yarn"}, []string{"yarn", "global", "remove", "eslint"}},
		{"bun update", GlobalPkgUpdate, inventory.Package{Name: "cowsay", Manager: "bun"}, []string{"bun", "add", "-g", "cowsay@latest"}},
		{"bun uninstall", GlobalPkgUninstall, inventory.Package{Name: "cowsay", Manager: "bun"}, []string{"bun", "remove", "-g", "cowsay"}},
		{"uv update", GlobalPkgUpdate, inventory.Package{Name: "ruff", Manager: "uv"}, []string{"uv", "tool", "upgrade", "ruff"}},
		{"uv uninstall", GlobalPkgUninstall, inventory.Package{Name: "ruff", Manager: "uv"}, []string{"uv", "tool", "uninstall", "ruff"}},
		{"pipx update", GlobalPkgUpdate, inventory.Package{Name: "black", Manager: "pipx"}, []string{"pipx", "upgrade", "black"}},
		{"pipx uninstall", GlobalPkgUninstall, inventory.Package{Name: "black", Manager: "pipx"}, []string{"pipx", "uninstall", "black"}},
		{"cargo update", GlobalPkgUpdate, inventory.Package{Name: "ripgrep", Manager: "cargo"}, []string{"cargo", "install", "ripgrep"}},
		{"cargo uninstall", GlobalPkgUninstall, inventory.Package{Name: "ripgrep", Manager: "cargo"}, []string{"cargo", "uninstall", "ripgrep"}},
		{"gem update", GlobalPkgUpdate, inventory.Package{Name: "bundler", Manager: "gem"}, []string{"gem", "update", "bundler"}},
		{"gem uninstall", GlobalPkgUninstall, inventory.Package{Name: "bundler", Manager: "gem"}, []string{"gem", "uninstall", "-a", "-x", "bundler"}},
		{"dotnet update", GlobalPkgUpdate, inventory.Package{Name: "dotnet-ef", Manager: "dotnet"}, []string{"dotnet", "tool", "update", "-g", "dotnet-ef"}},
		{"dotnet uninstall", GlobalPkgUninstall, inventory.Package{Name: "dotnet-ef", Manager: "dotnet"}, []string{"dotnet", "tool", "uninstall", "-g", "dotnet-ef"}},
		{"composer update", GlobalPkgUpdate, inventory.Package{Name: "laravel/installer", Manager: "composer"}, []string{"composer", "global", "require", "laravel/installer"}},
		{"composer uninstall", GlobalPkgUninstall, inventory.Package{Name: "laravel/installer", Manager: "composer"}, []string{"composer", "global", "remove", "laravel/installer"}},
		{"go update", GlobalPkgUpdate, inventory.Package{Name: "gopls", Manager: "go", Module: "golang.org/x/tools/gopls"}, []string{"go", "install", "golang.org/x/tools/gopls@latest"}},
		{"go update without module", GlobalPkgUpdate, inventory.Package{Name: "gopls", Manager: "go"}, nil},
		{"go update module with @", GlobalPkgUpdate, inventory.Package{Name: "gopls", Manager: "go", Module: "golang.org/x/tools/gopls@v1"}, nil},
		{"go uninstall unsupported", GlobalPkgUninstall, inventory.Package{Name: "gopls", Manager: "go", Module: "golang.org/x/tools/gopls"}, nil},
		{"unmanaged", GlobalPkgUpdate, inventory.Package{Name: "buildx"}, nil},
		{"invalid name flag", GlobalPkgUninstall, inventory.Package{Name: "--all", Manager: "npm"}, nil},
		{"invalid name bare @", GlobalPkgUpdate, inventory.Package{Name: "@", Manager: "npm"}, nil},
		{"invalid name embedded @", GlobalPkgUpdate, inventory.Package{Name: "foo@1.0", Manager: "npm"}, nil},
		{"invalid name shell", GlobalPkgUpdate, inventory.Package{Name: "foo; id", Manager: "npm"}, nil},
		{"invalid name traversal", GlobalPkgUpdate, inventory.Package{Name: "../foo", Manager: "npm"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := GlobalPackagePlan(tt.action, tt.pkg, nil)
			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, want %v", ok, tt.want != nil)
			}
			if !ok {
				return
			}
			if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.want) {
				t.Errorf("argv = %q, want %q", p.Steps[0].Argv, tt.want)
			}
		})
	}
}

func TestGlobalPackagePlanVerify(t *testing.T) {
	pkg := inventory.Package{Name: "ruff", Manager: "uv"}
	detectEmpty := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "python"}, nil
	}
	p, ok := GlobalPackagePlan(GlobalPkgUninstall, pkg, detectEmpty)
	if !ok {
		t.Fatal("expected uninstall plan")
	}
	f := &run.Fake{}
	f.Stdout("", p.Steps[0].Argv...)
	if got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Errorf("phase = %v, err = %q", got.Phase, got.Err)
	}

	detectStillThere := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{
			Tool:          "python",
			Installations: []inventory.Installation{{Packages: []inventory.Package{pkg}}},
		}, nil
	}
	p2, _ := GlobalPackagePlan(GlobalPkgUninstall, pkg, detectStillThere)
	if got := Execute(t.Context(), f, p2, &strings.Builder{}, func(State) {}); got.Phase != NotVerified {
		t.Errorf("still present: phase = %v, want NotVerified", got.Phase)
	}
}
