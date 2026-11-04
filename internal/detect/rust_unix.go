//go:build !windows

package detect

// rustcExe and rustupExe are rustc's and rustup's executables.
const (
	rustcExe  = "rustc"
	rustupExe = "rustup"
)
