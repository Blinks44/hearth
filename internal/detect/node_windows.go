package detect

import "path/filepath"

const (
	nodeExe = "node.exe"
	// nodeBin is where node lies in an installation's directory.
	nodeBin = "node.exe"
)

// nodeModules is the global node_modules of the node at path:
// <prefix>\node.exe keeps them in <prefix>\node_modules.
func nodeModules(path string) string {
	return filepath.Join(filepath.Dir(path), "node_modules")
}

// prefixModules and prefixBin are where npm -g with prefix puts packages
// and their commands: prefix\node_modules, and the .cmd files in prefix.
func prefixModules(prefix string) string { return filepath.Join(prefix, "node_modules") }
func prefixBin(prefix string) string     { return prefix }
