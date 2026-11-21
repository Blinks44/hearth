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

// EditorPathPlan returns the plan that puts the directory of e.CommandSource
// on the terminal's PATH through Hearth's managed block in the user's shell
// startup file.
func EditorPathPlan(e inventory.Editor, env sysenv.Env, terminal Terminal) (Plan, error) {
	if e.Command == "" || e.CommandSource == "" || !filepath.IsAbs(e.CommandSource) {
		return Plan{}, ErrNoFix
	}
	dir := filepath.Dir(e.CommandSource)
	if strings.ContainsAny(dir, "\"$`\\\n") {
		return Plan{}, ErrNoFix
	}
	home := env.Get("HOME")
	named := dir
	if home != "" && strings.HasPrefix(dir, home+"/") {
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
	step, err := firstLineStep(file, line, true)
	if err != nil {
		return Plan{}, err
	}
	steps := []Step{step}
	if login := bashLogin(home); shell == "bash" && login != "" {
		step, err := firstLineStep(login, line, false)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, step)
	}
	return editorPathPlan(steps, locales.EditorsAddPathWhy(dir, e.Command), e.Command, dir, terminal), nil
}
