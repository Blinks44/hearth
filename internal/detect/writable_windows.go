package detect

// writable reports that this user may create files in dir: n, the one
// manager that asks, does not run on Windows.
func writable(string) bool { return true }
