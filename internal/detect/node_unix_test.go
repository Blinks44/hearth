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

const (
	vm = inventory.KindVersionManager
	pm = inventory.KindPackageManager
)

// homeEnv is an environment whose home and N_PREFIX lie in root, so the
// detector sees no manager of the machine it runs on. Its PATH is the one
// directory pathDir joins.
func homeEnv(root string, pathDir ...string) sysenv.Env {
	return sysenv.Env{
		"HOME=" + filepath.Join(root, "home"),
		"N_PREFIX=" + filepath.Join(root, "n"),
		"PATH=" + filepath.Join(pathDir...),
	}
}

func TestNodeFnmAndBrew(t *testing.T) {
	root := root(t)
	fnm := filepath.Join(root, "home", ".local", "share", "fnm")
	v22 := filepath.Join(fnm, "node-versions", "v22.11.0")
	v20 := filepath.Join(fnm, "node-versions", "v20.18.0")
	fnm22 := fakeNode(t, filepath.Join(v22, "installation"))
	fnm20 := fakeNode(t, filepath.Join(v20, "installation"))
	writeFile(t, filepath.Join(v22, "installation", "lib", "node_modules", "npm", "package.json"), `{"name":"npm","version":"10.9.0"}`)
	symlink(t, filepath.Join(v22, "installation"), filepath.Join(fnm, "aliases", "default"))
	shell := filepath.Join(root, "home", ".local", "state", "fnm_multishells", "123_456")
	symlink(t, filepath.Join(v22, "installation"), shell)

	brew := filepath.Join(root, "brew")
	keg := filepath.Join(brew, "Cellar", "node", "23.3.0_1")
	brewNode := fakeNode(t, keg)
	symlink(t, brewNode, filepath.Join(brew, "bin", "node"))
	writeFile(t, filepath.Join(brew, "lib", "node_modules", "npm", "package.json"), `{"name":"npm","version":"10.9.1"}`)
	writeFile(t, filepath.Join(brew, "lib", "node_modules", "@angular", "cli", "package.json"), `{"name":"@angular/cli","version":"19.0.0"}`)

	term := append(homeEnv(root), "HOMEBREW_PREFIX="+brew,
		"PATH="+filepath.Join(shell, "bin")+string(os.PathListSeparator)+filepath.Join(brew, "bin"))
	gui := sysenv.Env{"PATH=" + filepath.Join(brew, "bin")}
	r := &run.Fake{}

	got, err := Node(t.Context(), r, term, gui)
	if err != nil {
		t.Fatal(err)
	}
	checkInstallations(t, got, []inventory.Installation{
		{
			Tool: "node", Version: "22.11.0",
			Path: filepath.Join(shell, "bin", "node"), RealPath: fnm22,
			Source:   inventory.Source{Kind: vm, Manager: "fnm", Root: v22, Version: "22.11.0"},
			OnPath:   1,
			Default:  true,
			Packages: []inventory.Package{{Name: "npm", Version: "10.9.0"}},
		},
		{
			Tool: "node", Version: "23.3.0",
			Path: filepath.Join(brew, "bin", "node"), RealPath: brewNode,
			Source: inventory.Source{Kind: pm, Manager: "brew", Root: keg, Version: "23.3.0_1"},
			OnPath: 2, InGUI: 1,
			// brew's npm installs globals into the prefix, not the keg.
			Packages: []inventory.Package{{Name: "@angular/cli", Version: "19.0.0"}, {Name: "npm", Version: "10.9.1"}},
		},
		{
			Tool: "node", Version: "20.18.0",
			Path: fnm20, RealPath: fnm20,
			Source: inventory.Source{Kind: vm, Manager: "fnm", Root: v20, Version: "20.18.0"},
		},
	})
	if calls := r.Calls(); len(calls) != 0 {
		t.Errorf("ran %v; versions are in the paths", calls)
	}
}

