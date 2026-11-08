package ops

// quoteArg quotes a for a Windows command line: its paths keep their
// backslashes.
func quoteArg(a string) string { return windowsArg(a) }
