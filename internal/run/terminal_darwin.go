package run

import (
	"os"
	"path/filepath"
	"strings"
)

// macTerminals are the terminal apps Hearth opens a command in, Apple's
// first: each takes the script as its maker documents. Terminal, iTerm
// and Warp run a .command file they are asked to open; the others take
// the command on their command line.
var macTerminals = []struct {
	name string
	args func(script string) []string
}{
	{"Terminal", func(s string) []string { return []string{"/usr/bin/open", "-a", "Terminal", s} }},
	{"iTerm", func(s string) []string { return []string{"/usr/bin/open", "-a", "iTerm", s} }},
	{"Warp", func(s string) []string { return []string{"/usr/bin/open", "-a", "Warp", s} }},
	{"Ghostty", func(s string) []string { return []string{"/usr/bin/open", "-na", "Ghostty", "--args", "-e", s} }},
	{"kitty", func(s string) []string { return []string{"/usr/bin/open", "-na", "kitty", "--args", s} }},
	{"WezTerm", func(s string) []string {
		return []string{"/usr/bin/open", "-na", "WezTerm", "--args", "start", "--", s}
	}},
	{"Alacritty", func(s string) []string { return []string{"/usr/bin/open", "-na", "Alacritty", "--args", "-e", s} }},
}

// Terminals are the terminal apps installed that Hearth can open a
// command in, the system's first.
func Terminals() []string {
	home, _ := os.UserHomeDir()
	var names []string
	for _, t := range macTerminals {
		for _, dir := range []string{"/Applications", "/System/Applications/Utilities", filepath.Join(home, "Applications")} {
			if _, err := os.Stat(filepath.Join(dir, t.name+".app")); err == nil {
				names = append(names, t.name)
				break
			}
		}
	}
	return names
}

// terminalLauncher opens script in the terminal app of that name or custom
// path, falling back to Terminal for "" or an unknown name.
func terminalLauncher(app, script string) []string {
	for _, t := range macTerminals {
		if t.name == app {
			return t.args(script)
		}
	}
	trimmed := strings.TrimRight(strings.TrimSpace(app), "/")
	if trimmed != "" && (filepath.IsAbs(trimmed) || strings.Contains(trimmed, "/")) {
		if strings.HasSuffix(trimmed, ".app") {
			base := strings.ToLower(strings.TrimSuffix(filepath.Base(trimmed), ".app"))
			switch base {
			case "ghostty", "alacritty":
				return []string{"/usr/bin/open", "-na", trimmed, "--args", "-e", script}
			case "kitty":
				return []string{"/usr/bin/open", "-na", trimmed, "--args", script}
			case "wezterm":
				return []string{"/usr/bin/open", "-na", trimmed, "--args", "start", "--", script}
			default:
				return []string{"/usr/bin/open", "-a", trimmed, script}
			}
		}
		switch strings.ToLower(filepath.Base(trimmed)) {
		case "wezterm", "wezterm-gui":
			return []string{trimmed, "start", "--", "/bin/sh", script}
		case "kitty":
			return []string{trimmed, "/bin/sh", script}
		default:
			return []string{trimmed, "-e", "/bin/sh", script}
		}
	}
	return macTerminals[0].args(script)
}
