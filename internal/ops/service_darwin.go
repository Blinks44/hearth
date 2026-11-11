package ops

// systemService: macOS' own services are not databases'; Homebrew keeps
// those.
func systemService(ServiceAction, string) (Step, bool) { return Step{}, false }
