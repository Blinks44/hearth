//go:build !windows

package sysenv

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// pathEntryUsed reports that commands are looked up in dir: on Unix an
// empty entry is the working directory, so every entry is.
func pathEntryUsed(string) bool { return true }

func pathKey(dir string) string { return filepath.Clean(dir) }

// pathLine matches a line that sets PATH itself, not MANPATH or PYTHONPATH:
// sh assignments, zsh's path array and fish's set and fish_add_path.
var pathLine = regexp.MustCompile(`(^|[\s;])(PATH\+?=|path\+?=\()|^set\s.*\s(PATH|fish_user_paths)(\s|$)|^fish_add_path\s`)

// brewShellenv matches the line Homebrew's installer asks for, as
// eval "$(/opt/homebrew/bin/brew shellenv zsh)": it puts bin and sbin of
// that prefix on PATH.
var brewShellenv = regexp.MustCompile(`([^\s"'$(]+)/bin/brew\s+shellenv`)

// miseActivateShims, pyenvInit, and rbenvInit match the deterministic shim
// hooks their installers (and Hearth) add to shell startup files.
var (
	miseActivateShims = regexp.MustCompile(`(^|[\s"'$(/])mise\s+activate\b.*--shims\b`)
	pyenvInit         = regexp.MustCompile(`(^|[\s"'$(/])pyenv\s+init\b`)
	rbenvInit         = regexp.MustCompile(`(^|[\s"'$(/])rbenv\s+init\b`)
	sourceInclude     = regexp.MustCompile(`(?:^|[;&|]\s*|\bthen\s+)(?:source|\\?\.)\s+(?:"([^"]+)"|'([^']+)'|([^\s;&|<>]+))`)
)

func pathSources(env Env) map[string][]inventory.PathSource {
	home := env.Get("HOME")
	files := []string{"/etc/profile", "/etc/environment", "/etc/paths"}
	shell := filepath.Base(env.Get("SHELL"))
	switch shell {
	case "zsh":
		zd := env.Get("ZDOTDIR")
		if zd == "" {
			zd = home
		}
		files = append(files, "/etc/zshenv", "/etc/zprofile", "/etc/zshrc", "/etc/zlogin")
		for _, n := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
			files = append(files, filepath.Join(zd, n))
		}
	case "bash":
		// bash reads the first of .bash_profile and .bash_login, and .profile
		// only without them; yet they usually source it, as RVM's does, and
		// Ubuntu's desktop session reads it anyway.
		files = append(files, filepath.Join(home, ".bashrc"))
		for _, n := range []string{".bash_profile", ".bash_login"} {
			p := filepath.Join(home, n)
			if _, err := os.Stat(p); err == nil {
				files = append(files, p)
				break
			}
		}
		files = append(files, filepath.Join(home, ".profile"))
	case "fish":
		config := env.Get("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		files = append(files, filepath.Join(config, "fish", "config.fish"))
	default:
		files = append(files, filepath.Join(home, ".profile"))
	}
	parts, _ := filepath.Glob("/etc/paths.d/*")
	files = append(files, parts...)
	return sourceFiles(env, files)
}

const maxSourceDepth = 4

func sourceFiles(env Env, files []string) map[string][]inventory.PathSource {
	out := map[string][]inventory.PathSource{}
	visited := map[string]bool{}
	var scan func(file string, depth int)
	scan = func(file string, depth int) {
		if depth > maxSourceDepth {
			return
		}
		clean := filepath.Clean(file)
		if visited[clean] {
			return
		}
		visited[clean] = true
		info, err := os.Stat(clean)
		if err != nil || info.Size() > 1<<20 || !info.Mode().IsRegular() {
			return
		}
		b, err := os.ReadFile(clean)
		if err != nil {
			return
		}
		for i, line := range strings.Split(string(b), "\n") {
			dirs, hook := lineDirs(clean, line, env)
			for _, v := range dirs {
				key := pathKey(v)
				out[key] = append(out[key], inventory.PathSource{File: clean, Line: i + 1, Hook: hook})
			}
			for _, inc := range sourcedFiles(clean, line, env) {
				scan(inc, depth+1)
			}
		}
	}
	for _, file := range files {
		scan(file, 0)
	}
	return out
}

