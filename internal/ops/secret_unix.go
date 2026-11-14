//go:build unix

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// userVars are the steps that export vars in Hearth's block of the startup
// file of the user's shell, with that file, as ~/.zshrc: terminals opened
// after, and the agents started there, get them.
func userVars(vars []secretVar, env sysenv.Env) ([]Step, string, error) {
	shell := filepath.Base(env.Get("SHELL"))
	rc := rcFile(env, shell)
	if rc == "" {
		return nil, "", errors.New(locales.SecretShellUnsupported(shell))
	}
	where := tildeIn(rc, env)
	if len(vars) == 0 {
		return nil, where, nil
	}
	data, err := os.ReadFile(rc)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("read %s: %w", rc, err)
	}
	var lines, hide []string
	for _, v := range vars {
		if shell == "fish" {
			lines = append(lines, "set -gx "+v.Name+" "+fishQuote(v.Value))
		} else {
			lines = append(lines, "export "+v.Name+"="+shQuote(v.Value))
		}
		hide = append(hide, v.Value)
	}
	text := string(data)
	edit := &FileEdit{Path: rc, Before: text, After: AddToBlock(text, lines...), New: err != nil, Hide: hide, DropCopy: true}
	return []Step{{Edit: edit}}, where, nil
}

// shQuote is v in single quotes for a POSIX shell: nothing in it expands.
func shQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

// fishQuote is v in single quotes for fish.
func fishQuote(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(v) + "'"
}
