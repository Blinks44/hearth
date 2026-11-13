package ops

// systemDirs hold the desktop's own programs, as GNOME's.
func systemDirs() []string {
	return []string{"/usr/libexec/", "/usr/sbin/", "/sbin/", "/lib/systemd/", "/usr/lib/systemd/"}
}
