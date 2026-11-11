//go:build !windows

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathFirstable reports that Hearth may put the i-th entry of the
// terminal's PATH first: a line in the user's startup file can name it.
func PathFirstable(entries []inventory.PathEntry, i int, _ string) bool {
	return firstable(entries, i) && !strings.ContainsAny(entries[i].Dir, "\"$`\\\n")
}

// PathFirstPlan returns the plan that puts dir, an entry of entries, the
// terminal's PATH as the scan found it, first in it: a line in Hearth's
// block at the end of the interactive shell's startup file puts it before
// the rest, which stays as it is. env is the terminal's; terminal checks
// the result.
func PathFirstPlan(entries []inventory.PathEntry, dir string, env sysenv.Env, terminal Terminal) (Plan, error) {
	i := firstIndex(entries, dir)
	if i < 0 || !PathFirstable(entries, i, "") {
		return Plan{}, ErrNoFix
	}
	home := env.Get("HOME")
	named := dir
	if strings.HasPrefix(dir, home+"/") {
		named = "$HOME" + strings.TrimPrefix(dir, home)
	}
	shell := filepath.Base(env.Get("SHELL"))
	file := rcFile(env, shell)
	line := `export PATH="` + named + `:$PATH"`
	switch shell {
	case "zsh", "bash":
	case "fish":
		line = `set -gx PATH "` + named + `" $PATH`
	default:
		return Plan{}, errors.New(locales.MiseShellUnsupported(shell))
	}
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Plan{}, fmt.Errorf("read %s: %w", file, err)
	}
	// A line put there before goes to the end of the block, after those
	// that put other directories first since.
	after := AddToBlock(withoutBlockLine(string(data), line), line)
	step := Step{Edit: &FileEdit{Path: file, Before: string(data), After: after, New: err != nil}}
	return pathFirstPlan(step, locales.PathFirstWhy(), entries, dir, 0, terminal), nil
}

// rcFile is the startup file of shell's interactive sessions, as zsh's
// .zshrc, for the user of env; "" for a shell Hearth does not know.
func rcFile(env sysenv.Env, shell string) string {
	home := env.Get("HOME")
	switch shell {
	case "zsh":
		dir := env.Get("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "fish":
		config := env.Get("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		return filepath.Join(config, "fish", "config.fish")
	}
	return ""
}

// withoutBlockLine is text without line in Hearth's block.
func withoutBlockLine(text, line string) string {
	all := splitLines(text)
	start, end := blockLines(all)
	if start < 0 {
		return text
	}
	var body []string
	for _, l := range all[start+1 : end] {
		if l != line {
			body = append(body, l)
		}
	}
	return Block(text, strings.Join(body, "\n"))
}
