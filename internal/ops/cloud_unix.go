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

func awsProfileSteps(profile string, env sysenv.Env) ([]Step, []Note, error) {
	shell := filepath.Base(env.Get("SHELL"))
	rc := rcFile(env, shell)
	if rc == "" {
		return nil, nil, errors.New(locales.SecretShellUnsupported(shell))
	}
	data, err := os.ReadFile(rc)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read %s: %w", rc, err)
	}
	text := string(data)
	after := replaceAWSProfileInBlock(text, shell, profile)
	var notes []Note
	if hasAWSProfileOutsideBlock(text) {
		notes = append(notes, Note{
			Title: locales.CloudAwsOutsideBlockTitle(),
			Text:  locales.CloudAwsOutsideBlockText(tildeIn(rc, env)),
			Tag:   "AWS_PROFILE",
		})
	}
	edit := &FileEdit{
		Path:   rc,
		Before: text,
		After:  after,
		New:    errors.Is(err, os.ErrNotExist),
	}
	return []Step{{Edit: edit}}, notes, nil
}

func replaceAWSProfileInBlock(text, shell, profile string) string {
	all := splitLines(text)
	var body []string
	if start, end := blockLines(all); start >= 0 {
		body = slices.Clone(all[start+1 : end])
	}
	body = slices.DeleteFunc(body, isAWSProfileLine)
	if profile != "" && profile != "default" {
		if shell == "fish" {
			body = append(body, "set -gx AWS_PROFILE "+fishQuote(profile))
		} else {
			body = append(body, "export AWS_PROFILE="+shQuote(profile))
		}
	}
	return Block(text, strings.Join(body, "\n"))
}

func isAWSProfileLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "export AWS_PROFILE=") ||
		strings.HasPrefix(t, "AWS_PROFILE=") ||
		strings.HasPrefix(t, "set -gx AWS_PROFILE ") ||
		strings.HasPrefix(t, "set -x AWS_PROFILE ")
}

func hasAWSProfileOutsideBlock(text string) bool {
	all := splitLines(text)
	start, end := blockLines(all)
	for i, line := range all {
		if start >= 0 && i >= start && i <= end {
			continue
		}
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if isAWSProfileLine(t) {
			return true
		}
	}
	return false
}
