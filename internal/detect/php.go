package detect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// PHP finds CLI copies, including unlinked Homebrew kegs and Herd's PHPs.
// Manager shims are never executed. Configuration is read only for the copy
// the terminal runs, under the terminal environment; it does not describe a
// web server's PHP.
func PHP(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("php", term)
	f.lookPath("php", term, gui)
	d := sources.ManagerDirs(term)
	patterns := phpOSGlobs(term, d.Home)
	for _, dir := range []string{d.Mise, d.Asdf} {
		if dir != "" {
			patterns = append(patterns, filepath.Join(dir, "installs", "php", "*", phpBin))
		}
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && !info.IsDir() && phpExecutable.MatchString(filepath.Base(m)) {
				f.add(m)
			}
		}
	}
	st := inventory.ToolState{Tool: "php", Installations: f.ins}
	var installed []string
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		if phpHerd(in.Path, d.Home) || phpHerd(in.RealPath, d.Home) {
			in.Source = inventory.Source{Kind: inventory.KindInstaller, Manager: "Herd", Root: filepath.Dir(in.RealPath)}
		}
		in.Version = phpVersion.FindString(pathVersion(in.Source.Version))
		if in.Version == "" {
			in.Version = firstMatch(phpCLIVersion, phpOutput(ctx, r, term, in.RealPath, "-v"))
		}
		if in.Source.Manager == "mise" {
			installed = append(installed, in.Version)
		}
	}
	def := miseDefault(d, term, "php", installed)
	for i := range st.Installations {
		in := &st.Installations[i]
		in.Default = def != "" && in.Source.Manager == "mise" && !in.Source.Shim && in.Version == def
	}
	if i := inventory.TerminalCopy(st.Installations); i >= 0 {
		st.Installations[i].PHP = phpConfig(ctx, r, term, st.Installations[i].RealPath)
	}
	if len(installed) > 0 {
		st.Managers = []inventory.Manager{miseManager(ctx, r, term, gui, d)}
	}
	return st, ctx.Err()
}

// composerPathVersion is a version a path names, as Homebrew's and
// Scoop's directories do.
var composerPathVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var phpExecutable = regexp.MustCompile(`^php(?:[0-9]+(?:\.[0-9]+)*)?(?:\.exe)?$`)

var phpVersion = regexp.MustCompile(`\b\d+\.\d+\.\d+(?:[A-Za-z]+\d*)?\b`)

// phpCLIVersion and composerVersion skip startup warnings printed before
// the version line, which may name other copies' versions.
var (
	phpCLIVersion   = regexp.MustCompile(`(?m)^PHP (\d+\.\d+\.\d+(?:[A-Za-z]+\d*)?)\b`)
	composerVersion = regexp.MustCompile(`(?m)^Composer version (\d+\.\d+\.\d+(?:[A-Za-z0-9-]*)?)\b`)
)

// firstMatch returns re's first group in out, or "".
func firstMatch(re *regexp.Regexp, out string) string {
	if m := re.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// phpConfig reads the ini files and extensions of the copy at path.
func phpConfig(ctx context.Context, r run.Runner, env sysenv.Env, path string) *inventory.PHPConfig {
	ini := phpOutput(ctx, r, env, path, "--ini")
	mods := phpOutput(ctx, r, env, path, "-m")
	cfg := &inventory.PHPConfig{Files: phpIniFiles(ini), Extensions: phpModules(mods)}
	if ini == "" || mods == "" {
		cfg.Err = "could not read PHP CLI configuration"
	}
	return cfg
}

func phpOutput(ctx context.Context, r run.Runner, env sysenv.Env, path string, args ...string) string {
	res, err := r.Run(ctx, run.Cmd{Args: append([]string{path}, args...), Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(res.Stdout))
}

// phpIniFiles reads only loaded files, excluding the search directory and (none).
func phpIniFiles(out string) []string {
	var files []string
	additional := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Loaded Configuration File:"):
			line = strings.TrimSpace(strings.TrimPrefix(line, "Loaded Configuration File:"))
		case strings.HasPrefix(line, "Additional .ini files parsed:"):
			line = strings.TrimSpace(strings.TrimPrefix(line, "Additional .ini files parsed:"))
			additional = true
		case !additional:
			continue
		}
		for path := range strings.SplitSeq(line, ",") {
			// PHP 8.5 quotes the loaded file: "/etc/php.ini".
			if path = strings.Trim(strings.TrimSpace(path), `"`); filepath.IsAbs(path) && !slices.Contains(files, path) {
				files = append(files, path)
			}
		}
	}
	return files
}

func phpModules(out string) []string {
	var mods []string
	section := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line == "[PHP Modules]" || line == "[Zend Modules]"
			continue
		}
		if section && line != "" && !slices.Contains(mods, line) {
			mods = append(mods, line)
		}
	}
	slices.Sort(mods)
	return mods
}

// Composer reads the CLI version and installed global packages. Plugins and
// scripts are disabled, and no project directory is used to inspect globals.
// Composer is run only for what its path and COMPOSER_HOME do not tell:
// Scoop's starts PowerShell, about a second each run.
func Composer(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("composer", term)
	f.lookPath("composer", term, gui)
	home := sources.ManagerDirs(term).Home
	st := inventory.ToolState{Tool: "composer"}
	for i := range f.ins {
		in := &f.ins[i]
		if in.Source.Shim {
			continue
		}
		query := func(args ...string) string {
			argv := append([]string{in.Path, "--no-plugins", "--no-scripts", "--no-interaction"}, args...)
			res, err := r.Run(ctx, run.Cmd{Args: argv, Env: term, Dir: home})
			if err != nil || res.ExitCode != 0 {
				return ""
			}
			return strings.TrimSpace(string(res.Stdout))
		}
		if v := pathVersion(in.Source.Version); composerPathVersion.MatchString(v) {
			in.Version = v
		} else {
			in.Version = firstMatch(composerVersion, query("--version"))
		}
		dir := term.Get("COMPOSER_HOME")
		if dir == "" {
			dir = query("config", "--global", "home")
		}
		if filepath.IsAbs(dir) {
			in.Packages = composerPackages(dir)
			if in.OnPath == 1 {
				st.Vars = map[string]string{"COMPOSER_HOME": dir}
			}
		}
	}
	st.Installations = f.ins
	return st, ctx.Err()
}

func composerPackages(home string) []inventory.Package {
	var manifest struct{ Require map[string]string }
	if json.Unmarshal([]byte(readFile(filepath.Join(home, "composer.json"))), &manifest) != nil {
		return nil
	}
	var installed struct {
		Packages []struct {
			Name, Version string
			Bin           []string
		}
	}
	data := []byte(readFile(filepath.Join(home, "vendor", "composer", "installed.json")))
	if json.Unmarshal(data, &installed) != nil {
		_ = json.Unmarshal(data, &installed.Packages)
	}
	var pkgs []inventory.Package
	for _, p := range installed.Packages {
		if _, ok := manifest.Require[p.Name]; !ok {
			continue
		}
		pkg := inventory.Package{Name: p.Name, Version: strings.TrimPrefix(p.Version, "v")}
		for _, bin := range p.Bin {
			pkg.Commands = append(pkg.Commands, filepath.Base(bin))
		}
		pkgs = append(pkgs, pkg)
	}
	slices.SortFunc(pkgs, func(a, b inventory.Package) int { return strings.Compare(a.Name, b.Name) })
	return pkgs
}
