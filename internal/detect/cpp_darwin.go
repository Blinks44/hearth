package detect

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const ccCommand = "cc"

// exeName is the file of the command name.
func exeName(name string) string { return name }

// cppStub reports macOS's /usr/bin/cc, clang, make and the rest when the
// Command Line Tools are not installed: they then offer to install them
// instead of running.
func cppStub(path string) bool {
	if !strings.HasPrefix(path, "/usr/bin/") {
		return false
	}
	_, err := filepath.EvalSymlinks(xcodeSelectLink)
	return err != nil
}

// cppBase is the developer directory xcode-select chose: the Command Line
// Tools, with their version from their package's receipt, or Xcode.
func cppBase(ctx context.Context, r run.Runner, env sysenv.Env) *inventory.CppBase {
	dir, err := filepath.EvalSymlinks(xcodeSelectLink)
	if err != nil {
		return nil
	}
	if strings.Contains(dir, ".app/") {
		return &inventory.CppBase{ID: "xcode", Name: "Xcode", Path: dir}
	}
	b := &inventory.CppBase{ID: "clt", Name: "Command Line Tools", Path: dir}
	// version: 16.2.0.0.1.1733547573
	res, err := r.Run(ctx, run.Cmd{Args: []string{"pkgutil", "--pkg-info=com.apple.pkg.CLTools_Executables"}, Env: env})
	if err == nil && res.ExitCode == 0 {
		for line := range strings.Lines(string(res.Stdout)) {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "version: "); ok {
				b.Version = cppVersion.FindString(v)
			}
		}
	}
	return b
}
