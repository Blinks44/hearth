package run

import (
	"errors"
	"os/exec"
	"slices"
	"testing"
)

func TestLinuxTerminalLauncher(t *testing.T) {
	found := map[string]bool{"/usr/bin/konsole": true, "/usr/bin/kitty": true, "/usr/bin/rio": true}
	lookPath = func(name string) (string, error) {
		if found["/usr/bin/"+name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = exec.LookPath })
	script := "/tmp/a b.command"
	for app, want := range map[string][]string{
		"":               {"/usr/bin/konsole", "-e", "/bin/sh", script},
		"kitty":          {"/usr/bin/kitty", "/bin/sh", script},
		"rio":            {"/usr/bin/rio", "-e", "/bin/sh", script},
		"gnome-terminal": {"/usr/bin/konsole", "-e", "/bin/sh", script},
	} {
		if got := terminalLauncher(app, script); !slices.Equal(got, want) {
			t.Errorf("launcher(%q) = %q, want %q", app, got, want)
		}
	}
	if got := Terminals(); !slices.Equal(got, []string{"konsole", "kitty"}) {
		t.Errorf("terminals = %q", got)
	}
}
