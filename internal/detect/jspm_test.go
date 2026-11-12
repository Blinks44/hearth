package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Corepack's .corepack marker denotes a completed download; an interrupted
// cache folder is not an installation. No launcher may run during the scan.
func TestJSCorepackCache(t *testing.T) {
	for _, name := range []string{"pnpm", "yarn"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			launcher := filepath.Join(bin, jsLaunchers(name)[0])
			writeFile(t, launcher, "#!/usr/bin/env node\nrequire('./node_modules/corepack/dist/"+name+".js')")
			if err := os.Chmod(launcher, 0o755); err != nil {
				t.Fatal(err)
			}
			cache := filepath.Join(dir, "cache")
			writeFile(t, filepath.Join(cache, "v1", name, "9.1.0", ".corepack"), `{"hash":"abc"}`)
			writeFile(t, filepath.Join(cache, "v1", name, "8.0.0", ".corepack"), `{}`)
			writeFile(t, filepath.Join(cache, "v1", name, "10.0.0", "package.json"), `{}`)
			writeFile(t, filepath.Join(cache, "lastKnownGood.json"), `{"`+name+`":"9.1.0+sha512.abc"}`)
			env := sysenv.Env{"PATH=" + bin, "PATHEXT=.CMD;.EXE", "COREPACK_HOME=" + cache, "HOME=" + dir, "USERPROFILE=" + dir}
			r := &run.Fake{}
			st, err := jsManager(t.Context(), r, name, env, sysenv.Env{"PATH="})
			if err != nil {
				t.Fatal(err)
			}
			if len(st.Installations) != 3 {
				t.Fatalf("installations: %+v", st.Installations)
			}
			if !st.Installations[0].Source.Shim || st.Installations[0].Version != "" {
				t.Fatalf("shim: %+v", st.Installations[0])
			}
			active := inventory.TerminalCopy(st.Installations)
			if active < 0 || st.Installations[active].Version != "9.1.0" {
				t.Fatalf("active=%d: %+v", active, st.Installations)
			}
			if len(st.Managers) != 1 || st.Managers[0].Name != "corepack" {
				t.Fatalf("managers: %+v", st.Managers)
			}
			if len(r.Calls()) != 0 {
				t.Fatalf("ran launchers: %+v", r.Calls())
			}
		})
	}
}

// npm beside node_modules/corepack, as in the Node folder on Windows,
// is npm itself, not a Corepack shim.
func TestJSNpmManifest(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	launcher := filepath.Join(bin, jsLaunchers("npm")[0])
	writeFile(t, launcher, "#!/usr/bin/env node\n")
	if err := os.Chmod(launcher, 0o755); err != nil {
		t.Fatal(err)
	}
	modules := nodeModules(filepath.Join(bin, nodeExe))
	writeFile(t, filepath.Join(modules, "npm", "package.json"), `{"name":"npm","version":"10.9.0"}`)
	writeFile(t, filepath.Join(modules, "typescript", "package.json"), `{"name":"typescript","version":"5.7.2","bin":{"tsc":"bin/tsc"}}`)
	writeFile(t, filepath.Join(bin, "node_modules", "corepack", "package.json"), `{"name":"corepack","version":"0.30.0"}`)
	r := &run.Fake{}
	st, err := Npm(t.Context(), r, sysenv.Env{"PATH=" + bin, "PATHEXT=.CMD;.EXE", "COREPACK_HOME=" + filepath.Join(dir, "cache"), "HOME=" + dir, "USERPROFILE=" + dir, "N_PREFIX=" + dir}, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 1 || st.Installations[0].Version != "10.9.0" || len(st.Installations[0].Packages) < 2 || st.Installations[0].Source.Shim {
		t.Fatalf("state: %+v", st)
	}
	if len(r.Calls()) != 0 {
		t.Fatalf("ran npm: %+v", r.Calls())
	}
}

func TestCorepackHome(t *testing.T) {
	for _, tc := range []struct {
		env  sysenv.Env
		want string
	}{
		{sysenv.Env{"COREPACK_HOME=/c", "XDG_CACHE_HOME=/x"}, "/c"},
		{sysenv.Env{"XDG_CACHE_HOME=/x", "LOCALAPPDATA=/l"}, filepath.Join("/x", "node", "corepack")},
		{sysenv.Env{"LOCALAPPDATA=/l"}, filepath.Join("/l", "node", "corepack")},
	} {
		if got := corepackHome(tc.env); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.env, got, tc.want)
		}
	}
}

// Since Node 25 Corepack comes with npm install -g: its command and the
// package lie in npm's prefix, as %APPDATA%\npm on Windows.
func TestCorepackFromNpm(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "npm")
	launcher := filepath.Join(prefix, jsLaunchers("corepack")[0])
	writeFile(t, launcher, "#!/bin/sh\nnode \"$basedir/node_modules/corepack/dist/corepack.js\" \"$@\"\n")
	if err := os.Chmod(launcher, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prefix, "node_modules", "corepack", "package.json"), `{"name": "corepack", "version": "0.36.0"}`)
	env := sysenv.Env{"PATH=" + prefix, "PATHEXT=.CMD;.EXE", "HOME=" + dir, "USERPROFILE=" + dir, "N_PREFIX=" + dir}
	st, err := jsManager(t.Context(), &run.Fake{}, "corepack", env, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 1 {
		t.Fatalf("installations: %+v", st.Installations)
	}
	if in := st.Installations[0]; in.Version != "0.36.0" || in.Source.Manager != "npm" || in.Source.Kind != inventory.KindPackageManager {
		t.Fatalf("corepack: %+v", in)
	}
}

// mise's pnpm shim runs the version its global config picks.
func TestJSMiseDefault(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mise := filepath.Join(dir, "mise")
	shim := filepath.Join(mise, "shims", jsLaunchers("pnpm")[0])
	writeFile(t, shim, "shim")
	for _, v := range []string{"10.6.2", "12.18.0"} {
		writeFile(t, filepath.Join(mise, "installs", "pnpm", v, jsLaunchers("pnpm")[0]), "pnpm")
	}
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "config.toml"), "[tools]\npnpm = \"latest\"\n")
	env := sysenv.Env{"PATH=" + filepath.Dir(shim), "PATHEXT=.CMD;.EXE", "MISE_DATA_DIR=" + mise,
		"MISE_GLOBAL_CONFIG_FILE=" + filepath.Join(dir, "config.toml"), "HOME=" + dir, "USERPROFILE=" + dir}
	st, err := jsManager(t.Context(), &run.Fake{}, "pnpm", env, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 || st.Installations[active].Version != "12.18.0" {
		t.Fatalf("active=%d: %+v", active, st.Installations)
	}
}
