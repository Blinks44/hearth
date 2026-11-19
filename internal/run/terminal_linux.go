package run

func terminalLauncher(script string) []string {
	return []string{"/usr/bin/gnome-terminal", "--", "/bin/sh", script}
}
