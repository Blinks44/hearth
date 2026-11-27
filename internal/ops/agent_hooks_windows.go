//go:build windows

package ops

// quoteHookExe safely quotes an executable path for a Windows hook command.
func quoteHookExe(exe string) string {
	return windowsArg(exe)
}
