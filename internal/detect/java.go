package detect

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Java reads installed JDKs without executing Java or a manager's shims.
func Java(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	d := sources.ManagerDirs(term)
	sdk := term.Get("SDKMAN_DIR")
	if sdk == "" && d.Home != "" {
		sdk = filepath.Join(d.Home, ".sdkman")
	}
	paths, systemHome := javaSystemPaths(ctx, r, term)
	f := newFinder("java", term)
	for n, env := range []sysenv.Env{term, gui} {
		for i, path := range sysenv.LookPathAll(javaExe, env) {
			if javaLauncher(path) {
				selected := env.Get("JAVA_HOME")
				if selected == "" || !exists(filepath.Join(selected, "bin", javaExe)) {
					selected = systemHome
				}
				if selected != "" {
					path = filepath.Join(selected, "bin", javaExe)
				}
			}
			in := f.add(path)
			if n == 0 && in.OnPath == 0 {
				in.OnPath = inventory.PathRank(i + 1)
			}
			if n == 1 && in.InGUI == 0 {
				in.InGUI = inventory.PathRank(i + 1)
			}
		}
	}
	for _, env := range []sysenv.Env{term, gui} {
		if home := env.Get("JAVA_HOME"); home != "" {
			paths = append(paths, filepath.Join(home, "bin", javaExe))
		}
	}
	for _, base := range []struct{ dir, pattern string }{{d.Mise, "installs/java/*"}, {d.Asdf, "installs/java/*"}, {sdk, "candidates/java/*"}} {
		if base.dir != "" {
			paths = append(paths, filepath.Join(base.dir, filepath.FromSlash(base.pattern), "bin", javaExe))
		}
	}
	for _, pattern := range paths {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			if exists(path) {
				f.add(path)
			}
		}
	}
	st := inventory.ToolState{Tool: "java", Installations: f.ins, Vars: map[string]string{"JAVA_HOME": term.Get("JAVA_HOME")}}
	if home := term.Get("JAVA_HOME"); home != "" {
		real, err := sysenv.RealPath(home)
		if err == nil {
			st.Vars["JAVA_HOME_REAL"] = real
		}
	}
	for _, command := range []string{"gradle", "mvn"} {
		if found := sysenv.LookPathAll(command, term); len(found) > 0 {
			st.Vars[command] = found[0]
		}
	}

	for i := range st.Installations {
		in := &st.Installations[i]
		if javaLauncher(in.Path) {
			in.Stub = true
			continue
		}
		if in.Source.Shim {
			continue
		}
		root := filepath.Dir(filepath.Dir(in.RealPath))
		props := javaProperties(readFile(filepath.Join(root, "release")))
		in.Version = javaVersion(props["JAVA_VERSION"])
		if in.Version != "" && in.Source.Kind == inventory.KindUnknown {
			in.Source.Kind = inventory.KindInstaller
			in.Source.Root = root
		}
		in.Java = &inventory.JDK{Home: root, Vendor: props["IMPLEMENTOR"]}
		if sdk != "" && within(root, filepath.Join(sdk, "candidates", "java")) {
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: "sdkman", Root: root, Version: filepath.Base(root)}
			current, _ := sysenv.RealPath(filepath.Join(sdk, "candidates", "java", "current"))
			in.Default = current == root
		}
	}
	want := miseTool(miseConfig(d, term), "java")
	best := -1
	for i, in := range st.Installations {
		if in.Source.Manager != "mise" || in.Source.Shim || want == "" {
			continue
		}
		if in.Source.Version == want || strings.HasPrefix(in.Source.Version, want+".") || in.Version == want || strings.HasPrefix(in.Version, want+".") {
			if best < 0 || semver.Compare("v"+in.Version, "v"+st.Installations[best].Version) > 0 {
				best = i
			}
		}
	}
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Manager == "mise" && !in.Source.Shim {
			in.Default = i == best
		}
	}
	if slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool { return in.Source.Manager == "mise" }) {
		st.Managers = append(st.Managers, miseManager(ctx, r, term, gui, d))
	}
	if sdk != "" && exists(filepath.Join(sdk, "candidates", "java")) {
		config := javaProperties(readFile(filepath.Join(sdk, "etc", "config")))
		st.Managers = append(st.Managers, inventory.Manager{Name: "sdkman", SwitchOnCD: config["sdkman_auto_env"] == "true", Files: []string{".sdkmanrc"}})
	}
	return st, ctx.Err()
}

// javaProperties reads release and SDKMAN key=value files without evaluating them.
func javaProperties(text string) map[string]string {
	out := map[string]string{}
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return out
}

// javaVersion makes Java 8's legacy version comparable with modern JDK versions.
func javaVersion(v string) string {
	if rest, ok := strings.CutPrefix(v, "1."); ok {
		if line, update, ok := strings.Cut(rest, "_"); ok {
			major, _, _ := strings.Cut(line, ".")
			return major + ".0." + update
		}
	}
	v, _, _ = strings.Cut(v, "+")
	return v
}
