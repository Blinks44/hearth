//go:build !windows

package detect

import "github.com/Blinks44/hearth/internal/sysenv"

// goExe is go's executable in GOROOT/bin.
const goExe = "go"

// goInstallDirs are where Go's official installer puts it; a variable,
// for tests.
var goInstallDirs = func(sysenv.Env) []string { return []string{"/usr/local/go"} }
