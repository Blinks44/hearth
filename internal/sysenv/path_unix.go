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
		files = append(files, filepath.Join(home, ".bashrc"))
		for _, n := range []string{".bash_profile", ".bash_login", ".profile"} {
			p := filepath.Join(home, n)
			if _, err := os.Stat(p); err == nil {
				files = append(files, p)
				break
			}
		}
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

func sourceFiles(env Env, files []string) map[string][]inventory.PathSource {
	out := map[string][]inventory.PathSource{}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil || info.Size() > 1<<20 || !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		plain := file == "/etc/paths" || strings.HasPrefix(file, "/etc/paths.d/")
		for i, line := range strings.Split(string(b), "\n") {
			line, _, _ = strings.Cut(line, "#")
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var values []string
			if plain {
				values = []string{line}
			} else if m := brewShellenv.FindStringSubmatch(line); m != nil {
				values = []string{m[1] + "/bin", m[1] + "/sbin"}
			} else {
				// A command substitution is computed: its directories stay unknown.
				if !pathLine.MatchString(line) || strings.Contains(line, "$(") || strings.Contains(line, "`") {
					continue
				}
				values = strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune(" \t:=()\"'", r) })
			}
			seen := map[string]bool{}
			for _, v := range values {
				if v == "$PATH" || v == "${PATH}" {
					continue
				}
				v = expandPath(v, env)
				if !filepath.IsAbs(v) || strings.ContainsAny(v, "`;$()") {
					continue
				}
				key := pathKey(v)
				if seen[key] {
					continue
				}
				seen[key] = true
				out[key] = append(out[key], inventory.PathSource{File: file, Line: i + 1})
			}
		}
	}
	return out
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
