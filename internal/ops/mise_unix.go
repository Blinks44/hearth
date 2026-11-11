//go:build !windows

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

// SetUpMise returns what sets mise up for the user of env, the terminal's
// environment: installing it as installMise does where it is not found,
// then hooking it into the shell's startup files.
func SetUpMise(env sysenv.Env) (MiseSetup, error) {
	s := MiseSetup{Mise: findMise(env)}
	if s.Mise == "" {
		var err error
		if s, err = installMise(env); err != nil {
			return MiseSetup{}, err
		}
	}
	hook, err := hookMise(env, s.Mise)
	if err != nil {
		return MiseSetup{}, err
	}
	s.Steps = append(s.Steps, hook...)
	return s, nil
}

// findMise is where mise is: on the terminal's PATH, or where its own
// installer and Homebrew put it, which a startup file may not have put on
// PATH yet; "" when nowhere.
func findMise(env sysenv.Env) string {
	if found := sysenv.LookPathAll("mise", env); len(found) > 0 {
		return found[0]
	}
	places := []string{filepath.Join(env.Get("HOME"), ".local", "bin", "mise")}
	for _, prefix := range brewPrefixes {
		places = append(places, filepath.Join(prefix, "bin", "mise"))
	}
	for _, p := range places {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// brewPrefixes are where Homebrew lives: Apple silicon's, Intel's and
// Linux's; tests change them.
var brewPrefixes = []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}

// hookMise returns the steps that hook mise, at path mise, into the
// user's shell as mise's documentation says: activated in the file of
// interactive shells, which switches versions as the terminal enters a
// folder, and its shims in the login shell's file, for what starts
// without a prompt, as editors' tasks. Each line goes into Hearth's
// block; none when a startup file activates mise already.
func hookMise(env sysenv.Env, mise string) ([]Step, error) {
	home := env.Get("HOME")
	if strings.HasPrefix(mise, home+"/") {
		mise = "$HOME" + strings.TrimPrefix(mise, home)
	}
	type line struct{ file, text string }
	var lines []line
	switch shell := filepath.Base(env.Get("SHELL")); shell {
	case "zsh":
		rc := rcFile(env, shell)
		lines = []line{
			{rc, `eval "$(` + mise + ` activate zsh)"`},
			{filepath.Join(filepath.Dir(rc), ".zprofile"), `eval "$(` + mise + ` activate zsh --shims)"`},
		}
	case "bash":
		// The shims go to .profile, which Linux desktop sessions read as
		// the user signs in, so that apps see them too.
		lines = []line{
			{rcFile(env, shell), `eval "$(` + mise + ` activate bash)"`},
			{filepath.Join(home, ".profile"), `eval "$(` + mise + ` activate bash --shims)"`},
		}
	case "fish":
		lines = []line{{rcFile(env, shell), mise + ` activate fish | source`}}
	default:
		return nil, errors.New(locales.MiseShellUnsupported(shell))
	}
	var steps []Step
	for _, l := range lines {
		data, err := os.ReadFile(l.file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", l.file, err)
		}
		if activates(string(data)) {
			return nil, nil
		}
		after := AddToBlock(string(data), l.text)
		if after != string(data) {
			steps = append(steps, Step{Edit: &FileEdit{Path: l.file, Before: string(data), After: after, New: err != nil}})
		}
	}
	return steps, nil
}

// activates reports that a startup file runs mise activate outside a
// comment, as the user's own setup or an earlier one of Hearth's does.
func activates(text string) bool {
	for _, l := range splitLines(text) {
		code, _, _ := strings.Cut(l, "#")
		if strings.Contains(code, "mise activate") && !strings.Contains(code, "--shims") {
			return true
		}
	}
	return false
}
