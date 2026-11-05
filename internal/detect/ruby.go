package detect

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Ruby finds MRI copies without running shims or evaluating Ruby project files.
func Ruby(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	d := sources.ManagerDirs(term)
	f := newFinder("ruby", term)
	f.lookPath(rubyExe, term, gui)
	patterns := rubyOSGlobs(term)
	for _, base := range []struct{ dir, pattern string }{{d.Rbenv, "versions/*"}, {d.Rvm, "rubies/*"}, {d.Mise, "installs/ruby/*"}, {d.Asdf, "installs/ruby/*"}} {
		if base.dir != "" {
			patterns = append(patterns, filepath.Join(base.dir, filepath.FromSlash(base.pattern), "bin", rubyExe))
		}
	}
	if d.Home != "" {
		patterns = append(patterns, filepath.Join(d.Home, ".rubies", "*", "bin", rubyExe))
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			if exists(path) {
				f.add(path)
			}
		}
	}
	st := inventory.ToolState{Tool: "ruby", Installations: f.ins}
	var miseVersions []string
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		if root := rubyInstallerRoot(in.RealPath); root != "" {
			in.Source = inventory.Source{Kind: inventory.KindInstaller, Manager: "RubyInstaller", Root: root}
		}
		in.Version = rubyVersion.FindString(pathVersion(strings.TrimPrefix(in.Source.Version, "ruby-")))
		if in.Version == "" {
			in.Version = firstMatch(rubyCLIVersion, rubyOutput(ctx, r, term, in.RealPath, "--version"))
		}
		if in.Source.Manager == "mise" {
			miseVersions = append(miseVersions, in.Version)
		}
	}
	miseDef := miseDefault(d, term, "ruby", miseVersions)
	rbenvDef := strings.TrimSpace(term.Get("RBENV_VERSION"))
	if rbenvDef == "" {
		rbenvDef = strings.TrimSpace(readFile(filepath.Join(d.Rbenv, "version")))
	}
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		switch in.Source.Manager {
		case "rbenv":
			in.Default = in.Source.Version == rbenvDef
		case "asdf":
			want := term.Get("ASDF_RUBY_VERSION")
			if want == "" && d.Home != "" {
				want = toolVersion(filepath.Join(d.Home, ".tool-versions"), "ruby")
			}
			in.Default = want != "" && in.Source.Version == want
		case "mise":
			in.Default = miseDef != "" && in.Version == miseDef
		}
	}
	for _, manager := range []string{"rbenv", "rvm", "chruby", "asdf", "mise"} {
		if !slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool { return in.Source.Manager == manager }) {
			continue
		}
		m := inventory.Manager{Name: manager, Files: []string{".ruby-version"}, SwitchOnCD: true}
		switch manager {
		case "rbenv":
			m.SwitchOnCD = term.Get("RBENV_VERSION") == ""
			m.Shims = onPath(term, filepath.Join(d.Rbenv, "shims"))
			m.ShimsInGUI = onPath(gui, filepath.Join(d.Rbenv, "shims"))
			if paths := sysenv.LookPathAll("rbenv", term); len(paths) > 0 {
				m.Version = rubyVersion.FindString(phpOutput(ctx, r, term, paths[0], "--version"))
			}
		case "asdf":
			m.Files = []string{".tool-versions"}
		case "mise":
			m = miseManager(ctx, r, term, gui, d)
		case "chruby":
			m.SwitchOnCD = startupRuns(term, d.Home, "chruby/auto.sh")
		}
		st.Managers = append(st.Managers, m)
	}
	if active := inventory.TerminalCopy(st.Installations); active >= 0 {
		in := &st.Installations[active]
		in.Packages = rubyGems(in.RealPath, term)
	}
	return st, ctx.Err()
}

var (
	rubyVersion    = regexp.MustCompile(`\b\d+\.\d+\.\d+\b`)
	rubyCLIVersion = regexp.MustCompile(`(?m)^ruby (\d+\.\d+\.\d+)`)
	gemSpecName    = regexp.MustCompile(`^(.+)-(\d+\.[0-9A-Za-z.]+)\.gemspec$`)
)

// rubyOutput prevents RUBYOPT from loading user code during a probe.
func rubyOutput(ctx context.Context, r run.Runner, env sysenv.Env, path string, args ...string) string {
	clean := make(sysenv.Env, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "RUBYOPT") && !strings.EqualFold(k, "RUBYLIB") {
			clean = append(clean, kv)
		}
	}
	return phpOutput(ctx, r, clean, path, args...)
}

// rubyGems reads names from installed specification filenames without evaluating
// gemspecs, loading RubyGems plugins, or running gem commands.
func rubyGems(path string, env sysenv.Env) []inventory.Package {
	root := filepath.Dir(filepath.Dir(path))
	homes, _ := filepath.Glob(filepath.Join(root, "lib", "ruby", "gems", "*"))
	if home := env.Get("GEM_HOME"); home != "" {
		homes = append(homes, home)
	}
	if paths := env.Get("GEM_PATH"); paths != "" {
		homes = append(homes, filepath.SplitList(paths)...)
	}
	var pkgs []inventory.Package
	seen := map[string]bool{}
	for _, home := range homes {
		for _, pattern := range []string{"specifications/*.gemspec", "specifications/default/*.gemspec"} {
			files, _ := filepath.Glob(filepath.Join(home, filepath.FromSlash(pattern)))
			for _, file := range files {
				m := gemSpecName.FindStringSubmatch(filepath.Base(file))
				if m == nil || seen[m[0]] {
					continue
				}
				seen[m[0]] = true
				pkgs = append(pkgs, inventory.Package{Name: m[1], Version: m[2]})
			}
		}
	}
	slices.SortFunc(pkgs, func(a, b inventory.Package) int { return strings.Compare(a.Name+" "+a.Version, b.Name+" "+b.Version) })
	return pkgs
}
