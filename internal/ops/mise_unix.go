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
	s := MiseSetup{Mise: findMise(env), envPath: env.Get("PATH")}
	if s.Mise == "" {
		var err error
		if s, err = installMise(env); err != nil {
			return MiseSetup{}, err
		}
		s.envPath = env.Get("PATH")
	}
	hook, err := hookMise(env, s.Mise)
	if err != nil {
		return MiseSetup{}, err
	}
	s.Steps = append(s.Steps, hook...)
	return s, nil
}

// findMise is where mise is: on the terminal's PATH, or where its own
// installer, Cargo, and Homebrew put it, which a startup file may not have
// put on PATH yet; "" when nowhere.
func findMise(env sysenv.Env) string {
	if found := sysenv.LookPathAll("mise", env); len(found) > 0 {
		return found[0]
	}
	home := env.Get("HOME")
	places := []string{
		filepath.Join(home, ".local", "bin", "mise"),
		filepath.Join(home, ".cargo", "bin", "mise"),
		filepath.Join(home, ".linuxbrew", "bin", "mise"),
	}
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
// without a prompt, as editors' tasks. When mise lives outside standard
// system directories and is not yet on PATH, its directory is also added
// to PATH so new terminals find mise (and Homebrew when beside it). Each
// line goes into Hearth's block; none when the interactive startup file
// activates mise already.
func hookMise(env sysenv.Env, mise string) ([]Step, error) {
	home := env.Get("HOME")
	rawDir := filepath.Dir(mise)
	if home != "" && strings.HasPrefix(mise, home+"/") {
		mise = "$HOME" + strings.TrimPrefix(mise, home)
	}
	dirNamed := rawDir
	if home != "" && strings.HasPrefix(rawDir, home+"/") {
		dirNamed = "$HOME" + strings.TrimPrefix(rawDir, home)
	}
	needsPath := rawDir != "" && rawDir != "." && rawDir != "/usr/bin" && rawDir != "/bin" && !envHasPathDir(env, rawDir)

	type line struct {
		file  string
		lines []string
		shims bool
	}
	withPath := func(pathLine, activateLine string) []string {
		if needsPath {
			return []string{pathLine, activateLine}
		}
		return []string{activateLine}
	}
	posixPath := `export PATH="` + dirNamed + `:$PATH"`
	fishPath := `fish_add_path ` + dirNamed

	var entries []line
	switch shell := filepath.Base(env.Get("SHELL")); shell {
	case "zsh":
		rc := rcFile(env, shell)
		entries = []line{
			{file: rc, lines: withPath(posixPath, `eval "$(`+mise+` activate zsh)"`)},
			{file: filepath.Join(filepath.Dir(rc), ".zprofile"), lines: withPath(posixPath, `eval "$(`+mise+` activate zsh --shims)"`), shims: true},
		}
	case "bash":
		// The shims go to .profile, which Linux desktop sessions read as
		// the user signs in, so that apps see them too. If bash uses a
		// separate login file (.bash_profile / .bash_login) that does not
		// source .bashrc or .profile, add the interactive activation there too.
		activate := withPath(posixPath, `eval "$(`+mise+` activate bash)"`)
		entries = []line{
			{file: rcFile(env, shell), lines: activate},
			{file: filepath.Join(home, ".profile"), lines: withPath(posixPath, `eval "$(`+mise+` activate bash --shims)"`), shims: true},
		}
		if login := bashLogin(home); login != "" && login != filepath.Join(home, ".profile") && !bashLoginSourcesRC(home, login) && !fileMentions(login, ".profile") {
			entries = append(entries, line{file: login, lines: activate})
		}
	case "fish":
		entries = []line{{file: rcFile(env, shell), lines: withPath(fishPath, mise+` activate fish | source`)}}
	default:
		return nil, errors.New(locales.MiseShellUnsupported(shell))
	}
	var steps []Step
	for i, l := range entries {
		data, err := os.ReadFile(l.file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", l.file, err)
		}
		if !l.shims && activates(string(data)) {
			if i == 0 {
				return nil, nil
			}
			continue
		}
		if l.shims && activatesShims(string(data)) {
			continue
		}
		after := AddToBlock(string(data), l.lines...)
		if after != string(data) {
			steps = append(steps, Step{Edit: &FileEdit{Path: l.file, Before: string(data), After: after, New: err != nil}})
		}
	}
	return steps, nil
}

func envHasPathDir(env sysenv.Env, dir string) bool {
	for _, p := range filepath.SplitList(env.Get("PATH")) {
		if sysenv.SamePath(p, dir) {
			return true
		}
	}
	return false
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

func activatesShims(text string) bool {
	for _, l := range splitLines(text) {
		code, _, _ := strings.Cut(l, "#")
		if strings.Contains(code, "mise activate") && strings.Contains(code, "--shims") {
			return true
		}
	}
	return false
}
