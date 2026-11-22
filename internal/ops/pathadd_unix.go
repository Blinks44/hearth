//go:build !windows

package ops

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PathAddPlan returns the plan that adds raw to the terminal's PATH through
// Hearth's managed block in the user's shell startup file.
func PathAddPlan(entries []inventory.PathEntry, raw string, env sysenv.Env, terminal Terminal) (Plan, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" || strings.ContainsAny(dir, ":\"$`\\\r\n") {
		return Plan{}, errors.New(locales.PathAddInvalid())
	}
	home := env.Get("HOME")
	if home != "" {
		switch {
		case dir == "~":
			dir = home
		case strings.HasPrefix(dir, "~/"):
			dir = filepath.Join(home, dir[2:])
		}
	}
	if !filepath.IsAbs(dir) {
		return Plan{}, errors.New(locales.PathAddInvalid())
	}
	dir = filepath.Clean(dir)
	notes, err := validatePathAdd(entries, dir)
	if err != nil {
		return Plan{}, err
	}
	named := dir
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
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
		return Plan{}, errors.New(locales.SecretShellUnsupported(shell))
	}
	step, err := firstLineStep(file, line, true)
	if err != nil {
		return Plan{}, err
	}
	steps := []Step{step}
	if login := bashLogin(home); shell == "bash" && login != "" && !bashLoginSourcesRC(home, login) {
		step, err := firstLineStep(login, line, false)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, step)
	}
	return pathAddPlan(steps, locales.PathAddWhy(), dir, notes, terminal), nil
}

func bashLoginSourcesRC(home, loginPath string) bool {
	if fileMentions(loginPath, ".bashrc") {
		return true
	}
	if home != "" && filepath.Base(loginPath) != ".profile" && fileMentions(loginPath, ".profile") {
		return fileMentions(filepath.Join(home, ".profile"), ".bashrc")
	}
	return false
}

func fileMentions(path, needle string) bool {
	text, isNew, err := readText(path)
	if err != nil || isNew {
		return false
	}
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, needle) {
			return true
		}
	}
	return false
}
