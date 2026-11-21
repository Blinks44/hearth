//go:build !windows

package detect

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// A version mise installed is found though nothing puts it on the PATH,
// as mise install leaves it: pnpm in the version's folder, Yarn in its
// bin, its version by the folder.
func TestJSMiseInstalls(t *testing.T) {
	for name, exe := range map[string]string{"pnpm": "pnpm", "yarn": filepath.Join("bin", "yarn")} {
		t.Run(name, func(t *testing.T) {
			root := root(t)
			dir := filepath.Join(root, "home", ".local", "share", "mise", "installs", name, "10.4.1")
			path := fakeExe(t, filepath.Join(dir, exe))
			symlink(t, dir, filepath.Join(filepath.Dir(dir), "latest"))
			r := &run.Fake{}

			got, err := jsManager(t.Context(), r, name, homeEnv(root, "bin"), sysenv.Env{"PATH="})
			if err != nil {
				t.Fatal(err)
			}
			checkInstallations(t, got, []inventory.Installation{{
				Tool: name, Version: "10.4.1", Path: path, RealPath: path,
				Source: inventory.Source{Kind: vm, Manager: "mise", Root: dir, Version: "10.4.1"},
			}})
			if len(got.Managers) != 1 || got.Managers[0].Name != "mise" || !slices.Contains(got.Managers[0].Files, "mise.toml") {
				t.Errorf("managers = %+v, want mise, which picks the version by mise.toml", got.Managers)
			}
			if calls := r.Calls(); len(calls) != 0 {
				t.Errorf("ran %v; the version is in the path", calls)
			}
		})
	}
}

func TestJSPMGlobalPackages(t *testing.T) {
	dir := root(t)
	home := filepath.Join(dir, "home")
	pnpmDir := filepath.Join(dir, "pnpm-home")
	fakeExe(t, filepath.Join(pnpmDir, "pnpm"))
	writeFile(t, filepath.Join(pnpmDir, "global", "5", "package.json"), `{"dependencies":{"@antfu/ni":"^24.0.0","typescript":"^5.8.0"}}`)
	writeFile(t, filepath.Join(pnpmDir, "global", "5", "node_modules", "@antfu", "ni", "package.json"), `{"name":"@antfu/ni","version":"24.1.0","bin":{"ni":"bin/ni.mjs","nr":"bin/nr.mjs"}}`)
	writeFile(t, filepath.Join(pnpmDir, "global", "5", "node_modules", "typescript", "package.json"), `{"name":"typescript","version":"5.8.2","bin":{"tsc":"bin/tsc"}}`)
	writeFile(t, filepath.Join(pnpmDir, "global", "5", "node_modules", "transitive", "package.json"), `{"name":"transitive","version":"1.0.0"}`)

	yarnDir := filepath.Join(home, ".config", "yarn", "global")
	bin := filepath.Join(dir, "bin")
	yarnExe := fakeExe(t, filepath.Join(bin, "yarn"))
	writeFile(t, filepath.Join(yarnDir, "package.json"), `{"dependencies":{"serve":"^14.2.0"}}`)
	writeFile(t, filepath.Join(yarnDir, "node_modules", "serve", "package.json"), `{"name":"serve","version":"14.2.4","bin":"build/main.js"}`)
	writeFile(t, filepath.Join(yarnDir, "node_modules", "arg", "package.json"), `{"name":"arg","version":"5.0.2"}`)

	env := sysenv.Env{"HOME=" + home, "PNPM_HOME=" + pnpmDir, "PATH=" + pnpmDir + ":" + bin}
	r := &run.Fake{}
	r.Stdout("10.4.1\n", filepath.Join(pnpmDir, "pnpm"), "--version")
	r.Stdout("1.22.22\n", yarnExe, "--version")

	pnpmState, err := Pnpm(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pnpmState.Installations) != 1 || len(pnpmState.Installations[0].Packages) != 2 {
		t.Fatalf("pnpm packages = %+v", pnpmState.Installations)
	}
	if p := pnpmState.Installations[0].Packages[0]; p.Name != "@antfu/ni" || p.Version != "24.1.0" || len(p.Commands) != 2 {
		t.Errorf("pnpm pkg[0] = %+v", p)
	}
	if p := pnpmState.Installations[0].Packages[1]; p.Name != "typescript" || p.Version != "5.8.2" {
		t.Errorf("pnpm pkg[1] = %+v", p)
	}

	yarnState, err := Yarn(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(yarnState.Installations) != 1 || len(yarnState.Installations[0].Packages) != 1 {
		t.Fatalf("yarn packages = %+v", yarnState.Installations)
	}
	if p := yarnState.Installations[0].Packages[0]; p.Name != "serve" || p.Version != "14.2.4" {
		t.Errorf("yarn pkg[0] = %+v", p)
	}
}
