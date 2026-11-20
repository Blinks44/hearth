package run

import "os/exec"

// linuxTerminals are the terminal emulators Hearth opens a command in, in
// the order it looks for one: the desktop's chosen one first, as Debian
// and Ubuntu name it, then the desktops' own and the popular others.
var linuxTerminals = []struct {
	name string
	args func(path, script string) []string
}{
	{"x-terminal-emulator", func(p, s string) []string { return []string{p, "-e", "/bin/sh", s} }},
	{"gnome-terminal", func(p, s string) []string { return []string{p, "--", "/bin/sh", s} }},
	{"konsole", func(p, s string) []string { return []string{p, "-e", "/bin/sh", s} }},
	{"xfce4-terminal", func(p, s string) []string { return []string{p, "-x", "/bin/sh", s} }},
	{"kitty", func(p, s string) []string { return []string{p, "/bin/sh", s} }},
	{"alacritty", func(p, s string) []string { return []string{p, "-e", "/bin/sh", s} }},
	{"wezterm", func(p, s string) []string { return []string{p, "start", "--", "/bin/sh", s} }},
	{"ghostty", func(p, s string) []string { return []string{p, "-e", "/bin/sh", s} }},
	{"foot", func(p, s string) []string { return []string{p, "/bin/sh", s} }},
	{"xterm", func(p, s string) []string { return []string{p, "-e", "/bin/sh", s} }},
}

// lookPath finds a terminal's executable; tests replace it.
var lookPath = exec.LookPath

// Terminals are the terminal emulators found on the PATH that Hearth can
// open a command in.
func Terminals() []string {
	var names []string
	for _, t := range linuxTerminals {
		if _, err := lookPath(t.name); err == nil {
			names = append(names, t.name)
		}
	}
	return names
}

// terminalLauncher opens script in the terminal of that name, or, for ""
// or one not found, in the first one found; GNOME's when none is.
func terminalLauncher(app, script string) []string {
	for _, t := range linuxTerminals {
		if t.name == app {
			if p, err := lookPath(t.name); err == nil {
				return t.args(p, script)
			}
		}
	}
	for _, t := range linuxTerminals {
		if p, err := lookPath(t.name); err == nil {
			return t.args(p, script)
		}
	}
	return []string{"/usr/bin/gnome-terminal", "--", "/bin/sh", script}
}
