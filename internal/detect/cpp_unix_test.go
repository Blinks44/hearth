//go:build !windows

package detect

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// cc's copies and what it runs; the compilers and build tools each once,
// a gcc that is clang left out but noted, the versioned gcc-N after gcc;
// the build variables the terminal sets.
func TestCpp(t *testing.T) {
	dir := root(t)
	sys := filepath.Join(dir, "usr", "bin")
	brew := filepath.Join(dir, "brew", "bin")
	cc := fakeExe(t, filepath.Join(sys, "cc"))
	clang := fakeExe(t, filepath.Join(sys, "clang"))
	gcc := fakeExe(t, filepath.Join(sys, "gcc"))
	mk := fakeExe(t, filepath.Join(sys, "make"))
	gcc14 := fakeExe(t, filepath.Join(brew, "gcc-14"))
	fakeExe(t, filepath.Join(brew, "gcc-ar-14"))
	cmake := fakeExe(t, filepath.Join(brew, "cmake"))
	r := &run.Fake{}
	r.Stdout("Apple clang version 17.0.0 (clang-1700.0.13.3)\nTarget: arm64-apple-darwin24.4.0\n", cc, "--version")
	r.Stdout("Apple clang version 17.0.0 (clang-1700.0.13.3)\n", clang, "--version")
	r.Stdout("Apple clang version 17.0.0 (clang-1700.0.13.3)\n", gcc, "--version")
	r.Stdout("gcc-14 (Homebrew GCC 14.2.0_1) 14.2.0\nCopyright (C) 2024 Free Software Foundation, Inc.\n", gcc14, "--version")
	r.Stdout("cmake version 3.31.2\n\nCMake suite maintained and supported by Kitware (kitware.com/cmake).\n", cmake, "--version")
	r.Stdout("GNU Make 3.81\n", mk, "--version")
	env := sysenv.Env{"HOME=" + dir, "PATH=" + sys + string(filepath.ListSeparator) + brew, "CC=gcc-14", "CMAKE_C_COMPILER_LAUNCHER=ccache"}
	st, err := Cpp(t.Context(), r, env, sysenv.Env{"PATH=" + sys})
	if err != nil {
		t.Fatal(err)
	}
	if ins := copiesIn(st.Installations, dir); len(ins) != 1 || ins[0].Version != "17.0.0" || ins[0].OnPath != 1 || ins[0].InGUI != 1 {
		t.Fatalf("cc: %+v", st.Installations)
	}
	s := st.Cpp
	if s.Command != "cc" || s.Compiler != "clang" || !s.AppleClang || !s.GCCIsClang {
		t.Errorf("state: %+v", s)
	}
	var got []string
	for _, in := range copiesIn(s.Tools, dir) {
		got = append(got, in.Tool+" "+in.Version)
	}
	if want := []string{"clang 17.0.0", "gcc-14 14.2.0", "cmake 3.31.2", "make 3.81"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools %q, want %q", got, want)
	}
	if want := map[string]string{"CC": "gcc-14", "CMAKE_C_COMPILER_LAUNCHER": "ccache"}; !reflect.DeepEqual(st.Vars, want) {
		t.Errorf("vars %v", st.Vars)
	}
}

func TestLineVersion(t *testing.T) {
	for line, want := range map[string]string{
		"Apple clang version 17.0.0 (clang-1700.0.13.3)": "17.0.0",
		"Ubuntu clang version 18.1.3 (1ubuntu1)":         "18.1.3",
		"gcc (Ubuntu 13.3.0-6ubuntu2~24.04) 13.3.0":      "13.3.0",
		"gcc.exe (Rev2, Built by MSYS2 project) 14.2.0":  "14.2.0",
		"cc (GCC) 14.2.1 20240912 (Red Hat 14.2.1-3)":    "14.2.1",
		"ccache version 4.10.2":                          "4.10.2",
		"1.12.1":                                         "1.12.1",
		"GNU Make 4.3":                                   "4.3",
		"":                                               "",
	} {
		if got := lineVersion(line); got != want {
			t.Errorf("lineVersion(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestCompilerName(t *testing.T) {
	for _, c := range []struct{ line, real, want string }{
		{"Apple clang version 17.0.0", "/usr/bin/cc", "clang"},
		{"cc (Ubuntu 13.3.0-6ubuntu2~24.04) 13.3.0", "/usr/bin/x86_64-linux-gnu-gcc-13", "gcc"},
		{"cc (GCC) 14.2.1", "/usr/bin/cc", "gcc"},
		{"tcc version 0.9.27", "/usr/bin/tcc", "tcc"},
	} {
		if got := compilerName(c.line, c.real); got != c.want {
			t.Errorf("compilerName(%q, %q) = %q, want %q", c.line, c.real, got, c.want)
		}
	}
}