func TestNodeNvm(t *testing.T) {
	root := root(t)
	nvm := filepath.Join(root, "home", ".nvm")
	var nodes []string
	for _, v := range []string{"v20.18.0", "v22.11.0", "v22.12.0"} {
		nodes = append(nodes, fakeNode(t, filepath.Join(nvm, "versions", "node", v)))
	}
	// default -> lts/* -> lts/jod -> v22.11.0, as nvm writes them.
	writeFile(t, filepath.Join(nvm, "alias", "default"), "lts/*\n")
	writeFile(t, filepath.Join(nvm, "alias", "lts", "*"), "lts/jod\n")
	writeFile(t, filepath.Join(nvm, "alias", "lts", "jod"), "v22.11.0\n")
	term := homeEnv(root, filepath.Dir(nodes[1]))

	got, err := Node(t.Context(), &run.Fake{}, term, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	src := func(v string) inventory.Source {
		return inventory.Source{Kind: vm, Manager: "nvm", Root: filepath.Join(nvm, "versions", "node", "v"+v), Version: v}
	}
	checkInstallations(t, got, []inventory.Installation{
		{Tool: "node", Version: "22.11.0", Path: nodes[1], RealPath: nodes[1], Source: src("22.11.0"), OnPath: 1, Default: true},
		{Tool: "node", Version: "20.18.0", Path: nodes[0], RealPath: nodes[0], Source: src("20.18.0")},
		{Tool: "node", Version: "22.12.0", Path: nodes[2], RealPath: nodes[2], Source: src("22.12.0")},
	})
}

func TestNodeVolta(t *testing.T) {
	root := root(t)
	volta := filepath.Join(root, "home", ".volta")
	shim := fakeNode(t, volta) // ~/.volta/bin/node
	v22 := fakeNode(t, filepath.Join(volta, "tools", "image", "node", "22.11.0"))
	v20 := fakeNode(t, filepath.Join(volta, "tools", "image", "node", "20.18.0"))
	writeFile(t, filepath.Join(volta, "tools", "user", "platform.json"), `{"node":{"runtime":"22.11.0","npm":null}}`)
	term := homeEnv(root, volta, "bin")
	r := &run.Fake{}

	got, err := Node(t.Context(), r, term, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	image := func(v string) inventory.Source {
		return inventory.Source{Kind: vm, Manager: "volta", Root: filepath.Join(volta, "tools", "image", "node", v), Version: v}
	}
	checkInstallations(t, got, []inventory.Installation{
		{Tool: "node", Path: shim, RealPath: shim, Source: inventory.Source{Kind: vm, Manager: "volta", Root: volta, Shim: true}, OnPath: 1},
		{Tool: "node", Version: "20.18.0", Path: v20, RealPath: v20, Source: image("20.18.0")},
		{Tool: "node", Version: "22.11.0", Path: v22, RealPath: v22, Source: image("22.11.0"), Default: true},
	})
	if calls := r.Calls(); len(calls) != 0 {
		t.Errorf("ran %v; a shim must not run", calls)
	}
}

func TestNodeMise(t *testing.T) {
	root := root(t)
	mise := filepath.Join(root, "home", ".local", "share", "mise")
	miseBin := filepath.Join(root, "brew", "bin", "mise")
	writeFile(t, miseBin, "")
	if err := os.Chmod(miseBin, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(mise, "shims", "node")
	symlink(t, miseBin, shim) // mise's shims link to mise itself
	v22 := fakeNode(t, filepath.Join(mise, "installs", "node", "22.11.0"))
	symlink(t, "22.11.0", filepath.Join(mise, "installs", "node", "22")) // an alias mise keeps
	writeFile(t, filepath.Join(root, "home", ".config", "mise", "config.toml"), "[tools]\nnode = \"22\"\n")
	term := homeEnv(root, mise, "shims")

	got, err := Node(t.Context(), &run.Fake{}, term, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	checkInstallations(t, got, []inventory.Installation{
		{Tool: "node", Path: shim, RealPath: miseBin, Source: inventory.Source{Kind: vm, Manager: "mise", Root: mise, Shim: true}, OnPath: 1},
		{
			Tool: "node", Version: "22.11.0", Path: v22, RealPath: v22,
			Source:  inventory.Source{Kind: vm, Manager: "mise", Root: filepath.Join(mise, "installs", "node", "22.11.0"), Version: "22.11.0"},
			Default: true,
		},
	})
}

func TestNodeVersionCommand(t *testing.T) {
	root := root(t)
	node := fakeNode(t, filepath.Join(root, "custom"))
	broken := fakeNode(t, filepath.Join(root, "broken"))
	term := append(homeEnv(root), "PATH="+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Dir(broken))
	r := &run.Fake{}
	r.Stdout("v18.19.1\n", node, "--version")
	r.Set(run.Result{ExitCode: 1}, nil, broken, "--version")

	got, err := Node(t.Context(), r, term, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	checkInstallations(t, got, []inventory.Installation{
		{Tool: "node", Version: "18.19.1", Path: node, RealPath: node, OnPath: 1},
		{Tool: "node", Path: broken, RealPath: broken, OnPath: 2},
	})
}

func startupFile(home string) string { return filepath.Join(home, ".zshrc") }

// corepack enable on macOS and Linux links pnpm and yarn to Corepack's
// scripts.
func TestCorepackSymlink(t *testing.T) {
	root := root(t)
	node := fakeNode(t, filepath.Join(root, "prefix"))
	script := filepath.Join(root, "prefix", "lib", "node_modules", "corepack", "dist", "yarn.js")
	writeFile(t, script, "")
	symlink(t, script, filepath.Join(root, "prefix", "bin", "yarn"))
	if !corepackEnabled(node, node) {
		t.Error("yarn linked to Corepack, not enabled")
	}
}
