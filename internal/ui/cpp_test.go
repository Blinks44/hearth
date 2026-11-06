package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// The page shows the compilers and build systems, cc running Apple clang
// and macOS's gcc being clang beside Homebrew's gcc-14, the Command Line
// Tools with make, ccache's cache in Cleanup, and the build variables.
func TestCppPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	system := inventory.Source{Kind: inventory.KindSystem, Root: "/usr"}
	brew := func(name, version string) inventory.Source {
		return inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/" + name + "/" + version}
	}
	tool := func(name, version, path string, src inventory.Source, rank inventory.PathRank) inventory.Installation {
		return inventory.Installation{Tool: name, Version: version, Path: path, RealPath: path, Source: src, OnPath: rank, InGUI: rank}
	}
	cache := home + "/Library/Caches/ccache"
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{
		{Tool: "cpp", Installations: []inventory.Installation{tool("cpp", "17.0.0", "/usr/bin/cc", system, 1)},
			Vars: map[string]string{"CMAKE_C_COMPILER_LAUNCHER": "ccache"},
			Cpp: &inventory.CppState{Command: "cc", Compiler: "clang", AppleClang: true, GCCIsClang: true,
				Base: &inventory.CppBase{ID: "clt", Name: "Command Line Tools", Version: "16.2.0", Path: "/Library/Developer/CommandLineTools"},
				Tools: []inventory.Installation{
					tool("clang", "17.0.0", "/usr/bin/clang", system, 1),
					tool("gcc-14", "14.2.0", "/opt/homebrew/bin/gcc-14", brew("gcc", "14.2.0"), 2),
					tool("cmake", "3.31.2", "/opt/homebrew/bin/cmake", brew("cmake", "3.31.2"), 2),
					tool("make", "3.81", "/usr/bin/make", system, 1),
					tool("ccache", "4.10.2", "/opt/homebrew/bin/ccache", brew("ccache", "4.10.2"), 2),
				}}},
	}}
	a.Sizes = &inventory.Sizes{Taken: time.Now(), Caches: []inventory.Cache{{ID: "ccache", Path: cache}},
		Paths: map[string]inventory.Usage{cache: {Bytes: 1_200_000_000}}}
	a.router.Push(cppPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "cpp")
	hasTexts(t, tt, "Languages & runtimes · compilers and build", "Command Line Tools 16.2.0 · brew",
		"clang 17", "cmake 3.31", "ccache 4.10",
		"Apple clang", "GCC", "CMake", "command gcc-14", "in PATH",
		"How the terminal finds cc", "17.0.0",
		"On macOS the gcc command runs clang. The real GCC 14 is gcc-14: projects that need GCC set CC=gcc-14.",
		"Toolset", "Command Line Tools 16.2.0", "make 3.81",
		"Cache 1.2 GB", "~/Library/Caches/ccache · in Cleanup", "Connected to CMake", "CMAKE_C_COMPILER_LAUNCHER=ccache",
		"Variables", "not set · cc → clang", "not set · SDK of Command Line Tools")
}

// Without a copy or a toolset the page says C/C++ is not installed.
func TestCppPageEmpty(t *testing.T) {
	a := NewApp()
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "cpp", Cpp: &inventory.CppState{Command: "cc"}}}}
	a.router.Push(cppPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if !tt.HasText("C/C++ is not installed on this computer") {
		t.Errorf("texts %q", tt.Texts())
	}
}
