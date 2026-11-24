package detect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ownPage are the tools Hearth's detectors find on their own, mise's
// copies among them; mise's other tools go to its list.
var ownPage = []string{"node", "bun", "npm", "pnpm", "yarn", "corepack", "go", "python", "php", "composer", "rust", "java", "ruby", "dotnet", "docker", "git"}

// MiseTools records mise itself when installed and lists the tools mise
// installed that have no page of their own, by its installs/ directory:
// mise is only invoked for --version when its path does not carry it, never
// to evaluate user config or go online. A version's folder on the
// terminal's or the apps' PATH puts it there, as mise activate does; with
// mise's shims, the default version is the one that runs.
func MiseTools(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	st := inventory.ToolState{Tool: "mise"}
	d := sources.ManagerDirs(term)
	if m := miseManager(ctx, r, term, gui, d); m.Version != "" {
		st.Managers = []inventory.Manager{m}
	}
	if d.Mise == "" {
		return st, nil
	}
	installs := filepath.Join(d.Mise, "installs")
	tools, err := os.ReadDir(installs)
	if err != nil {
		return st, nil // no mise installs yet
	}
	shims := filepath.Join(d.Mise, "shims")
	termShims, guiShims := onPath(term, shims), onPath(gui, shims)
	for _, tool := range tools {
		name := tool.Name()
		if !tool.IsDir() || strings.HasPrefix(name, ".") || slices.Contains(ownPage, name) {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(installs, name))
		if err != nil {
			continue
		}
		mt := inventory.MiseTool{Name: name}
		var names []string
		for _, v := range versions {
			// latest, 20 and the like are mise's links to a version; an
			// empty folder is an install that did not finish.
			if !v.IsDir() || v.Type()&os.ModeSymlink != 0 || strings.HasPrefix(v.Name(), ".") {
				continue
			}
			if files, err := os.ReadDir(filepath.Join(installs, name, v.Name())); err != nil || len(files) == 0 {
				continue
			}
			names = append(names, v.Name())
		}
		def := miseDefault(d, term, name, names)
		for _, v := range names {
			root := filepath.Join(installs, name, v)
			in := inventory.Installation{Tool: name, Version: v, Path: root, RealPath: root, Default: v == def,
				Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Root: root, Version: v}}
			in.OnPath, in.InGUI = rankUnder(term, root), rankUnder(gui, root)
			if in.Default && in.OnPath == 0 && termShims {
				in.OnPath = 1
			}
			if in.Default && in.InGUI == 0 && guiShims {
				in.InGUI = 1
			}
			mt.Installations = append(mt.Installations, in)
		}
		if len(mt.Installations) == 0 {
			continue
		}
		slices.SortStableFunc(mt.Installations, func(a, b inventory.Installation) int {
			return semver.Compare("v"+b.Version, "v"+a.Version)
		})
		st.MiseTools = append(st.MiseTools, mt)
	}
	return st, nil
}

// rankUnder is 1 when a folder under root is on env's PATH, as mise
// activate puts a version's bin there; 0 when none is.
func rankUnder(env sysenv.Env, root string) inventory.PathRank {
	for _, p := range filepath.SplitList(env.Get("PATH")) {
		if p != "" && within(filepath.Clean(p), root) {
			return 1
		}
	}
	return 0
}
