package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Python finds every copy of Python: on the terminal's PATH (term), on
// the GUI's (gui), and where pyenv, uv, mise, asdf, conda, Homebrew and
// the official installers keep them. Several copies are normal: the
// system's, a manager's, Homebrew's as other formulas' dependency. A
// stub, which would open the Store or offer to install the Command Line
// Tools, is marked and never run. The version comes from the path when it
// names one, else from python --version. The commands installed by uv
// tool and pipx come with every copy, since they bring their own Python.
func Python(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("python", term)
	f.lookPath(pythonExe, term, gui)
	dirs := sources.ManagerDirs(term)
	for _, pattern := range pythonGlobs(dirs, term) {
		matches, _ := filepath.Glob(pattern) // only a malformed pattern fails
		for _, m := range matches {
			if real, err := sysenv.RealPath(m); err == nil {
				f.add(real)
			}
		}
	}

	tools := slices.Concat(uvTools(uvToolDir(term, dirs.Home)), pipxTools(pipxHome(term, dirs.Home)))
	installed := map[string][]string{}
	state := inventory.ToolState{Tool: "python", Installations: f.ins}
	for i := range state.Installations {
		in := &state.Installations[i]
		if pythonManagerAlias(in.Path) {
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: "pymanager", Shim: true}
		}
		if in.Source.Shim {
			continue
		}
		if pythonStub(in.Path, in.RealPath) {
			in.Stub = true
			continue
		}
		if v := pathVersion(in.Source.Version); pythonVersionRe.MatchString(v) {
			in.Version = v
		} else {
			in.Version = pythonRunVersion(ctx, r, term, in.RealPath)
		}
		in.Packages = tools
		if m := in.Source.Manager; m != "" && in.Version != "" {
			installed[m] = append(installed[m], in.Version)
		}
	}
	defaults := map[string]string{
		"pyenv": pickVersion(firstLine(readFile(filepath.Join(dirs.Pyenv, "version"))), installed["pyenv"]),
		"mise":  miseDefault(dirs, term, "python", installed["mise"]),
		// Python's install manager prefers its own Pythons to others.
		"pymanager": pickVersion(pythonManagerTag(term), installed["pymanager"]),
	}
	for i := range state.Installations {
		in := &state.Installations[i]
		if want := defaults[in.Source.Manager]; want != "" && !in.Source.Shim && in.Version == want {
			in.Default = true
		}
	}
	state.Managers = pythonManagers(ctx, r, state.Installations, term, gui, dirs)
	return state, ctx.Err()
}

// pythonVersionRe is a Python version, as 3.12.8; a pyenv directory may
// be named otherwise, as miniconda3-latest.
var pythonVersionRe = regexp.MustCompile(`^\d+\.\d+(\.\d+)?`)

// pythonGlobs are where the managers and installers keep Pythons, one
// match per copy.
func pythonGlobs(d sources.Dirs, env sysenv.Env) []string {
	var globs []string
	glob := func(dir string, parts ...string) {
		if dir != "" {
			globs = append(globs, filepath.Join(append([]string{dir}, parts...)...))
		}
	}
	glob(d.Pyenv, "versions", "*", pythonBin)
	glob(d.Uv, "*", pythonBin)
	glob(d.Mise, "installs", "python", "*", pythonBin)
	glob(d.Asdf, "installs", "python", "*", pythonBin)
	for _, conda := range d.Conda {
		glob(conda, pythonBin)
	}
	return append(globs, pythonOSGlobs(env)...)
}

// pythonRunVersion asks the python at path its version: python --version
// prints "Python 3.13.1".
func pythonRunVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	v := versionOf(ctx, r, env, path)
	return strings.TrimPrefix(v, "Python ")
}

// pythonManagers tells how the version managers that keep Pythons are set
// up: pyenv's shims switch by .python-version; mise as it is hooked in;
// uv, which python3 does not go through, by its settings of when it
// downloads Pythons and whether it prefers its own.
func pythonManagers(ctx context.Context, r run.Runner, ins []inventory.Installation, term, gui sysenv.Env, d sources.Dirs) []inventory.Manager {
	var managers []inventory.Manager
	has := func(name string) bool {
		return slices.ContainsFunc(ins, func(in inventory.Installation) bool { return in.Source.Manager == name })
	}
	if has("pyenv") {
		managers = append(managers, inventory.Manager{Name: "pyenv", SwitchOnCD: true, Files: []string{".python-version"}})
	}
	if has("mise") {
		managers = append(managers, miseManager(ctx, r, term, gui, d))
	}
	if has("pymanager") {
		managers = append(managers, inventory.Manager{Name: "pymanager"})
	}
	if uv := sysenv.LookPathAll("uv", term); len(uv) > 0 || has("uv") {
		m := inventory.Manager{Name: "uv", Settings: uvSettings(term, d.Home)}
		if len(uv) > 0 {
			m.Version = uvVersion(ctx, r, term, uv[0])
		}
		managers = append(managers, m)
	}
	return managers
}

// uvVersionRe finds uv's version in its path or uv --version's output,
// "uv 0.5.21 (2025-01-20)".
var uvVersionRe = regexp.MustCompile(`\b0\.\d+\.\d+\b|\b[1-9]\d*\.\d+\.\d+\b`)

// uvVersion is the version of the uv at path: from Homebrew's directory
// when the path names it, else from uv --version.
func uvVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	if real, err := sysenv.RealPath(path); err == nil && strings.Contains(real, string(filepath.Separator)+"Cellar"+string(filepath.Separator)) {
		if v := uvVersionRe.FindString(real); v != "" {
			return v
		}
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "--version"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return uvVersionRe.FindString(string(res.Stdout))
}

