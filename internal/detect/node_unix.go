//go:build !windows

package detect

import "path/filepath"

const (
	nodeExe = "node"
	// nodeBin is where node lies in an installation's directory.
	nodeBin = "bin/node"
)

// nodeModules is the global node_modules of the node at path:
// <prefix>/bin/node keeps them in <prefix>/lib/node_modules.
func nodeModules(path string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(path)), "lib", "node_modules")
}
