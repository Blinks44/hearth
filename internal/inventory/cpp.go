package inventory

// CppState is how C and C++ are set up: the compilers and build tools
// found, and the toolset the OS's way of building comes with.
type CppState struct {
	// Command is the C compiler's command whose copies are the tool's
	// Installations, as cc; Compiler is what it runs: "clang", "gcc".
	Command, Compiler string
	// Tools are the copies of the compilers and build tools, each Tool
	// the command it is run by: clang, gcc-14, cmake, ninja, make, ...
	Tools []Installation `json:",omitempty"`
	// AppleClang reports that the system's clang is Apple's; GCCIsClang
	// that the gcc command runs it too, as macOS's does.
	AppleClang bool `json:",omitempty"`
	GCCIsClang bool `json:",omitempty"`
	// Base is the OS's toolset: the Command Line Tools or Xcode on macOS,
	// build-essential on Linux, MSVC Build Tools on Windows; nil when
	// none is found.
	Base *CppBase `json:",omitempty"`
}

// CppBase is the toolset C and C++ builds rest on.
type CppBase struct {
	// ID is "clt", "xcode", "build-essential" or "msvc".
	ID            string
	Name, Version string
	Path          string `json:",omitempty"`
}