// uvSettings reads uv's settings of which Python it uses: when it
// downloads one (python-downloads: automatic, manual, never) and whether
// it prefers its own (python-preference: managed, only-managed, system,
// only-system). The environment wins over uv.toml; unset, uv's defaults.
func uvSettings(env sysenv.Env, home string) map[string]string {
	var config struct {
		Downloads  string `toml:"python-downloads"`
		Preference string `toml:"python-preference"`
	}
	if path := uvConfigFile(env, home); path != "" {
		_, _ = toml.DecodeFile(path, &config) // none or unreadable: the defaults
	}
	pick := func(name, file, fallback string) string {
		if v := env.Get(name); v != "" {
			return v
		}
		if file != "" {
			return file
		}
		return fallback
	}
	return map[string]string{
		"python-downloads":  pick("UV_PYTHON_DOWNLOADS", config.Downloads, "automatic"),
		"python-preference": pick("UV_PYTHON_PREFERENCE", config.Preference, "managed"),
	}
}

// uvConfigFile is the user's uv.toml: UV_CONFIG_FILE, else uv in the
// config directory, AppData on Windows.
func uvConfigFile(env sysenv.Env, home string) string {
	if f := env.Get("UV_CONFIG_FILE"); f != "" {
		return f
	}
	if appData := env.Get("APPDATA"); appData != "" {
		return filepath.Join(appData, "uv", "uv.toml")
	}
	if xdg := env.Get("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "uv", "uv.toml")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "uv", "uv.toml")
}

// uvToolDir is where uv tool installs: UV_TOOL_DIR, else uv/tools in the
// data directory, AppData on Windows.
func uvToolDir(env sysenv.Env, home string) string {
	if dir := env.Get("UV_TOOL_DIR"); dir != "" {
		return dir
	}
	if appData := env.Get("APPDATA"); appData != "" {
		return filepath.Join(appData, "uv", "tools")
	}
	if xdg := env.Get("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "uv", "tools")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "uv", "tools")
}

// uvTools reads the tools uv tool installed in dir, each in a folder of
// its own: the commands from its receipt, the version from its package's
// dist-info folder.
func uvTools(dir string) []inventory.Package {
	if dir == "" {
		return nil
	}
	entries, _ := os.ReadDir(dir) // no directory, no tools
	var tools []inventory.Package
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		var receipt struct {
			Tool struct {
				Entrypoints []struct{ Name string }
			}
		}
		if _, err := toml.DecodeFile(filepath.Join(dir, e.Name(), "uv-receipt.toml"), &receipt); err != nil {
			continue
		}
		p := inventory.Package{Name: e.Name(), Version: distVersion(filepath.Join(dir, e.Name()), e.Name())}
		for _, ep := range receipt.Tool.Entrypoints {
			p.Commands = append(p.Commands, strings.TrimSuffix(ep.Name, ".exe"))
		}
		tools = append(tools, p)
	}
	return tools
}

// distVersion is the version of package name installed in the
// environment at venv, by its dist-info folder, as ruff-0.9.2.dist-info.
func distVersion(venv, name string) string {
	norm := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(name))
	for _, sitePackages := range []string{
		filepath.Join(venv, "lib", "python*", "site-packages"),
		filepath.Join(venv, "Lib", "site-packages"),
	} {
		matches, _ := filepath.Glob(filepath.Join(sitePackages, "*.dist-info")) // only a malformed pattern fails
		for _, m := range matches {
			base := strings.TrimSuffix(filepath.Base(m), ".dist-info")
			if pkg, v, ok := strings.Cut(base, "-"); ok && strings.ToLower(pkg) == norm {
				return v
			}
		}
	}
	return ""
}

// pipxHome is where pipx keeps its environments: PIPX_HOME, else
// pipx in the data directory, or the older ~/.local/pipx when it is there.
func pipxHome(env sysenv.Env, home string) string {
	if dir := env.Get("PIPX_HOME"); dir != "" {
		return dir
	}
	if home == "" {
		return ""
	}
	if old := filepath.Join(home, ".local", "pipx"); exists(old) {
		return old
	}
	if xdg := env.Get("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "pipx")
	}
	if localAppData := env.Get("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, "pipx", "pipx")
	}
	return filepath.Join(home, ".local", "share", "pipx")
}

// pipxTools reads the apps pipx installed, from each environment's
// pipx_metadata.json.
func pipxTools(home string) []inventory.Package {
	if home == "" {
		return nil
	}
	venvs := filepath.Join(home, "venvs")
	entries, _ := os.ReadDir(venvs) // no directory, no tools
	var tools []inventory.Package
	for _, e := range entries {
		var meta struct {
			MainPackage struct {
				Package        string
				PackageVersion string `json:"package_version"`
				Apps           []string
			} `json:"main_package"`
		}
		if json.Unmarshal([]byte(readFile(filepath.Join(venvs, e.Name(), "pipx_metadata.json"))), &meta) != nil {
			continue
		}
		p := inventory.Package{Name: meta.MainPackage.Package, Version: meta.MainPackage.PackageVersion}
		if p.Name == "" {
			p.Name = e.Name()
		}
		for _, app := range meta.MainPackage.Apps {
			p.Commands = append(p.Commands, strings.TrimSuffix(app, ".exe"))
		}
		tools = append(tools, p)
	}
	return tools
}
