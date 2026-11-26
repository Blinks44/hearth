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
		orig := lookPath
		lookPath = func(name string) (string, error) {
			if name == "rio" || name == "alacritty" {
				return "/opt/homebrew/bin/" + name, nil
			}
			return "", errors.New("not found")
		}
		t.Cleanup(func() { lookPath = orig })
		for app, want := range map[string][]string{
			"":                            {"/usr/bin/open", "-a", "Terminal", script},
			"Warp":                        {"/usr/bin/open", "-a", "Warp", script},
			"Ghostty":                     {"/usr/bin/open", "-na", "Ghostty", "--args", "-e", script},
			"unknown":                     {"/usr/bin/open", "-a", "Terminal", script},
			"rio":                         {"/opt/homebrew/bin/rio", "-e", "/bin/sh", script},
			"alacritty":                   {"/opt/homebrew/bin/alacritty", "-e", "/bin/sh", script},
			"/Applications/Ghostty.app/":  {"/usr/bin/open", "-na", "/Applications/Ghostty.app", "--args", "-e", script},
			"/Applications/WezTerm.app":   {"/usr/bin/open", "-na", "/Applications/WezTerm.app", "--args", "start", "--", script},
			"/Applications/MyTerm.app":    {"/usr/bin/open", "-a", "/Applications/MyTerm.app", script},
			"/opt/homebrew/bin/wezterm":   {"/opt/homebrew/bin/wezterm", "start", "--", "/bin/sh", script},
			"/opt/homebrew/bin/alacritty": {"/opt/homebrew/bin/alacritty", "-e", "/bin/sh", script},
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
	if err := writeTerminalScript(f, "", args, "/usr/bin:/bin", "config with spaces", "Press Enter"); err != nil {
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

func TestTerminalScriptInDir(t *testing.T) {
	workDir := t.TempDir()
	f, err := os.CreateTemp(t.TempDir(), "terminal-dir-*.command")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTerminalScript(f, workDir, []string{"/bin/pwd"}, "/usr/bin:/bin", "", ""); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "/bin/sh", f.Name())
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v (%q)", err, got)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(got)), strings.TrimPrefix(workDir, "/private")) {
		t.Fatalf("pwd = %q, want suffix %q", strings.TrimSpace(string(got)), workDir)
	}
}
