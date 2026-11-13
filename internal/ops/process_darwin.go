package ops

// systemDirs hold macOS' own programs, as ControlCenter, which keeps
// AirPlay's ports 5000 and 7000.
func systemDirs() []string {
	return []string{"/System/", "/usr/libexec/", "/usr/sbin/", "/sbin/", "/Library/Apple/"}
}
