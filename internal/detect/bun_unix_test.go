//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fakeExe puts an empty executable at path.
func fakeExe(t *testing.T, path string) string {
	t.Helper()
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBunScriptAndNpm(t *testing.T) {
	root := root(t)
	home := filepath.Join(root, "home")
	install := filepath.Join(home, ".bun")
	script := fakeExe(t, filepath.Join(install, "bin", "bun"))
	writeFile(t, filepath.Join(home, ".zshrc"), "export BUN_INSTALL=\"$HOME/.bun\"\nexport PATH=\"$BUN_INSTALL/bin:$PATH\"\n")
	global := filepath.Join(install, "install", "global")
	writeFile(t, filepath.Join(global, "package.json"), `{"dependencies":{"vercel":"^41.4.1","@biomejs/biome":"1.9.4"}}`)
	writeFile(t, filepath.Join(global, "node_modules", "vercel", "package.json"), `{"name":"vercel","version":"41.4.1","bin":{"vercel":"dist/index.js"}}`)
	// A dependency of vercel's, not a global package itself.
	writeFile(t, filepath.Join(global, "node_modules", "chalk", "package.json"), `{"name":"chalk","version":"5.4.1"}`)

	// npm i -g bun under fnm's Node 22: the command links into the package.
	prefix := filepath.Join(home, ".local", "share", "fnm", "node-versions", "v22.12.0", "installation")
	pkg := filepath.Join(prefix, "lib", "node_modules", "bun")
	writeFile(t, filepath.Join(pkg, "package.json"), `{"name":"bun","version":"1.1.30"}`)
	npmBun := fakeExe(t, filepath.Join(pkg, "bin", "bun.exe"))
	symlink(t, npmBun, filepath.Join(prefix, "bin", "bun"))

	term := append(homeEnv(root), "PATH="+filepath.Join(install, "bin")+string(os.PathListSeparator)+filepath.Join(prefix, "bin"))
	r := &run.Fake{}
	r.Stdout("1.2.5\n", script, "--version")

	globals := []inventory.Package{
		{Name: "@biomejs/biome", Version: "1.9.4", Manager: "bun"},
		{Name: "vercel", Version: "41.4.1", Manager: "bun", Commands: []string{"vercel"}},
	}
	got, err := Bun(t.Context(), r, term, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	checkInstallations(t, got, []inventory.Installation{
		{
			Tool: "bun", Version: "1.2.5",
			Path: script, RealPath: script,
			Source:   inventory.Source{Kind: inventory.KindInstaller, Manager: inventory.ScriptInstall, Root: install},
			OnPath:   1,
			Packages: globals,
			PathFile: filepath.Join(home, ".zshrc"),
		},
		{
			Tool: "bun", Version: "1.1.30",
			Path: filepath.Join(prefix, "bin", "bun"), RealPath: npmBun,
			Source: inventory.Source{Kind: pm, Manager: "npm", Root: pkg},
			OnPath: 2,
			// bun add -g installs into ~/.bun whichever copy runs it.
			Packages: globals,
		},
	})
	if calls := r.Calls(); len(calls) != 1 {
		t.Errorf("ran %v; only the script's copy needs asking", calls)
	}
}

func TestBunMiseAndBrew(t *testing.T) {
	root := root(t)
	mise := filepath.Join(root, "home", ".local", "share", "mise", "installs", "bun", "1.2.4")
	miseBun := fakeExe(t, filepath.Join(mise, "bin", "bun"))
	brew := filepath.Join(root, "brew")
	keg := filepath.Join(brew, "Cellar", "bun", "1.2.5")
	brewBun := fakeExe(t, filepath.Join(keg, "bin", "bun"))
	symlink(t, brewBun, filepath.Join(brew, "bin", "bun"))

	term := append(homeEnv(root, brew, "bin"), "HOMEBREW_PREFIX="+brew)
	gui := sysenv.Env{"PATH=" + filepath.Join(brew, "bin")}
	r := &run.Fake{}

	got, err := Bun(t.Context(), r, term, gui)
	if err != nil {
		t.Fatal(err)
	}
	checkInstallations(t, got, []inventory.Installation{
		{
			Tool: "bun", Version: "1.2.5",
			Path: filepath.Join(brew, "bin", "bun"), RealPath: brewBun,
			Source: inventory.Source{Kind: pm, Manager: "brew", Root: keg, Version: "1.2.5"},
			OnPath: 1, InGUI: 1,
		},
		{
			Tool: "bun", Version: "1.2.4",
			Path: miseBun, RealPath: miseBun,
			Source: inventory.Source{Kind: vm, Manager: "mise", Root: mise, Version: "1.2.4"},
		},
	})
	if calls := r.Calls(); len(calls) != 0 {
		t.Errorf("ran %v; versions are in the paths", calls)
	}
}
