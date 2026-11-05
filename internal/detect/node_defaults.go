package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// nodeDefaults returns the Node version each version manager picks by
// default, by manager, read from the manager's own files. installed lists
// the versions each manager has, to settle a request like "22" or "lts".
func nodeDefaults(d sources.Dirs, env sysenv.Env, installed map[string][]string) map[string]string {
	defaults := map[string]string{}
	set := func(manager, want string) {
		if v := pickVersion(want, installed[manager]); v != "" {
			defaults[manager] = v
		}
	}
	if d.Nvm != "" {
		set("nvm", nvmAlias(d.Nvm, "default"))
	}
	for _, fnm := range d.Fnm {
		// fnm's default alias links to node-versions/<v>/installation.
		if target, err := filepath.EvalSymlinks(filepath.Join(fnm, "aliases", "default")); err == nil {
			set("fnm", filepath.Base(filepath.Dir(target)))
			break
		}
	}
	if d.Volta != "" {
		var platform struct{ Node struct{ Runtime string } }
		if data, err := os.ReadFile(filepath.Join(d.Volta, "tools", "user", "platform.json")); err == nil && json.Unmarshal(data, &platform) == nil {
			set("volta", platform.Node.Runtime)
		}
	}
	if v := miseDefault(d, env, "node", installed["mise"]); v != "" {
		defaults["mise"] = v
	}
	if d.Home != "" {
		set("asdf", toolVersion(filepath.Join(d.Home, ".tool-versions"), "nodejs"))
	}
	if d.Nodenv != "" {
		set("nodenv", firstLine(readFile(filepath.Join(d.Nodenv, "version"))))
	}
	// nvm-windows switches versions by pointing NVM_SYMLINK at one.
	if link := env.Get("NVM_SYMLINK"); link != "" {
		if target, err := filepath.EvalSymlinks(link); err == nil {
			set("nvm-windows", filepath.Base(target))
		}
	}
	return defaults
}

// pickVersion settles a requested version against the installed ones: an
// exact one, the newest of a line like "22" or "22.11", or the newest of
// all for "node", "stable", "latest" or "lts" (installed versions do not
// tell which are LTS, so the newest stands in). "" when none fits.
func pickVersion(want string, installed []string) string {
	want = strings.TrimPrefix(strings.TrimSpace(want), "v")
	if want == "" {
		return ""
	}
	newest := want == "node" || want == "stable" || want == "latest" || strings.HasPrefix(want, "lts")
	best := ""
	for _, v := range installed {
		if v == want {
			return v
		}
		if (newest || strings.HasPrefix(v, want+".")) && (best == "" || semver.Compare("v"+v, "v"+best) > 0) {
			best = v
		}
	}
	return best
}

// nvmAlias follows nvm's alias files: alias/default may hold another alias,
// like lts/jod, whose file holds the version.
func nvmAlias(dir, name string) string {
	for range 10 {
		value := firstLine(readFile(filepath.Join(dir, "alias", name)))
		if value == "" {
			return name
		}
		name = value
	}
	return name
}

// miseConfig is mise's global config file.
func miseConfig(d sources.Dirs, env sysenv.Env) string {
	if f := env.Get("MISE_GLOBAL_CONFIG_FILE"); f != "" {
		return f
	}
	if dir := env.Get("MISE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.toml")
	}
	if xdg := env.Get("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mise", "config.toml")
	}
	if d.Home == "" {
		return ""
	}
	return filepath.Join(d.Home, ".config", "mise", "config.toml")
}

// miseTool reads the version of tool in a mise config's [tools] table:
// tool = "22", tool = ["22", "20"] or tool = { version = "22" }, the first
// quoted string. It is not a TOML parser, only enough for that table.
func miseTool(path, tool string) string {
	inTools := false
	for line := range strings.Lines(readFile(path)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !inTools || !ok || strings.Trim(strings.TrimSpace(key), `"'`) != tool {
			continue
		}
		for _, q := range []string{`"`, `'`} {
			if _, rest, ok := strings.Cut(value, q); ok {
				v, _, _ := strings.Cut(rest, q)
				return v
			}
		}
	}
	return ""
}

// toolVersion reads the first version of tool in a .tool-versions file,
// lines like "nodejs 22.11.0 20.18.0".
func toolVersion(path, tool string) string {
	for line := range strings.Lines(readFile(path)) {
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == tool {
			return fields[1]
		}
	}
	return ""
}

// readFile is the file's text, or "" when it cannot be read.
func readFile(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
