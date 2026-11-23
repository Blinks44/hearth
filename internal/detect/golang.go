package detect

import (
	"bufio"
	"context"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// GoToolchain is the Manager of a toolchain the go command downloaded
// itself, as a go.mod's toolchain line asks with GOTOOLCHAIN=auto.
const GoToolchain = "go"

// Go finds every copy of Go: on the terminal's PATH (term), on the GUI's
// (gui), in mise's directory, the official installer's, and the module
// cache, where go keeps the toolchains it downloads. The version is read
// from the VERSION file beside a copy, else from go version; nothing is
// run that could download a toolchain. GOPATH, GOMODCACHE and GOTOOLCHAIN
// are read as go reads them, from the environment, go env -w's file and
// GOROOT's go.env.
func Go(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("go", term)
	f.lookPath("go", term, gui)
	dirs := sources.ManagerDirs(term)
	vars := goEnv{env: term, file: readGoEnvFile(goEnvFile(term, dirs.Home))}
	gopath := vars.gopath(dirs.Home)
	var globs []string
	if dirs.Mise != "" {
		globs = append(globs, filepath.Join(dirs.Mise, "installs", "go", "*", "bin", goExe))
	}
	for _, dir := range goInstallDirs(term) {
		globs = append(globs, filepath.Join(dir, "bin", goExe))
	}
	toolchains := filepath.Join(vars.modcache(gopath), "golang.org")
	globs = append(globs, filepath.Join(toolchains, "toolchain@*", "bin", goExe))
	for _, pattern := range globs {
		matches, _ := filepath.Glob(pattern) // only a malformed pattern fails
		for _, m := range matches {
			if real, err := sysenv.RealPath(m); err == nil {
				f.add(real)
			}
		}
	}

	tools := goTools(vars.bin(gopath))
	installed := map[string][]string{}
	state := inventory.ToolState{Tool: "go", Installations: f.ins}
	for i := range state.Installations {
		in := &state.Installations[i]
		if in.Source.Shim {
			continue
		}
		root := goRoot(in.RealPath)
		if within(root, toolchains) {
			in.Source = inventory.Source{Kind: inventory.KindVersionManager, Manager: GoToolchain, Root: root}
		}
		in.Version = goVersionFile(root)
		if in.Version == "" {
			in.Version = pathVersion(in.Source.Version)
		}
		if in.Version == "" {
			in.Version = goRunVersion(ctx, r, term, in.RealPath)
		}
		in.Packages = tools
		if m := in.Source.Manager; m != "" && in.Version != "" {
			installed[m] = append(installed[m], in.Version)
		}
	}
	if def := miseDefault(dirs, term, "go", installed["mise"]); def != "" {
		for i := range state.Installations {
			in := &state.Installations[i]
			in.Default = in.Source.Manager == "mise" && !in.Source.Shim && in.Version == def
		}
	}
	if slices.ContainsFunc(state.Installations, func(in inventory.Installation) bool { return in.Source.Manager == "mise" }) {
		state.Managers = []inventory.Manager{miseManager(ctx, r, term, gui, dirs)}
	}

	goroot := ""
	if i := inventory.TerminalCopy(state.Installations); i >= 0 {
		goroot = goRoot(state.Installations[i].RealPath)
	}
	vars.root = readGoEnvFile(filepath.Join(goroot, "go.env"))
	state.Vars = map[string]string{
		"GOPATH":      gopath,
		"GOBIN":       vars.bin(gopath),
		"GOMODCACHE":  vars.modcache(gopath),
		"GOTOOLCHAIN": vars.get("GOTOOLCHAIN", "auto"),
	}
	return state, ctx.Err()
}

// goRoot is the GOROOT of the go at real, which lies in GOROOT/bin.
func goRoot(real string) string {
	return filepath.Dir(filepath.Dir(real))
}

// goVersionFile reads the version from GOROOT's VERSION file, whose first
// line is like go1.23.4; "" without one.
func goVersionFile(root string) string {
	line := firstLine(readFile(filepath.Join(root, "VERSION")))
	if !strings.HasPrefix(line, "go1") {
		return ""
	}
	return strings.TrimPrefix(line, "go")
}

// goRunVersion asks the go at path its version, by go version, which
// prints "go version go1.22.2 linux/amd64". GOTOOLCHAIN=local keeps it
// from switching to, and downloading, another toolchain.
func goRunVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "version"}, Env: append(slices.Clone(env), "GOTOOLCHAIN=local")})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	for field := range strings.FieldsSeq(string(res.Stdout)) {
		if strings.HasPrefix(field, "go1") {
			return strings.TrimPrefix(field, "go")
		}
	}
	return ""
}

// goEnv reads Go's variables as go does: the environment first, then the
// file go env -w writes, then GOROOT's go.env.
type goEnv struct {
	env        sysenv.Env
	file, root map[string]string
}

func (g goEnv) get(name, fallback string) string {
	if v := g.env.Get(name); v != "" {
		return v
	}
	if v := g.file[name]; v != "" {
		return v
	}
	if v := g.root[name]; v != "" {
		return v
	}
	return fallback
}

// gopath is the first directory of GOPATH, by default go in the home.
func (g goEnv) gopath(home string) string {
	fallback := ""
	if home != "" {
		fallback = filepath.Join(home, "go")
	}
	list := filepath.SplitList(g.get("GOPATH", fallback))
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// modcache is GOMODCACHE, by default pkg/mod in gopath.
func (g goEnv) modcache(gopath string) string {
	fallback := ""
	if gopath != "" {
		fallback = filepath.Join(gopath, "pkg", "mod")
	}
	return g.get("GOMODCACHE", fallback)
}

// bin is where go install puts commands: GOBIN, by default bin in gopath.
func (g goEnv) bin(gopath string) string {
	fallback := ""
	if gopath != "" {
		fallback = filepath.Join(gopath, "bin")
	}
	return g.get("GOBIN", fallback)
}

// goEnvFile is the file go env -w writes: GOENV, or go/env in the user's
// config directory.
func goEnvFile(env sysenv.Env, home string) string {
	if f := env.Get("GOENV"); f != "" && f != "off" {
		return f
	}
	if dir := userConfigDir(env, home); dir != "" {
		return filepath.Join(dir, "go", "env")
	}
	return ""
}

// readGoEnvFile reads a go env file's NAME=value lines.
func readGoEnvFile(path string) map[string]string {
	vars := map[string]string{}
	if path == "" {
		return vars
	}
	s := bufio.NewScanner(strings.NewReader(readFile(path)))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if name, value, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") {
			vars[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return vars
}

// goTools reads the commands go install put in dir, by the build
// information Go writes into them; files that are not Go programs are
// left out.
func goTools(dir string) []inventory.Package {
	if dir == "" {
		return nil
	}
	entries, _ := os.ReadDir(dir) // no directory, no tools
	var tools []inventory.Package
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := buildinfo.ReadFile(filepath.Join(dir, e.Name()))
		// Go's own commands, as go and gofmt in GOROOT/bin, belong to no
		// module: they are there when GOBIN is that folder, as mise sets it.
		if err != nil || info.Main.Path == "" && strings.HasPrefix(info.Path, "cmd/") {
			continue
		}
		tools = append(tools, inventory.Package{
			Name:    strings.TrimSuffix(e.Name(), ".exe"),
			Version: strings.TrimPrefix(info.Main.Version, "v"),
			Module:  info.Main.Path,
		})
	}
	return tools
}
