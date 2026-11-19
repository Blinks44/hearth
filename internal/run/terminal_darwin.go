package run

func terminalLauncher(script string) []string {
	return []string{"/usr/bin/open", "-a", "Terminal", script}
}
