package detect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// cppTools are the commands of C and C++ builds the page shows, in its
// order: compilers, build systems, then what the toolset and the cache
// cards show. The versioned gcc-N found on PATH come after gcc.
var cppTools = []string{"clang", "gcc", "cmake", "ninja", "meson", "make", "pkg-config", "ccache"}

// cppVars are the terminal's variables that change how C and C++ build.
var cppVars = []string{"CC", "CXX", "SDKROOT", "CMAKE_GENERATOR", "CMAKE_C_COMPILER_LAUNCHER", "CMAKE_CXX_COMPILER_LAUNCHER"}

var (
	cppVersioned = regexp.MustCompile(`version (\d+(\.\d+)+)`)
	cppVersion   = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)
	gccNumbered  = regexp.MustCompile(`^gcc-\d+(\.exe)?$`)
)

// Cpp finds the C compiler's command and what it runs, the compilers and
// build tools on PATH, the OS's toolset, and the variables set for builds.
func Cpp(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("cpp", term)
	f.lookPath(exeName(ccCommand), term, gui)
	st := inventory.ToolState{Tool: "cpp", Installations: f.ins}
	s := &inventory.CppState{Command: ccCommand}
	st.Cpp = s
	active := inventory.TerminalCopy(st.Installations)
	for i := range st.Installations {
		in := &st.Installations[i]
		if cppStub(in.Path) {
			in.Stub = true
			continue
		}
		line := versionLine(ctx, r, term, in.Path)
		in.Version = lineVersion(line)
		if i == active {
			s.Compiler = compilerName(line, in.RealPath)
		}
	}
	seen := map[string]bool{}
	for _, name := range cppCommands(term) {
		g := newFinder(name, term)
		g.lookPath(exeName(name), term, gui)
		for _, in := range g.ins {
			if seen[in.RealPath] || cppStub(in.Path) {
				continue
			}
			seen[in.RealPath] = true
			line := versionLine(ctx, r, term, in.Path)
			// macOS's gcc is clang under that name.
			if name == "gcc" && strings.Contains(line, "clang") {
				s.GCCIsClang = true
				continue
			}
			if name == "clang" && strings.HasPrefix(line, "Apple clang") {
				s.AppleClang = true
			}
			in.Version = lineVersion(line)
			s.Tools = append(s.Tools, in)
		}
	}
	s.Base = cppBase(ctx, r, term)
	for _, name := range cppVars {
		if v := term.Get(name); v != "" {
			if st.Vars == nil {
				st.Vars = map[string]string{}
			}
			st.Vars[name] = v
		}
	}
	return st, ctx.Err()
}

// cppCommands are cppTools with the versioned gcc-N, as Homebrew's gcc-14,
// on the terminal's PATH put after gcc.
func cppCommands(env sysenv.Env) []string {
	var numbered []string
	for _, dir := range filepath.SplitList(env.Get("PATH")) {
		entries, _ := os.ReadDir(dir) // a missing directory has none
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".exe")
			if gccNumbered.MatchString(e.Name()) && !slices.Contains(numbered, name) {
				numbered = append(numbered, name)
			}
		}
	}
	slices.Sort(numbered)
	i := slices.Index(cppTools, "gcc") + 1
	return slices.Concat(cppTools[:i], numbered, cppTools[i:])
}

// versionLine is the first line the program at path prints for
// --version; "" when it fails.
func versionLine(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	return firstLine(versionOf(ctx, r, env, path))
}

// lineVersion is the version in a --version line: what follows "version",
// as clang's and cmake's, else the last one in it, as gcc's "gcc (Homebrew
// GCC 14.2.0_1) 14.2.0".
func lineVersion(line string) string {
	if m := cppVersioned.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	all := cppVersion.FindAllString(line, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

// compilerName is what a C compiler's command runs, by its --version line
// and its real file: "clang", "gcc", else the file's name.
func compilerName(line, real string) string {
	base := strings.TrimSuffix(filepath.Base(real), ".exe")
	switch {
	case strings.Contains(line, "clang"):
		return "clang"
	case strings.Contains(base, "gcc") || strings.Contains(line, "GCC"):
		return "gcc"
	}
	return base
}
