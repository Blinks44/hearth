package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// The Command Line Tools with their receipt's version; /usr/bin's tools
// are stubs without them.
func TestCppBaseDarwin(t *testing.T) {
	dir := root(t)
	link := filepath.Join(dir, "xcode_select_link")
	origLink, origCLT := xcodeSelectLink, cltDir
	xcodeSelectLink, cltDir = link, filepath.Join(dir, "none")
	t.Cleanup(func() { xcodeSelectLink, cltDir = origLink, origCLT })
	r := &run.Fake{}
	if cppBase(t.Context(), r, sysenv.Env{}) != nil || !cppStub("/usr/bin/clang") {
		t.Error("without the Command Line Tools: a base, or clang is no stub")
	}
	clt := filepath.Join(dir, "CommandLineTools")
	writeFile(t, filepath.Join(clt, "usr", "bin", "clang"), "")
	symlink(t, clt, link)
	r.Stdout("package-id: com.apple.pkg.CLTools_Executables\nversion: 16.2.0.0.1.1733547573\nvolume: /\n", "pkgutil", "--pkg-info=com.apple.pkg.CLTools_Executables")
	got := cppBase(t.Context(), r, sysenv.Env{})
	if want := (inventory.CppBase{ID: "clt", Name: "Command Line Tools", Version: "16.2.0", Path: clt}); got == nil || *got != want {
		t.Errorf("base %+v, want %+v", got, want)
	}
	if cppStub("/usr/bin/clang") || cppStub("/opt/homebrew/bin/gcc-14") {
		t.Error("a stub with the Command Line Tools")
	}
}
