//go:build !windows

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func colimaClientPathEdits(env sysenv.Env, brewBin, clientBin string) ([]Step, error) {
	// These strings go into shell code and a Homebrew service environment file.
	if strings.ContainsAny(clientBin+brewBin, "\"'`$\\\n\r:") {
		return nil, ErrNoFix
	}
	steps, err := startupEdits(env, `export PATH="`+clientBin+`:$PATH"`, `fish_add_path "`+clientBin+`"`)
	if err != nil {
		return nil, err
	}

	homebrew := env.Get("HOMEBREW_USER_CONFIG_HOME")
	if homebrew == "" {
		homebrew = filepath.Join(env.Get("HOME"), ".homebrew")
		if xdg := env.Get("XDG_CONFIG_HOME"); xdg != "" && !exists(homebrew) {
			homebrew = filepath.Join(xdg, "homebrew")
		}
	}
	path := filepath.Join(homebrew, "services", "colima.env")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read Colima's Homebrew service environment: %w", err)
	}
	before := string(data)
	servicePath := brewBin + ":" + filepath.Join(filepath.Dir(brewBin), "sbin") + ":/usr/bin:/bin:/usr/sbin:/sbin"
	for line := range strings.Lines(Block(before, "")) {
		if key, value, found := strings.Cut(line, "="); found && strings.TrimSpace(key) == "PATH" {
			servicePath = strings.TrimSpace(value)
		}
	}
	line := "PATH=" + clientBin + ":" + servicePath
	after := Block(before, line)
	if after != before {
		steps = append(steps, Step{Edit: &FileEdit{Path: path, Before: before, After: after, New: err != nil}})
	}
	return steps, nil
}