// sourcedFiles returns static absolute file paths included by `source` or `.`
// statements in line, without executing shell code.
func sourcedFiles(file, line string, env Env) []string {
	if file == "/etc/paths" || strings.HasPrefix(file, "/etc/paths.d/") {
		return nil
	}
	line, _, _ = strings.Cut(line, "#")
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	matches := sourceInclude.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	var out []string
	for _, m := range matches {
		target := m[1]
		if target == "" {
			target = m[2]
		}
		if target == "" {
			target = m[3]
		}
		v := expandPath(target, env)
		if !filepath.IsAbs(v) || strings.ContainsAny(v, "`;$()*?[]<>") {
			continue
		}
		out = append(out, filepath.Clean(v))
	}
	return out
}

// LineDirs are the directories a line of file puts on PATH, as far as
// Hearth can tell without running it, each once: none for a line that
// computes them, as an arbitrary command substitution does.
func LineDirs(file, line string, env Env) []string {
	dirs, _ := lineDirs(file, line, env)
	return dirs
}

func lineDirs(file, line string, env Env) ([]string, bool) {
	plain := file == "/etc/paths" || strings.HasPrefix(file, "/etc/paths.d/")
	line, _, _ = strings.Cut(line, "#")
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	var values []string
	hook := false
	switch {
	case plain:
		values = []string{line}
	case brewShellenv.MatchString(line):
		m := brewShellenv.FindStringSubmatch(line)
		values = []string{m[1] + "/bin", m[1] + "/sbin"}
		hook = true
	case miseActivateShims.MatchString(line):
		if dir := miseShimsDir(env); dir != "" {
			values = []string{dir}
		}
		hook = true
	case pyenvInit.MatchString(line):
		if dir := managerShimsDir(env, "PYENV_ROOT", ".pyenv"); dir != "" {
			values = []string{dir}
		}
		hook = true
	case rbenvInit.MatchString(line):
		if dir := managerShimsDir(env, "RBENV_ROOT", ".rbenv"); dir != "" {
			values = []string{dir}
		}
		hook = true
	default:
		// A command substitution is computed: its directories stay unknown.
		if !pathLine.MatchString(line) || strings.Contains(line, "$(") || strings.Contains(line, "`") {
			return nil, false
		}
		values = strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune(" \t:=()\"';", r) })
	}
	var dirs []string
	seen := map[string]bool{}
	for _, v := range values {
		if v == "$PATH" || v == "${PATH}" {
			continue
		}
		v = expandPath(v, env)
		if !filepath.IsAbs(v) || strings.ContainsAny(v, "`;$()") || seen[pathKey(v)] {
			continue
		}
		seen[pathKey(v)] = true
		dirs = append(dirs, v)
	}
	return dirs, hook
}

func miseShimsDir(env Env) string {
	if d := env.Get("MISE_DATA_DIR"); d != "" {
		return filepath.Join(d, "shims")
	}
	if d := env.Get("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "mise", "shims")
	}
	if h := env.Get("HOME"); h != "" {
		return filepath.Join(h, ".local", "share", "mise", "shims")
	}
	return ""
}

func managerShimsDir(env Env, rootVar, homeSubdir string) string {
	if d := env.Get(rootVar); d != "" {
		return filepath.Join(d, "shims")
	}
	if h := env.Get("HOME"); h != "" {
		return filepath.Join(h, homeSubdir, "shims")
	}
	return ""
}

func expandPath(value string, env Env) string {
	value = strings.Trim(value, "\"'")
	if value == "~" {
		return env.Get("HOME")
	}
	if strings.HasPrefix(value, "~/") {
		value = env.Get("HOME") + value[1:]
	}
	return os.Expand(value, func(key string) string {
		v := env.Get(key)
		if v == "" {
			return "$" + key
		}
		return v
	})
}
