//go:build !windows

package run

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestTerminalLauncher(t *testing.T) {
	script := "/tmp/a b.command"
	if runtime.GOOS == "darwin" {
		for app, want := range map[string][]string{
			"":        {"/usr/bin/open", "-a", "Terminal", script},
			"Warp":    {"/usr/bin/open", "-a", "Warp", script},
			"Ghostty": {"/usr/bin/open", "-na", "Ghostty", "--args", "-e", script},
			"unknown": {"/usr/bin/open", "-a", "Terminal", script},
		} {
			if got := terminalLauncher(app, script); !slices.Equal(got, want) {
				t.Errorf("launcher(%q) = %q, want %q", app, got, want)
			}
		}
	}
}

func TestTerminalScript(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "terminal-*.command")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"/bin/sh", "-c", `printf '%s\n' "$KUBECONFIG" "$1"; exit 19`, "sh", `x'; printf altered; #'`}
	if err := writeTerminalScript(f, args, "/usr/bin:/bin", "config with spaces", "Press Enter"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "/bin/sh", f.Name())
	cmd.Stdin = strings.NewReader("\n")
	got, err := cmd.CombinedOutput()
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exit.ExitCode() != 19 || string(got) != "config with spaces\nx'; printf altered; #'\nPress Enter\n" {
		t.Fatalf("script = %q, %v", got, err)
	}
	if _, err := os.Stat(f.Name()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("temporary script remains: %v", err)
	}
}
