//go:build unix

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func setEnvVarSteps(name, value string, env sysenv.Env) ([]Step, error) {
	shell := filepath.Base(env.Get("SHELL"))
	rc := rcFile(env, shell)
	if rc == "" {
		return nil, errors.New(locales.PathShellUnsupported(shell))
	}
	var prefix, line string
	if shell == "fish" {
		prefix = "set -gx " + name + " "
		line = prefix + fishQuote(value)
	} else {
		prefix = "export " + name + "="
		line = prefix + shQuote(value)
	}
	step, err := envVarFileStep(rc, prefix, line, true)
	if err != nil {
		return nil, err
	}
	steps := []Step{step}
	if login := bashLogin(env.Get("HOME")); shell == "bash" && login != "" && login != rc {
		loginStep, err := envVarFileStep(login, prefix, line, false)
		if err != nil {
			return nil, err
		}
		steps = append(steps, loginStep)
	}
	return steps, nil
}

func envVarFileStep(path, prefix, line string, allowNew bool) (Step, error) {
	data, err := os.ReadFile(path)
	if err != nil && (!allowNew || !errors.Is(err, os.ErrNotExist)) {
		return Step{}, fmt.Errorf("read %s: %w", path, err)
	}
	before := string(data)
	after := upsertBlockLine(before, prefix, line)
	return Step{Edit: &FileEdit{Path: path, Before: before, After: after, New: err != nil}}, nil
}

// upsertBlockLine replaces an existing line with prefix inside Hearth's
// managed block, or appends line to the block if none exists.
func upsertBlockLine(text, prefix, line string) string {
	all := splitLines(text)
	var body []string
	if start, end := blockLines(all); start >= 0 {
		body = slices.Clone(all[start+1 : end])
	}
	replaced := false
	for i, existing := range body {
		if strings.HasPrefix(strings.TrimSpace(existing), prefix) {
			body[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		body = append(body, line)
	}
	return Block(text, strings.Join(body, "\n"))
}
