package ops

// systemService: macOS' own services are not databases'; Homebrew keeps
// those.
func systemService(ServiceAction, string) ([]Step, bool) { return nil, false }

// systemAutostart: Homebrew keeps the databases' services on macOS.
func systemAutostart(bool, string) (Step, bool) { return Step{}, false }
